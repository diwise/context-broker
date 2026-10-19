package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Each test uses a fresh schema in an explicitly supplied test database.
// The fixture mirrors the relevant TRoE keys and includes all value families.
func testDatabase(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	address := os.Getenv("TROE_CLEANER_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("set TROE_CLEANER_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	root, err := pgxpool.New(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(root.Close)
	schema := pgx.Identifier{"cleaner_" + strings.ReplaceAll(uuid.NewString(), "-", "")}.Sanitize()
	if _, err := root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if _, err := root.Exec(cleanup, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})
	cfg, err := pgxpool.ParseConfig(address)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	p, err := newPool(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	_, err = p.Exec(ctx, `
CREATE TABLE entities (instanceid text PRIMARY KEY, ts timestamp NOT NULL, opmode text, id text NOT NULL, type text);
CREATE TABLE attributes (
    instanceid text NOT NULL, ts timestamp NOT NULL, datasetid text NOT NULL DEFAULT '',
    id text NOT NULL, entityid text NOT NULL, opmode text, observedat timestamp,
    subproperties boolean DEFAULT false, unitcode text, valuetype text,
    text text, number double precision, boolean boolean, datetime timestamp,
    compound jsonb, geopoint text, correlator text,
    PRIMARY KEY (instanceid,datasetid,ts));
CREATE TABLE subattributes (
    instanceid text NOT NULL, ts timestamp NOT NULL, id text NOT NULL,
    entityid text NOT NULL, attrinstanceid text NOT NULL, attrdatasetid text NOT NULL DEFAULT '',
    observedat timestamp, unitcode text, valuetype text, text text, number double precision,
    correlator text, PRIMARY KEY (instanceid,ts));
CREATE INDEX subattributes_attributeid_index ON subattributes(attrinstanceid,attrdatasetid);
INSERT INTO entities VALUES ('entity-create','2026-01-01 00:00:00','Create','entity','Beach');`)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, p
}

var baseTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

type row struct {
	id, dataset, mode string
	second            int
	data              map[string]any
}

func state(id, mode, value string, second int) row {
	return row{id: id, mode: mode, second: second, data: map[string]any{"valuetype": "String", "text": value}}
}

func insertRows(t *testing.T, ctx context.Context, p *pgxpool.Pool, rows ...row) {
	t.Helper()
	for _, r := range rows {
		columns := []string{"instanceid", "ts", "datasetid", "id", "entityid", "opmode", "correlator"}
		values := []any{r.id, baseTime.Add(time.Duration(r.second) * time.Second), r.dataset, "description", "entity", r.mode, "write-" + r.id}
		for _, column := range []string{"observedat", "subproperties", "unitcode", "valuetype", "text", "number", "boolean", "datetime", "compound", "geopoint"} {
			if value, ok := r.data[column]; ok {
				columns = append(columns, column)
				values = append(values, value)
			}
		}
		placeholders := make([]string, len(values))
		for i := range placeholders {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		}
		_, err := p.Exec(ctx, "INSERT INTO attributes ("+strings.Join(columns, ",")+") VALUES ("+strings.Join(placeholders, ",")+")", values...)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func retained(t *testing.T, ctx context.Context, p *pgxpool.Pool) []string {
	t.Helper()
	rows, err := p.Query(ctx, `SELECT instanceid FROM attributes ORDER BY ts,instanceid,datasetid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestConsecutiveWrites(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows []row
		want []string
	}{
		{"sync-transitions", []row{state("a1", "Create", "A", 1), state("a2", "Replace", "A", 2), state("a3", "Replace", "A", 3), state("b1", "Replace", "B", 4), state("b2", "Replace", "B", 5), state("a4", "Replace", "A", 6), state("a5", "Replace", "A", 7)}, []string{"a1", "b1", "a4"}},
		{"first-replace", []row{state("a1", "Replace", "A", 1), state("a2", "Replace", "A", 2)}, []string{"a1"}},
		{"single-field-updates", []row{state("a1", "Create", "A", 1), state("a2", "Update", "A", 2), state("a3", "Update", "A", 3), state("b1", "Update", "B", 4), state("b2", "Update", "B", 5)}, []string{"a1", "b1"}},
		{"attribute-recreated", []row{state("a1", "Create", "A", 1), state("a2", "Replace", "A", 2), {id: "delete", mode: "Delete", second: 3}, state("append", "Append", "A", 4), state("a3", "Replace", "A", 5)}, []string{"a1", "delete", "append"}},
		{"missing-append-after-delete", []row{state("a1", "Replace", "A", 1), {id: "delete", mode: "Delete", second: 2}, state("a2", "Replace", "A", 3), state("a3", "Replace", "A", 4)}, []string{"a1", "delete", "a2"}},
		{"preserve-create-markers", []row{state("a1", "Create", "A", 1), state("a2", "Create", "A", 2), state("a3", "Replace", "A", 3)}, []string{"a1", "a2"}},
		{"ambiguous-order", []row{state("a1", "Replace", "A", 1), state("b1", "Replace", "B", 1), state("b2", "Replace", "B", 2), state("b3", "Replace", "B", 3)}, []string{"a1", "b1", "b2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, p := testDatabase(t)
			insertRows(t, ctx, p, tc.rows...)
			removed, children, err := cleanEntity(ctx, p, "entity")
			if err != nil || removed != int64(len(tc.rows)-len(tc.want)) || children != 0 {
				t.Fatalf("removed=%d children=%d err=%v", removed, children, err)
			}
			if got := retained(t, ctx, p); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("retained=%v want=%v", got, tc.want)
			}
			if removed, children, err := cleanEntity(ctx, p, "entity"); err != nil || removed != 0 || children != 0 {
				t.Fatalf("second run: %d %d %v", removed, children, err)
			}
		})
	}
}

func TestValuesDatasetsAndMetadata(t *testing.T) {
	for _, data := range []map[string]any{
		{"valuetype": "Number", "number": 21.0, "unitcode": "CEL"},
		{"valuetype": "Boolean", "boolean": true},
		{"valuetype": "Relationship", "text": "urn:ngsi-ld:Place:1"},
		{"valuetype": "Compound", "compound": `{"coordinates":[1,2],"label":"A"}`},
		{"valuetype": "DateTime", "datetime": baseTime},
		{"valuetype": "GeoPoint", "geopoint": "POINT(1 2)"},
	} {
		t.Run(data["valuetype"].(string), func(t *testing.T) {
			ctx, p := testDatabase(t)
			insertRows(t, ctx, p, row{id: "first", mode: "Create", second: 1, data: data}, row{id: "sync", mode: "Replace", second: 2, data: data})
			if n, _, err := cleanEntity(ctx, p, "entity"); err != nil || n != 1 {
				t.Fatalf("removed=%d err=%v", n, err)
			}
			if got := retained(t, ctx, p); !reflect.DeepEqual(got, []string{"first"}) {
				t.Fatal(got)
			}
		})
	}
	t.Run("observation-time-and-replay", func(t *testing.T) {
		ctx, p := testDatabase(t)
		for i, r := range []struct {
			value       float64
			observation int
		}{{21, 11}, {21, 11}, {21, 12}, {21, 12}, {18, 10}, {21, 12}} {
			insertRows(t, ctx, p, row{id: fmt.Sprint(i), mode: "Replace", second: i + 1, data: map[string]any{"valuetype": "Number", "number": r.value, "observedat": baseTime.Add(time.Duration(r.observation) * time.Hour)}})
		}
		if n, _, err := cleanEntity(ctx, p, "entity"); err != nil || n != 2 {
			t.Fatalf("removed=%d err=%v", n, err)
		}
		if got := retained(t, ctx, p); !reflect.DeepEqual(got, []string{"0", "2", "4", "5"}) {
			t.Fatal(got)
		}
	})
	t.Run("dataset-separation", func(t *testing.T) {
		ctx, p := testDatabase(t)
		a := state("a", "Append", "A", 1)
		a.dataset = "urn:dataset:a"
		b := state("b", "Append", "A", 2)
		b.dataset = "urn:dataset:b"
		a2 := state("a2", "Replace", "A", 3)
		a2.dataset = a.dataset
		b2 := state("b2", "Replace", "A", 4)
		b2.dataset = b.dataset
		insertRows(t, ctx, p, a, b, a2, b2)
		if n, _, err := cleanEntity(ctx, p, "entity"); err != nil || n != 2 {
			t.Fatalf("removed=%d err=%v", n, err)
		}
		if got := retained(t, ctx, p); !reflect.DeepEqual(got, []string{"a", "b"}) {
			t.Fatal(got)
		}
	})
	t.Run("unit-change", func(t *testing.T) {
		ctx, p := testDatabase(t)
		for i, unit := range []string{"CEL", "FAH", "FAH"} {
			insertRows(t, ctx, p, row{id: fmt.Sprint(i), mode: "Replace", second: i + 1, data: map[string]any{"valuetype": "Number", "number": 21.0, "unitcode": unit}})
		}
		if n, _, err := cleanEntity(ctx, p, "entity"); err != nil || n != 1 {
			t.Fatalf("removed=%d err=%v", n, err)
		}
		if got := retained(t, ctx, p); !reflect.DeepEqual(got, []string{"0", "1"}) {
			t.Fatal(got)
		}
	})
}

func addMetadata(t *testing.T, ctx context.Context, p *pgxpool.Pool, parent, dataset, quality string, second int) {
	t.Helper()
	for _, field := range []string{"quality", "source"} {
		_, err := p.Exec(ctx, `INSERT INTO subattributes(instanceid,ts,id,entityid,attrinstanceid,attrdatasetid,valuetype,text,correlator)
VALUES($1,$2,$3,'entity',$4,$5,'String',$6,$7)`, parent+dataset+field, baseTime.Add(time.Duration(second)*time.Second), field, parent, dataset, quality, "trace-"+parent)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestSubattributesAndSharedKeys(t *testing.T) {
	t.Run("metadata-change-and-orphans", func(t *testing.T) {
		ctx, p := testDatabase(t)
		for i, quality := range []string{"good", "bad", "bad"} {
			r := state(fmt.Sprint(i), "Replace", "A", i+1)
			r.data["subproperties"] = true
			insertRows(t, ctx, p, r)
			addMetadata(t, ctx, p, r.id, "", quality, i+1)
		}
		if n, s, err := cleanEntity(ctx, p, "entity"); err != nil || n != 1 || s != 2 {
			t.Fatalf("removed=%d subattributes=%d err=%v", n, s, err)
		}
		if got := retained(t, ctx, p); !reflect.DeepEqual(got, []string{"0", "1"}) {
			t.Fatal(got)
		}
		var orphanCount int
		if err := p.QueryRow(ctx, `SELECT count(*) FROM subattributes s WHERE NOT EXISTS(SELECT 1 FROM attributes a WHERE a.instanceid=s.attrinstanceid AND a.datasetid=s.attrdatasetid)`).Scan(&orphanCount); err != nil || orphanCount != 0 {
			t.Fatalf("orphans=%d err=%v", orphanCount, err)
		}
	})
	t.Run("same-instance-across-datasets", func(t *testing.T) {
		ctx, p := testDatabase(t)
		a := state("first", "Create", "A", 1)
		a.dataset = "a"
		a.data["subproperties"] = true
		b := state("shared", "Append", "B", 2)
		b.dataset = "b"
		b.data["subproperties"] = true
		dup := state("shared", "Replace", "A", 3)
		dup.dataset = "a"
		dup.data["subproperties"] = true
		insertRows(t, ctx, p, a, b, dup)
		addMetadata(t, ctx, p, a.id, a.dataset, "good", 1)
		addMetadata(t, ctx, p, b.id, b.dataset, "other", 2)
		addMetadata(t, ctx, p, dup.id, dup.dataset, "good", 3)
		if n, s, err := cleanEntity(ctx, p, "entity"); err != nil || n != 1 || s != 2 {
			t.Fatalf("removed=%d children=%d err=%v", n, s, err)
		}
		var datasets []string
		rows, err := p.Query(ctx, `SELECT attrdatasetid FROM subattributes WHERE attrinstanceid='shared' ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var dataset string
			if err := rows.Scan(&dataset); err != nil {
				t.Fatal(err)
			}
			datasets = append(datasets, dataset)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(datasets, []string{"b", "b"}) {
			t.Fatal(datasets)
		}
	})
	t.Run("surviving-parent-version", func(t *testing.T) {
		ctx, p := testDatabase(t)
		a := state("shared", "Create", "A", 1)
		a.data["subproperties"] = true
		b := state("shared", "Replace", "A", 2)
		b.data["subproperties"] = true
		insertRows(t, ctx, p, a, b)
		addMetadata(t, ctx, p, "shared", "", "good", 1)
		if n, s, err := cleanEntity(ctx, p, "entity"); err != nil || n != 1 || s != 0 {
			t.Fatalf("removed=%d children=%d err=%v", n, s, err)
		}
		var count int
		if err := p.QueryRow(ctx, `SELECT count(*) FROM subattributes`).Scan(&count); err != nil || count != 2 {
			t.Fatalf("children=%d err=%v", count, err)
		}
	})
}

func TestEntityLifecyclesAndIsolation(t *testing.T) {
	ctx, p := testDatabase(t)
	_, err := p.Exec(ctx, `INSERT INTO entities VALUES ('entity-delete','2026-01-01 00:00:03','Delete','entity',NULL),('entity-recreate','2026-01-01 00:00:04','Create','entity','Beach'),('other-create','2026-01-01 00:00:00','Create','other','Beach')`)
	if err != nil {
		t.Fatal(err)
	}
	insertRows(t, ctx, p, state("a1", "Replace", "A", 1), state("a2", "Replace", "A", 2), state("recreate", "Create", "A", 4), state("a3", "Replace", "A", 5))
	_, err = p.Exec(ctx, `INSERT INTO attributes(instanceid,ts,id,entityid,opmode,valuetype,text) VALUES ('other1','2026-01-01 00:00:01','description','other','Replace','String','A'),('other2','2026-01-01 00:00:02','description','other','Replace','String','A')`)
	if err != nil {
		t.Fatal(err)
	}
	if n, _, err := cleanEntity(ctx, p, "entity"); err != nil || n != 2 {
		t.Fatalf("removed=%d err=%v", n, err)
	}
	var ids []string
	rows, err := p.Query(ctx, `SELECT instanceid FROM attributes WHERE entityid='entity' ORDER BY ts`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids, []string{"a1", "recreate"}) {
		t.Fatal(ids)
	}
	var count int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM attributes WHERE entityid='other'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("other rows=%d err=%v", count, err)
	}
	if err := p.QueryRow(ctx, `SELECT count(*) FROM entities WHERE id='entity'`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("lifecycle markers=%d err=%v", count, err)
	}
}

func TestAtomicRollbackCancellationAndConcurrentRuns(t *testing.T) {
	for _, mode := range []string{"rollback", "cancel", "concurrent"} {
		t.Run(mode, func(t *testing.T) {
			ctx, p := testDatabase(t)
			first := state("first", "Create", "A", 1)
			first.data["subproperties"] = true
			dup := state("dup", "Replace", "A", 2)
			dup.data["subproperties"] = true
			insertRows(t, ctx, p, first, dup)
			addMetadata(t, ctx, p, "first", "", "good", 1)
			addMetadata(t, ctx, p, "dup", "", "good", 2)
			switch mode {
			case "rollback":
				_, err := p.Exec(ctx, `CREATE FUNCTION reject_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test deletion failure'; END $$; CREATE TRIGGER reject_delete BEFORE DELETE ON subattributes FOR EACH ROW EXECUTE FUNCTION reject_delete()`)
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err := cleanEntity(ctx, p, "entity"); err == nil {
					t.Fatal("expected child deletion failure")
				}
			case "cancel":
				tx, err := p.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(ctx)
				if _, err := tx.Exec(ctx, `SELECT 1 FROM attributes WHERE instanceid='dup' FOR UPDATE`); err != nil {
					t.Fatal(err)
				}
				short, stop := context.WithTimeout(ctx, 50*time.Millisecond)
				defer stop()
				if _, _, err := cleanEntity(short, p, "entity"); err == nil || !errors.Is(short.Err(), context.DeadlineExceeded) {
					t.Fatalf("cancellation not applied: %v", err)
				}
				if err := tx.Rollback(ctx); err != nil {
					t.Fatal(err)
				}
			case "concurrent":
				var wg sync.WaitGroup
				results := make(chan error, 2)
				for range 2 {
					wg.Go(func() { ; _, _, err := cleanEntity(ctx, p, "entity"); results <- err })
				}
				wg.Wait()
				close(results)
				for err := range results {
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			wantRows, wantSubs := 2, 4
			if mode == "concurrent" {
				wantRows, wantSubs = 1, 2
			}
			if got := retained(t, ctx, p); len(got) != wantRows {
				t.Fatalf("retained=%v", got)
			}
			var count int
			if err := p.QueryRow(ctx, `SELECT count(*) FROM subattributes`).Scan(&count); err != nil || count != wantSubs {
				t.Fatalf("children=%d err=%v", count, err)
			}
		})
	}
}

func TestLargeSyncRun(t *testing.T) {
	ctx, p := testDatabase(t)
	const writes = 5000
	_, err := p.Exec(ctx, `
INSERT INTO attributes(instanceid,ts,id,entityid,opmode,valuetype,text,subproperties)
SELECT 'sync-'||i, timestamp '2026-01-01' + i * interval '1 second',
       'description','entity','Replace','String','Sandstrand',true
FROM generate_series(1,$1::int) i;`, writes)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Exec(ctx, `INSERT INTO subattributes(instanceid,ts,id,entityid,attrinstanceid,valuetype,text)
SELECT instanceid||'-quality',ts,'quality','entity',instanceid,'String','verified' FROM attributes;`)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	attributes, children, err := cleanEntity(ctx, p, "entity")
	if err != nil || attributes != writes-1 || children != writes-1 {
		t.Fatalf("removed=%d children=%d err=%v", attributes, children, err)
	}
	t.Logf("compacted %d sync writes with metadata in %s", writes, time.Since(start))
	if got := retained(t, ctx, p); !reflect.DeepEqual(got, []string{"sync-1"}) {
		t.Fatal(got)
	}
}

// This test uses actual Orion-LD 1.12.0/TRoE and Mintaka 0.7.0. The tenant and
// entity are unique. Use isolated services: deleting the entity leaves history.
func TestOrionMintakaSync(t *testing.T) {
	orion, mintaka, database := os.Getenv("TROE_CLEANER_TEST_ORION_URL"), os.Getenv("TROE_CLEANER_TEST_MINTAKA_URL"), os.Getenv("TROE_CLEANER_TEST_DATABASE_URL")
	if orion == "" || mintaka == "" || database == "" {
		t.Skip("set TROE_CLEANER_TEST_ORION_URL, TROE_CLEANER_TEST_MINTAKA_URL and TROE_CLEANER_TEST_DATABASE_URL for real backend verification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	tenant := "cleaner" + strings.ReplaceAll(uuid.NewString(), "-", "")
	id := "urn:ngsi-ld:Beach:" + tenant
	const jsonld = "https://raw.githubusercontent.com/diwise/context-broker/main/assets/jsonldcontexts/default-context.jsonld"
	client := &http.Client{Timeout: 30 * time.Second}
	defer client.CloseIdleConnections()
	call := func(method, base, path string, body any) map[string]any {
		t.Helper()
		var reader io.Reader
		if body != nil {
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			reader = bytes.NewReader(data)
		}
		req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, reader)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("NGSILD-Tenant", tenant)
		req.Header.Set("Link", fmt.Sprintf(`<%s>; rel="http://www.w3.org/ns/json-ld#context"; type="application/ld+json"`, jsonld))
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
			t.Fatalf("%s %s returned HTTP %d", method, path, resp.StatusCode)
		}
		if resp.StatusCode != http.StatusOK {
			return nil
		}
		var result map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	fragment := func(description string) map[string]any {
		return map[string]any{
			"description": map[string]any{"type": "Property", "value": description,
				"quality": map[string]any{"type": "Property", "value": "verified"},
				"source":  map[string]any{"type": "Property", "value": "registry"}},
			"name":   map[string]any{"type": "Property", "value": "Stranden"},
			"active": map[string]any{"type": "Property", "value": true},
		}
	}
	initial := fragment("A")
	initial["id"] = id
	initial["type"] = "Beach"
	path := "/ngsi-ld/v1/entities/" + url.PathEscape(id)
	call(http.MethodPost, orion, "/ngsi-ld/v1/entities", initial)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		req, _ := http.NewRequestWithContext(cleanup, http.MethodDelete, strings.TrimRight(orion, "/")+path, nil)
		req.Header.Set("NGSILD-Tenant", tenant)
		resp, err := client.Do(req)
		if err != nil {
			t.Errorf("cleanup: %v", err)
			return
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("cleanup status %d", resp.StatusCode)
		}
	})
	for _, description := range []string{"A", "A", "B", "B", "A", "A"} {
		time.Sleep(5 * time.Millisecond)
		call(http.MethodPatch, orion, path+"/attrs", fragment(description))
	}
	for range 2 {
		time.Sleep(5 * time.Millisecond)
		call(http.MethodPatch, orion, path+"/attrs/description", fragment("A")["description"])
	}
	cfg, err := pgxpool.ParseConfig(database)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = "orion_" + tenant
	p, err := newPool(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	attributes, children, err := cleanEntity(ctx, p, id)
	if err != nil || attributes != 18 || children != 12 {
		t.Fatalf("removed=%d children=%d err=%v", attributes, children, err)
	}
	if n, s, err := cleanEntity(ctx, p, id); err != nil || n != 0 || s != 0 {
		t.Fatalf("second run: %d %d %v", n, s, err)
	}
	current := call(http.MethodGet, orion, path, nil)
	if current["description"].(map[string]any)["value"] != "A" {
		t.Fatal("cleaner changed snapshot")
	}
	// Orion 1.12.0 and Mintaka 0.7.0 use different core-context mappings for
	// the compact term "description". Query its stored, expanded IRI.
	const descriptionIRI = "http://purl.org/dc/terms/description"
	q := url.Values{"attrs": {descriptionIRI}, "timeproperty": {"modifiedAt"}, "options": {"sysAttrs"}}
	history := call(http.MethodGet, mintaka, "/ngsi-ld/v1/temporal/entities/"+url.PathEscape(id)+"?"+q.Encode(), nil)
	instances, ok := history[descriptionIRI].([]any)
	if !ok || len(instances) != 3 {
		t.Fatalf("expected A,B,A, got %v", history)
	}
	for i, want := range []string{"A", "B", "A"} {
		attr := instances[i].(map[string]any)
		if attr["value"] != want || attr["quality"].(map[string]any)["value"] != "verified" || attr["source"].(map[string]any)["value"] != "registry" {
			t.Fatalf("history changed: %v", attr)
		}
	}
	var count int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM subattributes s WHERE s.entityid=$1 AND NOT EXISTS(SELECT 1 FROM attributes a WHERE a.instanceid=s.attrinstanceid AND a.datasetid=s.attrdatasetid)`, id).Scan(&count); err != nil || count != 0 {
		t.Fatalf("orphans=%d err=%v", count, err)
	}
	if err := vacuum(ctx, p); err != nil {
		t.Fatal(err)
	}
}
