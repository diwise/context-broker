// snapshot-reconciler repairs current Orion-LD attributes from Mintaka history.
// All job-specific code intentionally lives in this file.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/diwise/context-broker/internal/pkg/application/config"
	"github.com/diwise/service-chassis/pkg/infrastructure/buildinfo"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const defaultContext = "https://raw.githubusercontent.com/diwise/context-broker/main/assets/jsonldcontexts/default-context.jsonld"
const maxResponseBytes = 8 << 20

// Mintaka 0.7.0 limits both attribute and subattribute queries to 1000
// instances for a single explicitly selected attribute without lastN.
const mintakaInstanceLimit = 1000

type options struct {
	configPath, tenant, entityType, entityID, contextURL string
	ngsiTenant                                           string
	apply                                                bool
	pageSize                                             int
	requestTimeout, timeout                              time.Duration
}

type summary struct {
	Entities, Behind, Corrected, Skipped, Errors int
}

type job struct {
	options options
	client  *http.Client
	log     *slog.Logger
	stats   summary
}

type entity map[string]any

func main() {
	if err := execute(); err != nil {
		slog.Error("snapshot reconciliation failed", "err", err)
		os.Exit(1)
	}
}

func execute() error {
	var opts options
	flag.StringVar(&opts.configPath, "config", "/opt/diwise/config/default.yaml", "Context-broker configuration file")
	flag.StringVar(&opts.tenant, "tenant", "", "Restrict to a configured tenant ID")
	flag.StringVar(&opts.ngsiTenant, "ngsild-tenant", "", "Explicit physical Orion/Mintaka tenant; requires -tenant")
	flag.StringVar(&opts.entityType, "type", "", "Restrict to a configured entity type")
	flag.StringVar(&opts.entityID, "id", "", "Restrict to one entity ID")
	flag.StringVar(&opts.contextURL, "context", defaultContext, "JSON-LD context URL used for both APIs")
	flag.BoolVar(&opts.apply, "apply", false, "Apply repairs; otherwise only report discrepancies")
	flag.IntVar(&opts.pageSize, "page-size", 100, "Orion entities per page (1-1000)")
	flag.DurationVar(&opts.requestTimeout, "request-timeout", 30*time.Second, "Timeout per HTTP request")
	flag.DurationVar(&opts.timeout, "timeout", 30*time.Minute, "Deadline for the entire job")
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if opts.pageSize < 1 || opts.pageSize > 1000 || opts.requestTimeout <= 0 || opts.timeout <= 0 {
		return errors.New("page-size must be 1-1000 and timeouts must be positive")
	}
	if opts.ngsiTenant != "" && opts.tenant == "" {
		return errors.New("ngsild-tenant requires an explicit configured tenant filter")
	}
	if err := validateURL(opts.contextURL); err != nil {
		return fmt.Errorf("context URL: %w", err)
	}
	f, err := os.Open(opts.configPath)
	if err != nil {
		return fmt.Errorf("open configuration: %w", err)
	}
	cfg, err := config.Load(f)
	closeErr := f.Close()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close configuration: %w", closeErr)
	}
	root, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, log, cleanup := o11y.Init(root, "snapshot-reconciler", buildinfo.SourceVersion(), "json")
	defer cleanup()
	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()
	client := &http.Client{
		Timeout:   opts.requestTimeout,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		// Do not forward tenant information or repairs to redirect destinations.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	j := &job{options: opts, client: client, log: log}
	return j.run(ctx, cfg)
}

func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("expected an absolute HTTP(S) URL without credentials, query or fragment")
	}
	return nil
}

type scope struct {
	tenant  string
	source  config.ContextSourceConfig
	info    config.EntityInfo
	pattern *regexp.Regexp
}

func (j *job) run(ctx context.Context, cfg *config.Config) error {
	defer func() {
		j.log.Info("snapshot reconciliation complete", "apply", j.options.apply,
			"entities", j.stats.Entities, "behind", j.stats.Behind, "corrected", j.stats.Corrected,
			"skipped", j.stats.Skipped, "errors", j.stats.Errors)
	}()
	// Validate every selected scope before performing any writes.
	var scopes []scope
	for _, tenant := range cfg.Tenants {
		if j.options.tenant != "" && tenant.ID != j.options.tenant {
			continue
		}
		for _, src := range tenant.ContextSources {
			if !src.Temporal.Enabled {
				continue
			}
			for _, registration := range src.Information {
				for _, info := range registration.Entities {
					if j.options.entityType != "" && info.Type != j.options.entityType {
						continue
					}
					pattern, err := regexp.CompilePOSIX(info.IDPattern)
					if err != nil || info.IDPattern == "" || info.Type == "" {
						return errors.New("selected entity registration requires a type and valid nonempty ID pattern")
					}
					if j.options.entityID != "" && !pattern.MatchString(j.options.entityID) {
						continue
					}
					for _, endpoint := range []string{src.Endpoint, src.TemporalEndpoint()} {
						if err := validateURL(endpoint); err != nil {
							return fmt.Errorf("invalid endpoint for tenant %q: %w", tenant.ID, err)
						}
					}
					scopes = append(scopes, scope{tenant.ID, src, info, pattern})
				}
			}
		}
	}
	if len(scopes) == 0 {
		return errors.New("no matching registrations with temporal.enabled=true")
	}
	seen := make(map[string]bool)
	for _, s := range scopes {
		if err := j.scan(ctx, s, seen); err != nil {
			j.stats.Errors++
			j.log.Error("scope failed", "tenant", s.tenant, "type", s.info.Type, "err", err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if j.stats.Errors > 0 {
		return fmt.Errorf("reconciliation encountered %d errors", j.stats.Errors)
	}
	return nil
}

func (j *job) scan(ctx context.Context, s scope, seen map[string]bool) error {
	previousPage := ""
	for offset := 0; ; offset += j.options.pageSize {
		q := url.Values{"type": {s.info.Type}, "limit": {fmt.Sprint(j.options.pageSize)}, "offset": {fmt.Sprint(offset)}, "options": {"sysAttrs"}}
		if j.options.entityID != "" {
			q.Set("id", j.options.entityID)
		}
		var page []entity
		if err := j.request(ctx, j.options.ngsiTenant, http.MethodGet, s.source.Endpoint, "/ngsi-ld/v1/entities", q, nil, &page); err != nil {
			return fmt.Errorf("list snapshot entities: %w", err)
		}
		if page == nil {
			return errors.New("snapshot listing must be a JSON array, not null")
		}
		pageIDs := make([]string, 0, len(page))
		for _, current := range page {
			id, _ := current["id"].(string)
			kind, _ := current["type"].(string)
			if id == "" || kind == "" {
				return errors.New("snapshot entity has no id or type")
			}
			pageIDs = append(pageIDs, id)
			if kind != s.info.Type || !s.pattern.MatchString(id) || (j.options.entityID != "" && id != j.options.entityID) {
				continue
			}
			key := s.tenant + "\x00" + s.source.Endpoint + "\x00" + id
			if seen[key] {
				continue
			}
			seen[key] = true
			j.stats.Entities++
			if err := j.reconcile(ctx, s, id, current); err != nil {
				j.stats.Errors++
				j.log.Error("entity failed", "tenant", s.tenant, "entity_id", id, "err", err)
			}
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if len(page) < j.options.pageSize || j.options.entityID != "" {
			return nil
		}
		slices.Sort(pageIDs)
		signature := strings.Join(pageIDs, "\x00")
		if signature == previousPage {
			return errors.New("listing repeated a page; check pagination")
		}
		previousPage = signature
	}
}

func (j *job) reconcile(ctx context.Context, s scope, id string, current entity) error {
	fragment := entity{}
	names := make([]string, 0, len(current))
	for name := range current {
		if !systemField(name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		snapshot, snapshotTime, err := observedAttribute(current[name], false)
		if err != nil {
			j.skip(s, id, name, err.Error())
			continue
		}
		entityCreated, entityErr := systemTime(current, "createdAt")
		currentRaw, _ := current[name].(map[string]any)
		attributeCreated, attributeErr := systemTime(currentRaw, "createdAt")
		if entityErr != nil || attributeErr != nil {
			j.skip(s, id, name, "missing or invalid lifecycle timestamps in snapshot")
			continue
		}
		var history entity
		q := url.Values{"attrs": {name}, "lastN": {"1"}, "timeproperty": {"observedAt"}, "options": {"sysAttrs"}}
		err = j.request(ctx, j.options.ngsiTenant, http.MethodGet, s.source.TemporalEndpoint(), entityPath(id, true), q, nil, &history)
		if errors.Is(err, errNotFound) {
			j.skip(s, id, name, "no history")
			continue
		}
		if err != nil {
			return fmt.Errorf("read history for attribute %q: %w", name, err)
		}
		if history["id"] != id || history["type"] != current["type"] {
			return errors.New("history entity identity or type differs from snapshot")
		}
		candidate, err := singleInstance(history[name])
		if err != nil {
			j.skip(s, id, name, "history: "+err.Error())
			continue
		}
		// lastN=1 truncates subattributes too. Use this response only to select
		// an instance, never as a repair payload.
		latestTime, err := systemTime(candidate, "observedAt")
		if err != nil {
			j.skip(s, id, name, "history: "+err.Error())
			continue
		}
		if candidate["type"] != snapshot["type"] {
			j.skip(s, id, name, "attribute type differs")
			continue
		}
		if latestTime.Before(snapshotTime) {
			continue
		}
		written, err := systemTime(candidate, "modifiedAt")
		// Orion and Mintaka expose these system timestamps with millisecond
		// precision. A write at the creation boundary cannot prove which
		// lifecycle it belongs to; defer it rather than resurrect old data.
		if err != nil || !written.After(entityCreated) || !written.After(attributeCreated) {
			j.skip(s, id, name, "history predates the current lifecycle or is at its ambiguous creation boundary")
			continue
		}
		latest, err := j.completeInstance(ctx, s, id, name, current["type"], candidate, written)
		if errors.Is(err, errIncompleteHistory) {
			j.skip(s, id, name, err.Error())
			continue
		}
		if err != nil {
			return fmt.Errorf("read complete history for attribute %q: %w", name, err)
		}
		if latestTime.Equal(snapshotTime) && !equalJSON(snapshot, latest) {
			j.skip(s, id, name, "different contents at equal observedAt")
			continue
		}
		if !latestTime.After(snapshotTime) {
			continue
		}
		j.stats.Behind++
		j.log.Info("snapshot attribute behind history", "tenant", s.tenant, "entity_id", id,
			"attribute", name, "snapshot_observed_at", snapshotTime, "history_observed_at", latestTime)
		fragment[name] = latest
	}
	if len(fragment) == 0 || !j.options.apply {
		return nil
	}
	var fresh entity
	if err := j.request(ctx, j.options.ngsiTenant, http.MethodGet, s.source.Endpoint, entityPath(id, false), url.Values{"options": {"sysAttrs"}}, nil, &fresh); err != nil {
		if errors.Is(err, errNotFound) {
			j.skip(s, id, "", "entity removed before repair")
			return nil
		}
		return fmt.Errorf("reread snapshot: %w", err)
	}
	if fresh["id"] != id || fresh["type"] != current["type"] {
		return errors.New("snapshot identity or type changed before repair")
	}
	if !sameCreation(current, fresh) {
		j.skip(s, id, "", "entity lifecycle changed before repair")
		return nil
	}
	for name, value := range fragment {
		before, _, beforeErr := observedAttribute(current[name], false)
		now, _, nowErr := observedAttribute(fresh[name], false)
		// Any intervening change defers this attribute to a later run.
		beforeRaw, _ := current[name].(map[string]any)
		nowRaw, _ := fresh[name].(map[string]any)
		if beforeErr != nil || nowErr != nil || !equalJSON(before, now) || !sameCreation(beforeRaw, nowRaw) {
			delete(fragment, name)
			j.skip(s, id, name, "snapshot changed before repair")
			continue
		}
		fragment[name] = value
	}
	if len(fragment) == 0 {
		return nil
	}
	if err := j.request(ctx, j.options.ngsiTenant, http.MethodPatch, s.source.Endpoint, entityPath(id, false)+"/attrs", nil, fragment, nil); err != nil {
		return fmt.Errorf("repair snapshot: %w", err)
	}
	var verified entity
	if err := j.request(ctx, j.options.ngsiTenant, http.MethodGet, s.source.Endpoint, entityPath(id, false), url.Values{"options": {"sysAttrs"}}, nil, &verified); err != nil {
		return fmt.Errorf("verify snapshot: %w", err)
	}
	if verified["id"] != id || verified["type"] != current["type"] || !sameCreation(current, verified) {
		return errors.New("snapshot identity or type changed during verification")
	}
	for name, expected := range fragment {
		actual, _, err := observedAttribute(verified[name], false)
		initialRaw, _ := current[name].(map[string]any)
		verifiedRaw, _ := verified[name].(map[string]any)
		if err != nil || !equalJSON(actual, expected) || !sameCreation(initialRaw, verifiedRaw) {
			return fmt.Errorf("attribute %q did not match repair on readback", name)
		}
		j.stats.Corrected++
		j.log.Info("snapshot attribute corrected", "tenant", s.tenant, "entity_id", id, "attribute", name)
	}
	return nil
}

func systemTime(object map[string]any, name string) (time.Time, error) {
	text, _ := object[name].(string)
	ts, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("missing or invalid %s", name)
	}
	return ts, nil
}

func sameCreation(a, b map[string]any) bool {
	at, aerr := systemTime(a, "createdAt")
	bt, berr := systemTime(b, "createdAt")
	return aerr == nil && berr == nil && at.Equal(bt)
}

func singleInstance(value any) (map[string]any, error) {
	if values, ok := value.([]any); ok {
		if len(values) != 1 {
			return nil, errors.New("expected exactly one history instance")
		}
		value = values[0]
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("expected a history instance object")
	}
	return object, nil
}

var errIncompleteHistory = errors.New("complete historical instance could not be established")

// Re-fetch the selected write without lastN so Mintaka returns its full
// subattribute set. The millisecond system timestamp may cover several writes;
// instanceId, not array order, identifies the selected observation.
func (j *job) completeInstance(ctx context.Context, s scope, id, name string, kind any, candidate map[string]any, written time.Time) (map[string]any, error) {
	instanceID, _ := candidate["instanceId"].(string)
	if instanceID == "" {
		return nil, fmt.Errorf("%w: missing instanceId", errIncompleteHistory)
	}
	q := url.Values{"attrs": {name}, "options": {"sysAttrs"}, "timeproperty": {"modifiedAt"}, "timerel": {"between"},
		"timeAt":    {written.Add(-time.Millisecond).UTC().Format(time.RFC3339Nano)},
		"endTimeAt": {written.Add(time.Millisecond).UTC().Format(time.RFC3339Nano)}}
	var history entity
	if err := j.request(ctx, j.options.ngsiTenant, http.MethodGet, s.source.TemporalEndpoint(), entityPath(id, true), q, nil, &history); err != nil {
		if errors.Is(err, errNotFound) {
			return nil, fmt.Errorf("%w: selected write disappeared", errIncompleteHistory)
		}
		return nil, err
	}
	if history["id"] != id || history["type"] != kind {
		return nil, errors.New("complete history entity identity or type differs from snapshot")
	}
	values, ok := history[name].([]any)
	if !ok {
		values = []any{history[name]}
	}
	if len(values) >= mintakaInstanceLimit {
		return nil, fmt.Errorf("%w: attribute instance limit reached", errIncompleteHistory)
	}
	var selected map[string]any
	for _, value := range values {
		object, ok := value.(map[string]any)
		if ok && object["instanceId"] == instanceID {
			if selected != nil {
				return nil, fmt.Errorf("%w: duplicate instanceId", errIncompleteHistory)
			}
			selected = object
		}
	}
	if selected == nil || !completeSubattributes(selected) {
		return nil, fmt.Errorf("%w: missing instance or subattribute limit reached", errIncompleteHistory)
	}
	selectedWrite, err := systemTime(selected, "modifiedAt")
	if err != nil || !selectedWrite.Equal(written) || !equalJSON(attributeCore(candidate), attributeCore(selected)) {
		return nil, fmt.Errorf("%w: selected write changed", errIncompleteHistory)
	}
	clean, err := cleanAttribute(selected, true)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errIncompleteHistory, err)
	}
	return clean, nil
}

func attributeCore(attr map[string]any) map[string]any {
	core := make(map[string]any)
	for _, name := range []string{"type", "value", "object", "unitCode", "observedAt", "datasetId"} {
		if value, exists := attr[name]; exists {
			core[name] = value
		}
	}
	return core
}

func completeSubattributes(attr map[string]any) bool {
	count := 0
	for name, value := range attr {
		switch name {
		case "type", "value", "object", "unitCode", "datasetId", "observedAt", "createdAt", "modifiedAt", "instanceId", "@context":
			continue
		}
		values, ok := value.([]any)
		if !ok {
			values = []any{value}
		}
		count += len(values)
		for _, v := range values {
			child, ok := v.(map[string]any)
			if !ok || !completeSubattributes(child) {
				return false
			}
		}
	}
	// Mintaka does not flag truncation of subattributes in HTTP status. A
	// result at its known cap is ambiguous even when the response is 200.
	return count < mintakaInstanceLimit
}

func (j *job) skip(s scope, id, name, reason string) {
	j.stats.Skipped++
	j.log.Info("attribute skipped", "tenant", s.tenant, "entity_id", id, "attribute", name, "reason", reason)
}

func entityPath(id string, temporal bool) string {
	prefix := "/ngsi-ld/v1/entities/"
	if temporal {
		prefix = "/ngsi-ld/v1/temporal/entities/"
	}
	return prefix + url.PathEscape(id)
}

var errNotFound = errors.New("not found")

func (j *job) request(ctx context.Context, tenant, method, endpoint, path string, query url.Values, body any, result any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	address := strings.TrimRight(endpoint, "/") + path
	if len(query) > 0 {
		address += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, address, reader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Link", fmt.Sprintf(`<%s>; rel="http://www.w3.org/ns/json-ld#context"; type="application/ld+json"`, j.options.contextURL))
	req.Header.Set("User-Agent", "diwise-snapshot-reconciler")
	// This is the explicit physical tenant, not the federation config's tenant ID.
	if tenant != "default" && tenant != "" {
		req.Header.Set("NGSILD-Tenant", tenant)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if resp.StatusCode == http.StatusPartialContent && strings.Contains(path, "/temporal/") && query.Get("lastN") != "1" {
		return fmt.Errorf("%w: partial temporal response", errIncompleteHistory)
	}
	// Mintaka can return 206 for a lastN query. Only GET history permits it.
	acceptable := method == http.MethodGet && (resp.StatusCode == http.StatusOK ||
		(resp.StatusCode == http.StatusPartialContent && strings.Contains(path, "/temporal/") && query.Get("lastN") == "1"))
	if method == http.MethodPatch {
		acceptable = resp.StatusCode == http.StatusNoContent
	}
	if !acceptable {
		// Do not log response bodies: they may contain measurements or credentials.
		return fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}
	if result == nil {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return errors.New("response exceeds 8 MiB; reduce page-size")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(result); err != nil {
		return errors.New("invalid JSON response")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("response must contain a single JSON document")
	}
	return nil
}

func systemField(name string) bool {
	return name == "id" || name == "type" || name == "@context" || name == "createdAt" || name == "modifiedAt" || name == "instanceId" || name == "scope"
}

func observedAttribute(value any, temporal bool) (map[string]any, time.Time, error) {
	if temporal {
		// JSON-LD compaction collapses singleton arrays in Mintaka 0.7.
		if values, ok := value.([]any); ok {
			if len(values) != 1 {
				return nil, time.Time{}, errors.New("expected exactly one history instance")
			}
			value = values[0]
		}
	}
	clean, err := cleanAttribute(value, temporal)
	if err != nil {
		return nil, time.Time{}, err
	}
	observed, _ := clean["observedAt"].(string)
	ts, err := time.Parse(time.RFC3339Nano, observed)
	if err != nil {
		return nil, time.Time{}, errors.New("missing or invalid observedAt")
	}
	return clean, ts, nil
}

// Mintaka represents subattributes as instance arrays too. Normalize only
// attribute objects; compound Property values and coordinates remain untouched.
func cleanAttribute(value any, temporal bool) (map[string]any, error) {
	attr, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("expected a single attribute object; arrays are unsupported")
	}
	kind, _ := attr["type"].(string)
	valueKey := "value"
	switch kind {
	case "Property", "GeoProperty":
	case "Relationship":
		valueKey = "object"
	default:
		return nil, errors.New("unsupported attribute type")
	}
	if _, ok := attr[valueKey]; !ok {
		return nil, errors.New("attribute has no value/object")
	}
	if _, exists := attr["datasetId"]; exists {
		return nil, errors.New("datasetId attributes are unsupported")
	}
	clean := make(map[string]any, len(attr))
	for name, v := range attr {
		if name == "instanceId" || name == "createdAt" || name == "modifiedAt" || name == "@context" {
			continue
		}
		if name == "observedAt" {
			text, _ := v.(string)
			ts, err := time.Parse(time.RFC3339Nano, text)
			if err != nil {
				return nil, errors.New("invalid observedAt")
			}
			clean[name] = ts.UTC().Format(time.RFC3339Nano)
			continue
		}
		if name == valueKey || name == "type" || name == "unitCode" {
			clean[name] = v
			continue
		}
		if temporal {
			if values, ok := v.([]any); ok && len(values) == 1 {
				v = values[0]
			}
		}
		nested, err := cleanAttribute(v, temporal)
		if err != nil {
			return nil, fmt.Errorf("unsupported subattribute %q", name)
		}
		clean[name] = nested
	}
	return clean, nil
}

func equalJSON(a, b any) bool {
	switch x := a.(type) {
	case json.Number:
		y, ok := b.(json.Number)
		if !ok {
			return false
		}
		xr, xok := new(big.Rat).SetString(string(x))
		yr, yok := new(big.Rat).SetString(string(y))
		return xok && yok && xr.Cmp(yr) == 0
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, exists := y[k]
			if !exists || !equalJSON(v, w) {
				return false
			}
		}
		return true
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !equalJSON(x[i], y[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a, b)
	}
}
