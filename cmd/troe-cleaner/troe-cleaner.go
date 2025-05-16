package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/diwise/service-chassis/pkg/infrastructure/buildinfo"
	"github.com/diwise/service-chassis/pkg/infrastructure/env"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	appName string = "troe-cleaner"
)

func main() {
	appVersion := buildinfo.SourceVersion()

	ctx, log, cleanup := o11y.Init(context.Background(), appName, appVersion, "json")
	defer cleanup()

	log.Debug("begin clean troe")

	p, err := connect(ctx, LoadConfiguration(ctx))
	if err != nil {
		log.Error("failed to connect to database", "err", err.Error())
		os.Exit(1)
	}
	defer p.Close()

	entities, err := getEntites(ctx, p)
	if err != nil {
		log.Error("failed to get entities", "err", err.Error())
		os.Exit(1)
	}

	log.Debug("number of total entities", "count", len(entities))

	var totalCount int64 = 0

	for _, entity := range entities {
		l := log.With(slog.String("entity_id", entity))

		l.Debug("find duplicates for entity", slog.Time("start_time", time.Now()))

		dups, err := findDuplicates(ctx, p, entity)
		if err != nil {
			l.Error("failed to get duplicates", "err", err.Error())
			os.Exit(1)
		}

		if len(dups) == 0 {
			l.Debug("found no duplicates", slog.Time("end_time", time.Now()))
			continue
		}

		totalCount += int64(len(dups))

		err = deleteDuplicates(ctx, p, dups)
		if err != nil {
			l.Error("failed to delete duplicates", "err", err.Error())
			os.Exit(1)
		}

		l.Debug("done cleaning duplicates", slog.Int("count", len(dups)), slog.Time("end_time", time.Now()))
	}

	log.Debug("vacuum")

	err = vacuum(ctx, p)
	if err != nil {
		log.Error("failed to vacuum table", "err", err.Error())
		os.Exit(1)
	}

	log.Info("done cleaning", slog.Int64("total", totalCount))
}

type Config struct {
	host     string
	user     string
	password string
	port     string
	dbname   string
	sslmode  string
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
	conn, err := pgxpool.New(ctx, cfg.ConnStr())
	if err != nil {
		return nil, err
	}

	err = conn.Ping(ctx)
	if err != nil {
		return nil, err
	}

	return conn, err
}

func getEntites(ctx context.Context, p *pgxpool.Pool) ([]string, error) {
	sql := `SELECT distinct id FROM entities ORDER BY id;`

	rows, err := p.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entities := make([]string, 0)

	for rows.Next() {
		var e string
		err := rows.Scan(&e)
		if err != nil {
			return nil, err
		}
		entities = append(entities, e)
	}

	return entities, nil
}

func findDuplicates(ctx context.Context, p *pgxpool.Pool, entityid string) ([]string, error) {
	sql := `
		select distinct instanceid from (
			SELECT instanceid, entityid, id, observedAt, number, ROW_NUMBER() OVER(PARTITION BY entityid, id, observedAt, number ORDER BY ts desc) AS Row
			FROM attributes
			WHERE entityid=$1 and opmode = 'Replace' AND valuetype = 'Number'
		) dups
		where dups.Row > 1;`

	nDups, err := queryDuplicates(ctx, p, entityid, sql)
	if err != nil {
		return nil, err
	}

	sql = `
		select distinct instanceid from (			
			SELECT instanceid, entityid, id, text, ROW_NUMBER() OVER(PARTITION BY entityid, id, text ORDER BY ts desc) AS Row
			FROM attributes
			WHERE entityid=$1
			AND opmode = 'Replace' 	  
			AND observedat is null
			AND valuetype = 'String'
			) dups
		where dups.Row > 1;`

	sDups, err := queryDuplicates(ctx, p, entityid, sql)
	if err != nil {
		return nil, err
	}

	dups := Concat(nDups, sDups)

	return dups, nil
}

func queryDuplicates(ctx context.Context, p *pgxpool.Pool, entityid, sql string) ([]string, error) {
	rows, err := p.Query(ctx, sql, entityid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	instances := make([]string, 0)

	for rows.Next() {
		var i string
		err := rows.Scan(&i)
		if err != nil {
			return nil, err
		}
		instances = append(instances, i)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return instances, nil
}

func deleteDuplicates(ctx context.Context, p *pgxpool.Pool, dups []string) error {
	if len(dups) == 0 {
		return nil
	}

	tx, err := p.Begin(ctx)
	if err != nil {
		return err
	}

	for _, d := range dups {
		sql := `DELETE FROM attributes WHERE instanceid=$1;`

		_, err := tx.Exec(ctx, sql, d)
		if err != nil {
			tx.Rollback(ctx)
			return err
		}
	}

	return tx.Commit(ctx)
}

func vacuum(ctx context.Context, p *pgxpool.Pool) error {
	_, err := p.Exec(ctx, "VACUUM ANALYZE attributes;")
	if err != nil {
		return err
	}

	return nil
}

// from 1.22 slices package
func Concat[S ~[]E, E any](slices ...S) S {
	size := 0
	for _, s := range slices {
		size += len(s)
		if size < 0 {
			panic("len out of range")
		}
	}
	newslice := Grow[S](nil, size)
	for _, s := range slices {
		newslice = append(newslice, s...)
	}
	return newslice
}

func Grow[S ~[]E, E any](s S, n int) S {
	if n < 0 {
		panic("cannot be negative")
	}
	if n -= cap(s) - len(s); n > 0 {
		s = append(s[:cap(s)], make([]E, n)...)[:len(s)]
	}
	return s
}
