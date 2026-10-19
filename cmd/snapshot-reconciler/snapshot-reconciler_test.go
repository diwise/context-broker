package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/diwise/context-broker/internal/pkg/application/config"
)

const testID = "urn:ngsi-ld:WeatherObserved:reconcile-test"
const older = "2026-01-01T10:00:00Z"
const newer = "2026-01-01T11:00:00Z"
const created = "2025-01-01T00:00:00Z"
const written = "2026-01-02T00:00:00Z"

func decodeEntity(t *testing.T, text string) entity {
	t.Helper()
	var e entity
	d := json.NewDecoder(strings.NewReader(text))
	d.UseNumber()
	if err := d.Decode(&e); err != nil {
		t.Fatal(err)
	}
	return e
}

func attribute(value int, observed string) map[string]any {
	return map[string]any{"type": "Property", "value": json.Number(fmt.Sprint(value)), "observedAt": observed, "unitCode": "CEL",
		"createdAt": created, "modifiedAt": written, "instanceId": fmt.Sprintf("urn:instance:%d", value)}
}

func writeAttribute(value int, observed string) map[string]any {
	attr := attribute(value, observed)
	delete(attr, "createdAt")
	delete(attr, "modifiedAt")
	delete(attr, "instanceId")
	return attr
}

func testConfig(snapshot, temporal string) *config.Config {
	return &config.Config{Tenants: []config.Tenant{{ID: "town-a", ContextSources: []config.ContextSourceConfig{{
		Endpoint: snapshot, Temporal: config.TemporalInfo{Enabled: true, Endpoint: temporal},
		Information: []config.RegistrationInfo{{Entities: []config.EntityInfo{{Type: "WeatherObserved", IDPattern: "^urn:ngsi-ld:WeatherObserved:.+"}}}},
	}}}}}
}

func testJob(apply bool) *job {
	return &job{options: options{apply: apply, pageSize: 1, contextURL: defaultContext, ngsiTenant: "town-a"},
		client: &http.Client{Timeout: time.Second}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestReconcileHTTP(t *testing.T) {
	for _, mode := range []string{"dry-run", "apply", "apply-singleton", "changed", "removed", "partial-patch", "history-error", "verify-error"} {
		t.Run(mode, func(t *testing.T) {
			current := entity{"id": testID, "type": "WeatherObserved", "createdAt": created, "temperature": attribute(18, older), "humidity": attribute(60, newer)}
			// A compound value and nested subproperty must survive reconstruction.
			latest := attribute(21, newer)
			latest["instanceId"] = "urn:history:instance"
			latest["modifiedAt"] = "2026-01-02T00:00:00Z"
			latest["quality"] = []any{map[string]any{"type": "Property", "value": map[string]any{"codes": []any{json.Number("1"), json.Number("2")}}, "instanceId": "urn:nested"}}
			latest["confidence"] = []any{map[string]any{"type": "Property", "value": "high", "instanceId": "urn:confidence"}}
			patches, reads, listCalls := 0, 0, 0
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("NGSILD-Tenant") != "town-a" || !strings.Contains(r.Header.Get("Link"), defaultContext) {
					t.Error("missing tenant or JSON-LD context")
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/ngsi-ld/v1/entities":
					listCalls++
					if r.URL.Query().Get("limit") != "1" || r.URL.Query().Get("type") != "WeatherObserved" {
						t.Error("incorrect listing parameters")
					}
					if r.URL.Query().Get("offset") == "0" {
						json.NewEncoder(w).Encode([]entity{current})
					} else {
						io.WriteString(w, "[]")
					}
				case strings.Contains(r.URL.Path, "/temporal/"):
					if mode == "history-error" {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					selection := r.URL.Query().Get("lastN") == "1"
					if selection {
						if r.URL.Query().Get("timeproperty") != "observedAt" {
							t.Error("incorrect selection query")
						}
					} else if r.URL.Query().Get("lastN") != "" || r.URL.Query().Get("timeproperty") != "modifiedAt" || r.URL.Query().Get("timerel") != "between" {
						t.Error("incorrect complete-instance query")
					}
					name := r.URL.Query().Get("attrs")
					value := any(latest)
					if name == "humidity" {
						value = current[name]
					}
					if selection {
						copy := make(map[string]any)
						maps.Copy(copy, value.(map[string]any))
						delete(copy, "confidence") // Mintaka's lastN=1 truncates metadata.
						value = copy
						w.WriteHeader(http.StatusPartialContent)
					}
					if mode == "apply-singleton" {
						json.NewEncoder(w).Encode(entity{"id": testID, "type": "WeatherObserved", name: value})
					} else {
						json.NewEncoder(w).Encode(entity{"id": testID, "type": "WeatherObserved", name: []any{value}})
					}
				case r.Method == http.MethodPatch:
					patches++
					if r.Header.Get("Content-Type") != "application/json" {
						t.Error("incorrect patch content type")
					}
					var fragment entity
					d := json.NewDecoder(r.Body)
					d.UseNumber()
					if err := d.Decode(&fragment); err != nil {
						t.Fatal(err)
					}
					if len(fragment) != 1 || fragment["temperature"] == nil {
						t.Errorf("unexpected patch: %v", fragment)
					}
					expected, _, err := observedAttribute([]any{latest}, true)
					if err != nil || !equalJSON(expected, fragment["temperature"]) {
						t.Errorf("metadata lost or historical fields leaked: %v", fragment)
					}
					if mode == "partial-patch" {
						w.WriteHeader(http.StatusMultiStatus)
						return
					}
					current["temperature"] = fragment["temperature"]
					current["temperature"].(map[string]any)["createdAt"] = created
					current["temperature"].(map[string]any)["modifiedAt"] = written
					w.WriteHeader(http.StatusNoContent)
				case r.Method == http.MethodGet && r.URL.Path == "/ngsi-ld/v1/entities/"+testID:
					reads++
					if mode == "removed" {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					if mode == "changed" {
						current["temperature"] = attribute(25, "2026-01-01T12:00:00Z")
					}
					if mode == "verify-error" && reads > 1 {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					json.NewEncoder(w).Encode(current)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusBadRequest)
				}
			})
			server := httptest.NewServer(handler)
			defer server.Close()
			j := testJob(mode != "dry-run")
			err := j.run(context.Background(), testConfig(server.URL, server.URL))
			wantError := mode == "partial-patch" || mode == "history-error" || mode == "verify-error"
			if (err != nil) != wantError {
				t.Fatalf("error=%v, wantError=%v", err, wantError)
			}
			wantPatches := 0
			if mode == "apply" || mode == "apply-singleton" || mode == "partial-patch" || mode == "verify-error" {
				wantPatches = 1
			}
			if patches != wantPatches || listCalls != 2 {
				t.Fatalf("patches=%d listing calls=%d", patches, listCalls)
			}
			if mode == "apply" || mode == "apply-singleton" {
				if j.stats.Corrected != 1 {
					t.Fatalf("summary=%+v", j.stats)
				}
				if err := j.run(context.Background(), testConfig(server.URL, server.URL)); err != nil || patches != 1 {
					t.Fatalf("second run must not write: %v patches=%d", err, patches)
				}
			}
		})
	}
}

func TestUnsupportedAndConflictingAttributes(t *testing.T) {
	for _, raw := range []string{
		`{"type":"Property","value":18}`,
		`{"type":"Property","value":18,"observedAt":"invalid"}`,
		`[{"type":"Property","value":18,"observedAt":"2026-01-01T10:00:00Z"}]`,
		`{"type":"Property","value":18,"observedAt":"2026-01-01T10:00:00Z","datasetId":"urn:dataset"}`,
		`{"type":"Property","value":18,"observedAt":"2026-01-01T10:00:00Z","quality":[{"type":"Property","value":1},{"type":"Property","value":2}]}`,
	} {
		var v any
		json.Unmarshal([]byte(raw), &v)
		if _, _, err := observedAttribute(v, false); err == nil {
			t.Errorf("unsafe attribute accepted: %s", raw)
		}
	}
	for _, scenario := range []string{"equal", "older-history", "missing-history", "ambiguous-history"} {
		t.Run(scenario, func(t *testing.T) {
			current := entity{"id": testID, "type": "WeatherObserved", "createdAt": created, "temperature": attribute(18, newer)}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.Contains(r.URL.Path, "/temporal/") {
					t.Error("unexpected snapshot request or write")
				}
				if scenario == "missing-history" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				value := attribute(21, newer)
				if scenario == "older-history" {
					value = attribute(21, older)
				}
				instances := []any{value}
				if scenario == "ambiguous-history" {
					instances = append(instances, value)
				}
				json.NewEncoder(w).Encode(entity{"id": testID, "type": "WeatherObserved", "temperature": instances})
			}))
			defer server.Close()
			cfg := testConfig(server.URL, server.URL)
			src := cfg.Tenants[0].ContextSources[0]
			j := testJob(true)
			if err := j.reconcile(context.Background(), scope{tenant: "town-a", source: src}, testID, current); err != nil {
				t.Fatal(err)
			}
			if j.stats.Behind != 0 || j.stats.Corrected != 0 {
				t.Fatalf("unexpected correction: %+v", j.stats)
			}
		})
	}
}

func TestRequestCancellationAndValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	j := testJob(false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := j.request(ctx, "town-a", http.MethodGet, server.URL, "/wait", nil, nil, new(entity))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation not propagated: %v", err)
	}
	for _, raw := range []string{"ftp://example.org", "http://user:secret@example.org", "http://example.org?token=secret", "relative"} {
		if validateURL(raw) == nil {
			t.Errorf("accepted invalid endpoint %q", raw)
		}
	}
	for _, body := range []string{`{} {}`, `{"id":`, strings.Repeat(" ", maxResponseBytes+1)} {
		t.Run(fmt.Sprint(len(body)), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }))
			defer s.Close()
			if err := j.request(context.Background(), "default", http.MethodGet, s.URL, "/", nil, nil, new(entity)); err == nil {
				t.Error("accepted malformed/oversized response")
			}
		})
	}
}

func TestScopeValidationAndFilters(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("NGSILD-Tenant") != "" {
			t.Error("default tenant must use tenantless DB")
		}
		if r.URL.Query().Get("id") != testID {
			t.Error("missing ID filter")
		}
		io.WriteString(w, "[]")
	}))
	defer server.Close()
	cfg := testConfig(server.URL, server.URL)
	cfg.Tenants[0].ID = "default"
	j := testJob(false)
	j.options.tenant, j.options.entityType, j.options.entityID = "default", "WeatherObserved", testID
	j.options.ngsiTenant = ""
	if err := j.run(context.Background(), cfg); err != nil || calls != 1 {
		t.Fatalf("filtered run: %v calls=%d", err, calls)
	}
	// Federation tenant IDs must not implicitly select physical broker DBs.
	cfg.Tenants[0].ID, j.options.tenant = "town-a", "town-a"
	if err := j.run(context.Background(), cfg); err != nil || calls != 2 {
		t.Fatalf("logical tenant run: %v calls=%d", err, calls)
	}
	j.options.tenant = "unknown"
	if err := j.run(context.Background(), cfg); err == nil || calls != 2 {
		t.Fatal("unmatched scope should fail without HTTP calls")
	}
	j.options.tenant = "town-a"
	cfg.Tenants[0].ContextSources[0].Temporal.Enabled = false
	if err := j.run(context.Background(), cfg); err == nil {
		t.Fatal("disabled temporal source accepted")
	}
}

func TestJSONNumericAndTimestampEquality(t *testing.T) {
	a := decodeEntity(t, `{"type":"Property","value":21.0,"observedAt":"2026-01-01T11:00:00+01:00"}`)
	b := decodeEntity(t, `{"type":"Property","value":2.1e1,"observedAt":"2026-01-01T10:00:00Z","instanceId":"urn:history"}`)
	x, _, err := observedAttribute(map[string]any(a), false)
	if err != nil {
		t.Fatal(err)
	}
	y, _, err := observedAttribute([]any{map[string]any(b)}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !equalJSON(x, y) {
		t.Fatal("equivalent values/timestamps must compare equal")
	}
}

func TestCompleteHistoryAndLifecycleGuards(t *testing.T) {
	for _, mode := range []string{"complete", "partial", "subattribute-cap", "missing-instance", "changed-instance", "duplicate-instance", "old-entity-life", "old-attribute-life", "creation-boundary", "missing-write-time", "missing-creation-time", "entity-recreated-before-patch", "attribute-recreated-before-patch"} {
		t.Run(mode, func(t *testing.T) {
			current := entity{"id": testID, "type": "WeatherObserved", "createdAt": created, "temperature": attribute(18, older)}
			latest := attribute(21, newer)
			latest["quality"] = map[string]any{"type": "Property", "value": "good", "instanceId": "urn:q"}
			latest["confidence"] = map[string]any{"type": "Property", "value": "high", "instanceId": "urn:c"}
			switch mode {
			case "old-entity-life":
				current["createdAt"] = "2026-01-03T00:00:00Z"
			case "old-attribute-life":
				current["temperature"].(map[string]any)["createdAt"] = "2026-01-03T00:00:00Z"
			case "creation-boundary":
				latest["modifiedAt"] = created
			case "missing-write-time":
				delete(latest, "modifiedAt")
			case "missing-creation-time":
				delete(current, "createdAt")
			}
			patches, hydrations := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/temporal/") {
					if r.URL.Query().Get("options") != "sysAttrs" {
						t.Error("missing historical system fields")
					}
					if r.URL.Query().Get("lastN") == "1" {
						copy := make(map[string]any)
						maps.Copy(copy, latest)
						delete(copy, "confidence")
						json.NewEncoder(w).Encode(entity{"id": testID, "type": "WeatherObserved", "temperature": copy})
						return
					}
					hydrations++
					if r.URL.Query().Get("timeproperty") != "modifiedAt" || r.URL.Query().Get("timerel") != "between" || r.URL.Query().Get("timeAt") == "" || r.URL.Query().Get("endTimeAt") == "" {
						t.Error("missing write-time window")
					}
					if mode == "partial" {
						w.WriteHeader(http.StatusPartialContent)
						return
					}
					if mode == "subattribute-cap" {
						delete(latest, "quality")
						delete(latest, "confidence")
						for i := range mintakaInstanceLimit {
							latest[fmt.Sprintf("meta%d", i)] = map[string]any{"type": "Property", "value": "v"}
						}
					}
					if mode == "missing-instance" {
						latest["instanceId"] = "urn:different"
					}
					if mode == "changed-instance" {
						latest["value"] = json.Number("99")
					}
					// Include another write in the same millisecond, in front of the
					// selected instance. Selection must use identity, not array order.
					instances := []any{attribute(22, newer), latest}
					if mode == "duplicate-instance" {
						instances = append(instances, latest)
					}
					json.NewEncoder(w).Encode(entity{"id": testID, "type": "WeatherObserved", "temperature": instances})
					return
				}
				if r.Method == http.MethodPatch {
					patches++
					var patch entity
					decoder := json.NewDecoder(r.Body)
					decoder.UseNumber()
					if err := decoder.Decode(&patch); err != nil {
						t.Fatal(err)
					}
					current["temperature"] = patch["temperature"]
					current["temperature"].(map[string]any)["createdAt"] = created
					w.WriteHeader(http.StatusNoContent)
					return
				}
				if r.URL.Query().Get("options") != "sysAttrs" {
					t.Error("missing snapshot lifecycle fields")
				}
				data, _ := json.Marshal(current)
				response := decodeEntity(t, string(data))
				if mode == "entity-recreated-before-patch" {
					response["createdAt"] = "2026-01-03T00:00:00Z"
				}
				if mode == "attribute-recreated-before-patch" {
					response["temperature"].(map[string]any)["createdAt"] = "2026-01-03T00:00:00Z"
				}
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			j := testJob(true)
			cfg := testConfig(server.URL, server.URL)
			s := scope{tenant: "town-a", source: cfg.Tenants[0].ContextSources[0]}
			if err := j.reconcile(context.Background(), s, testID, current); err != nil {
				t.Fatal(err)
			}
			if mode == "complete" {
				if patches != 1 || hydrations != 1 || j.stats.Corrected != 1 {
					t.Fatalf("patches=%d hydrations=%d summary=%+v", patches, hydrations, j.stats)
				}
				attr := current["temperature"].(map[string]any)
				if attr["quality"] == nil || attr["confidence"] == nil {
					t.Fatal("complete subattributes not preserved")
				}
			} else if patches != 0 || j.stats.Skipped == 0 {
				t.Fatalf("unsafe history/lifecycle was not skipped: patches=%d summary=%+v", patches, j.stats)
			}
		})
	}
}

// Run against isolated Orion-LD 1.12.0 + TRoE + Mintaka 0.7.0 services.
// The test creates a unique entity, restores it, checks history and deletes it.
func TestBackendIntegration(t *testing.T) {
	orion, mintaka := os.Getenv("RECONCILER_TEST_ORION_URL"), os.Getenv("RECONCILER_TEST_MINTAKA_URL")
	if orion == "" || mintaka == "" {
		t.Skip("set RECONCILER_TEST_ORION_URL and RECONCILER_TEST_MINTAKA_URL for real backend integration")
	}
	for _, tenant := range []string{"default", fmt.Sprintf("reconciler%d", time.Now().UnixNano())} {
		t.Run(tenant, func(t *testing.T) {
			t.Run("complete-repair", func(t *testing.T) { testBackendIntegration(t, orion, mintaka, tenant) })
			for _, kind := range []string{"entity", "attribute", "metadata-limit", "creation-boundary"} {
				t.Run(kind, func(t *testing.T) { testBackendLifecycle(t, orion, mintaka, tenant, kind) })
			}
		})
	}
}

func testBackendLifecycle(t *testing.T, orion, mintaka, tenant, kind string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	id := fmt.Sprintf("urn:ngsi-ld:WeatherObserved:lifecycle-%d", time.Now().UnixNano())
	j := testJob(true)
	var logs bytes.Buffer
	j.log = slog.New(slog.NewTextHandler(&logs, nil))
	j.options.tenant, j.options.ngsiTenant, j.options.entityID = tenant, tenant, id
	j.client.Timeout = 20 * time.Second
	call := func(method, path string, body any) {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(data)
		}
		req, err := http.NewRequestWithContext(ctx, method, orion+path, reader)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Link", fmt.Sprintf(`<%s>; rel="http://www.w3.org/ns/json-ld#context"; type="application/ld+json"`, defaultContext))
		if tenant != "default" {
			req.Header.Set("NGSILD-Tenant", tenant)
		}
		response, err := j.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusNoContent {
			t.Fatalf("%s %s: HTTP %d", method, path, response.StatusCode)
		}
	}
	initial := writeAttribute(0, "2026-01-01T09:00:00Z")
	if kind == "creation-boundary" {
		initial = writeAttribute(21, newer)
	}
	call(http.MethodPost, "/ngsi-ld/v1/entities", entity{"id": id, "type": "WeatherObserved", "temperature": initial})
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		req, _ := http.NewRequestWithContext(cleanupCtx, http.MethodDelete, orion+entityPath(id, false), nil)
		if tenant != "default" {
			req.Header.Set("NGSILD-Tenant", tenant)
		}
		response, err := j.client.Do(req)
		if err != nil {
			t.Errorf("cleanup: %v", err)
			return
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			t.Errorf("cleanup: HTTP %d", response.StatusCode)
		}
	})
	time.Sleep(5 * time.Millisecond)
	latest := writeAttribute(21, newer)
	if kind == "metadata-limit" {
		for i := range mintakaInstanceLimit + 1 {
			latest[fmt.Sprintf("meta%d", i)] = map[string]any{"type": "Property", "value": "v"}
		}
	}
	if kind != "creation-boundary" {
		call(http.MethodPatch, entityPath(id, false)+"/attrs", entity{"temperature": latest})
	}
	time.Sleep(5 * time.Millisecond)
	if kind == "entity" {
		call(http.MethodDelete, entityPath(id, false), nil)
		call(http.MethodPost, "/ngsi-ld/v1/entities", entity{"id": id, "type": "WeatherObserved", "temperature": writeAttribute(18, older)})
	} else if kind == "attribute" {
		call(http.MethodDelete, entityPath(id, false)+"/attrs/temperature", nil)
		call(http.MethodPost, entityPath(id, false)+"/attrs", entity{"temperature": writeAttribute(18, older)})
	} else {
		call(http.MethodPatch, entityPath(id, false)+"/attrs", entity{"temperature": writeAttribute(18, older)})
	}
	var history entity
	q := url.Values{"attrs": {"temperature"}, "lastN": {"1"}, "timeproperty": {"observedAt"}, "options": {"sysAttrs"}}
	if err := j.request(ctx, tenant, http.MethodGet, mintaka, entityPath(id, true), q, nil, &history); err != nil {
		t.Fatal(err)
	}
	selected, err := singleInstance(history["temperature"])
	if err != nil || !equalJSON(selected["value"], json.Number("21")) {
		t.Fatal("fixture did not expose the previous lifecycle")
	}
	var lifecycle entity
	if err := j.request(ctx, tenant, http.MethodGet, orion, entityPath(id, false), url.Values{"options": {"sysAttrs"}}, nil, &lifecycle); err != nil {
		t.Fatal(err)
	}
	entityCreated, eerr := systemTime(lifecycle, "createdAt")
	attrCreated, aerr := systemTime(lifecycle["temperature"].(map[string]any), "createdAt")
	selectedWrite, werr := systemTime(selected, "modifiedAt")
	if eerr != nil || aerr != nil || werr != nil {
		t.Fatal("fixture lacks lifecycle timestamps")
	}
	if kind == "metadata-limit" && (!selectedWrite.After(entityCreated) || !selectedWrite.After(attrCreated)) {
		t.Fatal("metadata-limit fixture must be eligible for lifecycle validation")
	}
	cfg := testConfig(orion, mintaka)
	cfg.Tenants[0].ID = tenant
	if err := j.run(ctx, cfg); err != nil || j.stats.Corrected != 0 || j.stats.Skipped != 1 {
		t.Fatalf("recreated %s must be skipped: %v summary=%+v", kind, err, j.stats)
	}
	expectedReason := "history predates the current lifecycle or is at its ambiguous creation boundary"
	if kind == "metadata-limit" {
		expectedReason = "subattribute limit reached"
	}
	if !strings.Contains(logs.String(), expectedReason) {
		t.Fatalf("unexpected skip reason: %s", logs.String())
	}
	var current entity
	if err := j.request(ctx, tenant, http.MethodGet, orion, entityPath(id, false), nil, nil, &current); err != nil {
		t.Fatal(err)
	}
	actual, _, err := observedAttribute(current["temperature"], false)
	if err != nil || !equalJSON(actual, writeAttribute(18, older)) {
		t.Fatalf("old %s lifecycle was resurrected", kind)
	}
}

func testBackendIntegration(t *testing.T, orion, mintaka, tenant string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	id := fmt.Sprintf("urn:ngsi-ld:WeatherObserved:reconciler-%d", time.Now().UnixNano())
	j := testJob(true)
	j.log = slog.New(slog.NewTextHandler(os.Stderr, nil))
	j.options.tenant, j.options.ngsiTenant = tenant, tenant
	j.client.Timeout = 20 * time.Second
	call := func(method, path string, body any) {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(orion, "/")+path, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Link", fmt.Sprintf(`<%s>; rel="http://www.w3.org/ns/json-ld#context"; type="application/ld+json"`, defaultContext))
		if tenant != "default" {
			req.Header.Set("NGSILD-Tenant", tenant)
		}
		resp, err := j.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			t.Fatalf("%s returned %d: %s", method, resp.StatusCode, b)
		}
	}
	latest := writeAttribute(21, newer)
	latest["quality"] = map[string]any{"type": "Property", "value": "verified"}
	latest["confidence"] = map[string]any{"type": "Property", "value": "high"}
	call(http.MethodPost, "/ngsi-ld/v1/entities", entity{"id": id, "type": "WeatherObserved", "temperature": writeAttribute(0, "2026-01-01T09:00:00Z"), "humidity": writeAttribute(60, newer)})
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		req, _ := http.NewRequestWithContext(cleanupCtx, http.MethodDelete, orion+entityPath(id, false), nil)
		if tenant != "default" {
			req.Header.Set("NGSILD-Tenant", tenant)
		}
		resp, err := j.client.Do(req)
		if err != nil {
			t.Errorf("cleanup: %v", err)
			return
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("cleanup status %d", resp.StatusCode)
		}
	})
	// Ensure the valid candidate is strictly after the millisecond creation
	// boundary; the job deliberately cannot disambiguate writes at that boundary.
	time.Sleep(5 * time.Millisecond)
	call(http.MethodPatch, entityPath(id, false)+"/attrs", entity{"temperature": latest})
	call(http.MethodPatch, entityPath(id, false)+"/attrs", entity{"temperature": writeAttribute(18, older)})
	cfg := testConfig(orion, mintaka)
	cfg.Tenants[0].ID = tenant
	j.options.entityID = id
	// Wait until Mintaka sees both writes before assessing the job.
	var history entity
	q := url.Values{"attrs": {"temperature"}, "lastN": {"10"}, "timeproperty": {"observedAt"}}
	for {
		err := j.request(ctx, tenant, http.MethodGet, mintaka, entityPath(id, true), q, nil, &history)
		instances, _ := history["temperature"].([]any)
		if err == nil && len(instances) >= 3 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("history not ready: %v", err)
		case <-time.After(500 * time.Millisecond):
		}
	}
	j.options.apply = false
	if err := j.run(ctx, cfg); err != nil || j.stats.Behind != 1 || j.stats.Corrected != 0 {
		t.Fatalf("dry-run: %v summary=%+v", err, j.stats)
	}
	var snapshot entity
	if err := j.request(ctx, tenant, http.MethodGet, orion, entityPath(id, false), url.Values{"options": {"sysAttrs"}}, nil, &snapshot); err != nil {
		t.Fatal(err)
	}
	old, _, err := observedAttribute(snapshot["temperature"], false)
	if err != nil || old["observedAt"] != older {
		t.Fatal("dry-run changed the snapshot")
	}
	j.stats = summary{}
	j.options.apply = true
	if err := j.run(ctx, cfg); err != nil || j.stats.Corrected != 1 {
		t.Fatalf("apply: %v summary=%+v", err, j.stats)
	}
	if err := j.request(ctx, tenant, http.MethodGet, orion, entityPath(id, false), url.Values{"options": {"sysAttrs"}}, nil, &snapshot); err != nil {
		t.Fatal(err)
	}
	repaired, _, err := observedAttribute(snapshot["temperature"], false)
	expected, _, expectedErr := observedAttribute(latest, false)
	if err != nil || expectedErr != nil || !equalJSON(repaired, expected) {
		t.Fatal("repair lost value, observation time, unit or subattribute")
	}
	humidity, _, err := observedAttribute(snapshot["humidity"], false)
	if err != nil || !equalJSON(humidity, writeAttribute(60, newer)) {
		t.Fatal("repair changed a current attribute")
	}
	j.stats = summary{}
	if err := j.run(ctx, cfg); err != nil || j.stats.Behind != 0 || j.stats.Corrected != 0 {
		t.Fatalf("second run: %v summary=%+v", err, j.stats)
	}
	if err := j.request(ctx, tenant, http.MethodGet, mintaka, entityPath(id, true), q, nil, &history); err != nil {
		t.Fatal(err)
	}
	instances, _ := history["temperature"].([]any)
	if len(instances) != 4 {
		t.Fatalf("expected baseline + observation + replay + repair, got %d instances", len(instances))
	}
	latestCount := 0
	for _, v := range instances {
		a, ts, err := observedAttribute(v, true)
		if err != nil {
			t.Fatal(err)
		}
		if ts.Format(time.RFC3339Nano) == newer {
			if !equalJSON(a, expected) {
				t.Fatal("historical repair did not preserve the original observation")
			}
			latestCount++
		}
	}
	if latestCount != 2 {
		t.Fatalf("expected two latest observation instances, got %d", latestCount)
	}
}
