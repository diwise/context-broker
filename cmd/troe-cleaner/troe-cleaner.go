package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/diwise/service-chassis/pkg/infrastructure/buildinfo"
	"github.com/diwise/service-chassis/pkg/infrastructure/env"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/tracing"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgconn/ctxwatch"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const appName = "troe-cleaner"

func main() {
	if err := run(); err != nil {
		slog.Error("troe cleaning failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	root, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, log, cleanup := o11y.Init(root, appName, buildinfo.SourceVersion(), "json")
	defer cleanup()

	p, err := connect(ctx, LoadConfiguration(ctx))
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer p.Close()
	entities, err := getEntities(ctx, p)
	if err != nil {
		return fmt.Errorf("get entities: %w", err)
	}
	log.Debug("number of total entities", "count", len(entities))
	var totalAttributes, totalSubattributes int64
	for _, entity := range entities {
		attributes, subattributes, err := cleanEntity(ctx, p, entity)
		if err != nil {
			return fmt.Errorf("clean entity %q: %w", entity, err)
		}
		totalAttributes += attributes
		totalSubattributes += subattributes
		log.Debug("done cleaning redundant writes", "entity_id", entity, "attributes", attributes, "subattributes", subattributes)
	}
	if totalAttributes > 0 {
		if err := vacuum(ctx, p); err != nil {
			return fmt.Errorf("vacuum cleaned tables: %w", err)
		}
	}
	log.Info("done cleaning", "total", totalAttributes, "subattributes", totalSubattributes)
	return nil
}

type Config struct {
	host, user, password, port, dbname, sslmode string
}

func LoadConfiguration(ctx context.Context) Config {
	return Config{
		host:     env.GetVariableOrDefault(ctx, "POSTGRES_HOST", ""),
		user:     env.GetVariableOrDefault(ctx, "POSTGRES_USER", ""),
		password: env.GetVariableOrDefault(ctx, "POSTGRES_PASSWORD", ""),
		port:     env.GetVariableOrDefault(ctx, "POSTGRES_PORT", "5432"),
		dbname:   env.GetVariableOrDefault(ctx, "POSTGRES_DBNAME", "diwise"),
		sslmode:  env.GetVariableOrDefault(ctx, "POSTGRES_SSLMODE", "disable"),
	}
}

func (c Config) ConnStr() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", c.user, c.password, c.host, c.port, c.dbname, c.sslmode)
}

func connect(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.ConnStr())
	if err != nil {
		return nil, err
	}
	conn, err := newPool(ctx, poolConfig)
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func newPool(ctx context.Context, cfg *pgxpool.Config) (*pgxpool.Pool, error) {
	// Closing the client socket alone can leave a blocked DELETE running on
	// PostgreSQL. Send a real cancellation request, with a bounded fallback.
	cfg.ConnConfig.BuildContextWatcherHandler = func(conn *pgconn.PgConn) ctxwatch.Handler {
		return &pgconn.CancelRequestContextWatcherHandler{Conn: conn, DeadlineDelay: 2 * time.Second}
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

var tracer = otel.Tracer("context-broker/troe-cleaner")

func getEntities(ctx context.Context, p *pgxpool.Pool) (entities []string, err error) {
	ctx, span := tracer.Start(ctx, "list-entities")
	defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()
	rows, err := p.Query(ctx, `SELECT DISTINCT id FROM entities ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entities = make([]string, 0)
	for rows.Next() {
		var entity string
		if err := rows.Scan(&entity); err != nil {
			return nil, err
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entities, nil
}

// Classify and delete in one SQL statement/transaction per entity. LAG sees
// every operation, including deletion and creation markers. Only redundant
// Replace/Update rows are removed; first writes and lifecycle markers are retained.
// JSONB compares all stored value/metadata columns, including all subattributes,
// without a type-specific whitelist. Transport IDs/times are not domain data.
const cleanEntitySQL = `
WITH subcontent AS (
    SELECT s.attrinstanceid, COALESCE(s.attrdatasetid, '') AS datasetid,
           jsonb_agg(to_jsonb(s) - ARRAY['instanceid','ts','attrinstanceid','attrdatasetid','correlator']
                     ORDER BY to_jsonb(s) - ARRAY['instanceid','ts','attrinstanceid','attrdatasetid','correlator']) AS content
    FROM subattributes s
    WHERE s.entityid = $1
    GROUP BY s.attrinstanceid, COALESCE(s.attrdatasetid, '')
), events AS (
    SELECT a.ts, a.instanceid, a.id, COALESCE(a.datasetid, '') AS datasetid,
           a.opmode::text AS opmode, false AS boundary,
           jsonb_build_array(
               (to_jsonb(a) - ARRAY['instanceid','ts','opmode','correlator','datasetid'])
                   || jsonb_build_object('datasetid', COALESCE(a.datasetid, '')),
               COALESCE(s.content, '[]'::jsonb)) AS content
    FROM attributes a
    LEFT JOIN subcontent s ON s.attrinstanceid = a.instanceid AND s.datasetid = COALESCE(a.datasetid, '')
    WHERE a.entityid = $1
    UNION ALL
    SELECT e.ts, e.instanceid, '', '', e.opmode::text, true, NULL::jsonb
    FROM entities e
    WHERE e.id = $1 AND e.opmode::text IN ('Create','Delete')
), lifecycles AS (
    SELECT *, count(*) FILTER (WHERE boundary) OVER timeline AS lifecycle,
              max(ts) FILTER (WHERE boundary) OVER timeline AS boundary_at
    FROM events
    WINDOW timeline AS (ORDER BY ts, boundary DESC, instanceid, id, datasetid ROWS UNBOUNDED PRECEDING)
), counted AS (
    SELECT *, count(*) OVER (PARTITION BY lifecycle, id, datasetid, ts) AS same_time_count
    FROM lifecycles
    WHERE NOT boundary
), sequenced AS (
    SELECT *, lag(content) OVER series AS previous_content,
              lag(opmode) OVER series AS previous_mode,
              lag(ts) OVER series AS previous_ts,
              lag(same_time_count) OVER series AS previous_time_count
    FROM counted
    WINDOW series AS (PARTITION BY lifecycle, id, datasetid ORDER BY ts, instanceid)
), redundant AS (
    SELECT instanceid, datasetid, ts
    FROM sequenced
    WHERE opmode IN ('Replace','Update')
      AND previous_mode IN ('Create','Append','Update','Replace')
      AND content = previous_content
      AND ts > previous_ts AND same_time_count = 1 AND previous_time_count = 1
      AND ts > boundary_at
      AND (previous_ts > boundary_at OR previous_mode IN ('Create','Append'))
), removed_attributes AS (
    DELETE FROM attributes a USING redundant r
    WHERE a.entityid = $1 AND a.instanceid = r.instanceid
      AND COALESCE(a.datasetid, '') = r.datasetid AND a.ts = r.ts
    RETURNING a.instanceid, COALESCE(a.datasetid, '') AS datasetid, a.ts
), surviving_parents AS MATERIALIZED (
    -- CTE readers see the pre-delete snapshot. Compute the surviving keys
    -- once, excluding the actual removed rows rather than their candidates.
    SELECT DISTINCT a.instanceid, COALESCE(a.datasetid, '') AS datasetid
    FROM attributes a
    LEFT JOIN removed_attributes d ON d.instanceid = a.instanceid
      AND d.datasetid = COALESCE(a.datasetid, '') AND d.ts = a.ts
    WHERE a.entityid = $1 AND d.instanceid IS NULL
), removed_subattributes AS (
    DELETE FROM subattributes s USING removed_attributes d
    WHERE s.entityid = $1 AND s.attrinstanceid = d.instanceid
      AND COALESCE(s.attrdatasetid, '') = d.datasetid
      AND NOT EXISTS (
          SELECT 1 FROM surviving_parents survivor
          WHERE survivor.instanceid = d.instanceid AND survivor.datasetid = d.datasetid)
    RETURNING s.instanceid
)
SELECT (SELECT count(*) FROM removed_attributes), (SELECT count(*) FROM removed_subattributes);`

func cleanEntity(ctx context.Context, p *pgxpool.Pool, entityID string) (attributes, subattributes int64, err error) {
	ctx, span := tracer.Start(ctx, "clean-entity", trace.WithAttributes(attribute.String("entity_id", entityID)))
	defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()
	err = p.QueryRow(ctx, cleanEntitySQL, entityID).Scan(&attributes, &subattributes)
	return
}

func vacuum(ctx context.Context, p *pgxpool.Pool) (err error) {
	ctx, span := tracer.Start(ctx, "vacuum")
	defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()
	// VACUUM cannot run inside a transaction block; use separate statements.
	for _, table := range []string{"attributes", "subattributes"} {
		if _, err := p.Exec(ctx, "VACUUM ANALYZE "+table); err != nil {
			return err
		}
	}
	return nil
}
