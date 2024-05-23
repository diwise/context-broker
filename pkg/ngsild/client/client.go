package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	log "log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/diwise/context-broker/pkg/ngsild"
	"github.com/diwise/context-broker/pkg/ngsild/errors"
	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/tracing"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

//go:generate moq -rm -out ../../test/contextbrokerclient_mock.go . ContextBrokerClient

type ContextBrokerClient interface {
	CreateEntity(ctx context.Context, entity types.Entity, headers map[string][]string) (*ngsild.CreateEntityResult, error)
	QueryEntities(ctx context.Context, entityTypes, entityAttributes []string, query string, headers map[string][]string) (*ngsild.QueryEntitiesResult, error)
	RetrieveEntity(ctx context.Context, entityID string, headers map[string][]string) (types.Entity, error)
	QueryTemporalEvolutionOfEntities(ctx context.Context, headers map[string][]string, parameters ...RequestDecoratorFunc) (*ngsild.QueryTemporalEntitiesResult, error)
	RetrieveTemporalEvolutionOfEntity(ctx context.Context, entityID string, headers map[string][]string, parameters ...RequestDecoratorFunc) (*ngsild.RetrieveTemporalEvolutionOfEntityResult, error)
	MergeEntity(ctx context.Context, entityID string, fragment types.EntityFragment, headers map[string][]string) (*ngsild.MergeEntityResult, error)
	UpdateEntityAttributes(ctx context.Context, entityID string, fragment types.EntityFragment, headers map[string][]string) (*ngsild.UpdateEntityAttributesResult, error)
	DeleteEntity(ctx context.Context, entityID string) (*ngsild.DeleteEntityResult, error)
}

type RequestDecoratorFunc func([]string) []string

func Debug(enabled string) func(*cbClient) {
	return func(c *cbClient) {
		c.debug = (enabled == "true")
	}
}

func RequestHeader(key string, value []string) func(*cbClient) {
	return func(c *cbClient) {
		c.requestHeaders[http.CanonicalHeaderKey(key)] = value
	}
}

func Tenant(tenant string) func(*cbClient) {
	return func(c *cbClient) {
		c.tenant = tenant
	}
}

func UserAgent(useragent string) func(*cbClient) {
	return RequestHeader("user-agent", []string{useragent})
}

func NewContextBrokerClient(broker string, options ...func(*cbClient)) ContextBrokerClient {

	c := &cbClient{
		baseURL: broker,
		tenant:  entities.DefaultNGSITenant,
		debug:   true,
		httpClient: http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
		requestHeaders: map[string][]string{},
	}

	for _, option := range options {
		option(c)
	}

	return c
}

const (
	TraceAttributeEntityID     string = "entity-id"
	TraceAttributeNGSILDTenant string = "ngsild-tenant"
)

var tracer = otel.Tracer("context-broker-client")

type cbClient struct {
	baseURL string
	tenant  string
	debug   bool

	httpClient     http.Client
	requestHeaders map[string][]string
}

func (c cbClient) CreateEntity(ctx context.Context, entity types.Entity, headers map[string][]string) (*ngsild.CreateEntityResult, error) {
	var err error

	entityID := entity.ID()

	ctx, span := tracer.Start(ctx, "create-entity",
		trace.WithAttributes(attribute.String(TraceAttributeNGSILDTenant, c.tenant)),
		trace.WithAttributes(attribute.String(TraceAttributeEntityID, entityID)),
	)
	defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

	json, err := entity.MarshalJSON()
	body := bytes.NewBuffer(json)

	resp, respBody, err := c.callContextSource(
		ctx, http.MethodPost, c.baseURL+"/ngsi-ld/v1/entities", body, headers,
	)

	if err != nil {
		return nil, err
	}

	contentType := resp.Header.Get("Content-Type")
	log := logging.GetFromContext(ctx)

	if resp.StatusCode >= http.StatusBadRequest {
		err = errors.NewErrorFromProblemReport(resp.StatusCode, contentType, respBody)
		return nil, err
	}

	if resp.StatusCode != http.StatusCreated {
		err = fmt.Errorf("unexpected response code %d (%w)", resp.StatusCode, errors.ErrInternal)
		return nil, err
	}

	location := resp.Header.Get("Location")
	if location == "" {
		log.Warn("downstream context source failed to provide a location header with created response")
		location = "/ngsi-ld/v1/entities/" + url.QueryEscape(entityID)
	}

	return ngsild.NewCreateEntityResult(location), nil
}

func (c cbClient) RetrieveEntity(ctx context.Context, entityID string, headers map[string][]string) (types.Entity, error) {
	var err error

	ctx, span := tracer.Start(ctx, "retrieve-entity",
		trace.WithAttributes(attribute.String(TraceAttributeNGSILDTenant, c.tenant)),
		trace.WithAttributes(attribute.String(TraceAttributeEntityID, entityID)),
	)
	defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

	response, responseBody, err := c.callContextSource(
		ctx, http.MethodGet, c.baseURL+"/ngsi-ld/v1/entities/"+url.QueryEscape(entityID), nil, headers,
	)

	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		contentType := response.Header.Get("Content-Type")
		if response.StatusCode >= http.StatusBadRequest && response.StatusCode <= http.StatusInternalServerError {
			err = errors.NewErrorFromProblemReport(response.StatusCode, contentType, responseBody)
			return nil, err
		}

		err = fmt.Errorf("unexpected response code %d (%w)", response.StatusCode, errors.ErrInternal)
		return nil, err
	}

	return entities.NewFromJSON(responseBody)
}

func (c cbClient) QueryTemporalEvolutionOfEntities(ctx context.Context, headers map[string][]string, parameters ...RequestDecoratorFunc) (*ngsild.QueryTemporalEntitiesResult, error) {
	var err error

	ctx, span := tracer.Start(ctx, "query-temporal-entities",
		trace.WithAttributes(attribute.String(TraceAttributeNGSILDTenant, c.tenant)),
	)
	defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

	params := make([]string, 0, 5)
	for _, rdf := range parameters {
		params = rdf(params)
	}

	urlparams := ""
	if len(params) > 0 {
		urlparams = "?" + strings.Join(params, "&")
	}

	response, responseBody, err := c.callContextSource(
		ctx, http.MethodGet, c.baseURL+"/ngsi-ld/v1/temporal/entities"+urlparams, nil, headers,
	)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		contentType := response.Header.Get("Content-Type")
		if response.StatusCode >= http.StatusBadRequest && response.StatusCode <= http.StatusInternalServerError {
			err = errors.NewErrorFromProblemReport(response.StatusCode, contentType, responseBody)
			return nil, err
		}

		err = fmt.Errorf("unexpected response code %d (%w)", response.StatusCode, errors.ErrInternal)
		return nil, err
	}

	var entities []entities.EntityTemporalImpl
	err = json.Unmarshal(responseBody, &entities)
	if err != nil {
		if c.debug && len(responseBody) < 1000 {
			err = fmt.Errorf("unmarshaling of %s failed with err %s", string(responseBody), err.Error())
		}

		return nil, err
	}

	qer := ngsild.NewQueryTemporalEntitiesResult()

	if totalCount, ok := extractNGSILDResultsCount(response); ok {
		qer.TotalCount = totalCount
	}

	go func() {
		for idx := range entities {
			qer.Found <- entities[idx]
		}
		qer.Found <- nil
	}()
	return qer, nil
}

func (c cbClient) RetrieveTemporalEvolutionOfEntity(ctx context.Context, entityID string, headers map[string][]string, parameters ...RequestDecoratorFunc) (*ngsild.RetrieveTemporalEvolutionOfEntityResult, error) {
	var err error

	ctx, span := tracer.Start(ctx, "retrieve-entity-temporal",
		trace.WithAttributes(attribute.String(TraceAttributeNGSILDTenant, c.tenant)),
		trace.WithAttributes(attribute.String(TraceAttributeEntityID, entityID)),
	)
	defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

	params := make([]string, 0, 5)
	for _, rdf := range parameters {
		params = rdf(params)
	}

	urlparams := ""
	if len(params) > 0 {
		urlparams = "?" + strings.Join(params, "&")
	}

	requestURL := c.baseURL + "/ngsi-ld/v1/temporal/entities/" + url.QueryEscape(entityID) + urlparams
	response, responseBody, err := c.callContextSource(
		ctx, http.MethodGet, requestURL, nil, headers,
	)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		contentType := response.Header.Get("Content-Type")
		if response.StatusCode >= http.StatusBadRequest && response.StatusCode <= http.StatusInternalServerError {
			err = errors.NewErrorFromProblemReport(response.StatusCode, contentType, responseBody)
			return nil, err
		}

		err = fmt.Errorf("unexpected response code %d (%w)", response.StatusCode, errors.ErrInternal)
		return nil, err
	}

	entity, err := entities.NewTemporalFromJSON(responseBody)
	if err != nil {
		return nil, err
	}

	result := ngsild.NewRetrieveTemporalEvolutionOfEntityResult(entity)

	distances, _ := json.Marshal(result.Found.Property("distance"))

	fmt.Printf("TEMPORAL RESULT: %s\n", distances)

	if response.StatusCode == http.StatusPartialContent {
		contentRangeStr := response.Header.Get("Content-Range")

		fmt.Printf("context range header: %s\n", contentRangeStr)

		if contentRangeStr == "" {
			return nil, fmt.Errorf("partial response code received, but no content range header was found")
		}

		contentRangeSli := strings.Split(contentRangeStr, "")

		result.ContentRange = &ngsild.ContentRange{}
		result.PartialResult = true

		from := strings.Join(contentRangeSli[10:29], "")

		fmt.Printf("split content range header from: %s\n", from)

		startTime, err := time.Parse(time.RFC3339, from+"Z")
		if err != nil {
			log.Error(fmt.Sprintf("failed parse startTime: %s from query parameter from: %s", startTime, from), "err", err.Error())
			return nil, err
		}

		result.ContentRange.StartTime = &startTime

		to := strings.Join(contentRangeSli[30:49], "")

		fmt.Printf("split content range header to: %s\n", to)

		endTime, err := time.Parse(time.RFC3339, to+"Z")
		if err != nil {
			log.Error(fmt.Sprintf("failed parse endTime: %s from query parameter to: %s", endTime, to), "err", err.Error())
			return nil, err
		}

		result.ContentRange.EndTime = &endTime
	}

	return result, nil
}

func (c cbClient) MergeEntity(ctx context.Context, entityID string, fragment types.EntityFragment, headers map[string][]string) (*ngsild.MergeEntityResult, error) {
	var err error

	ctx, span := tracer.Start(ctx, "merge-entity",
		trace.WithAttributes(attribute.String(TraceAttributeNGSILDTenant, c.tenant)),
		trace.WithAttributes(attribute.String(TraceAttributeEntityID, entityID)),
	)
	defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

	json, err := fragment.MarshalJSON()
	body := bytes.NewBuffer(json)

	response, responseBody, err := c.callContextSource(
		ctx, http.MethodPatch, c.baseURL+"/ngsi-ld/v1/entities/"+url.QueryEscape(entityID), body, headers,
	)

	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusMultiStatus {
		contentType := response.Header.Get("Content-Type")
		if response.StatusCode >= http.StatusBadRequest && response.StatusCode <= http.StatusInternalServerError {
			return nil, errors.NewErrorFromProblemReport(response.StatusCode, contentType, responseBody)
		}

		return nil, fmt.Errorf("context source returned status code %d (content-type: %s, body: %s)", response.StatusCode, contentType, string(responseBody))
	}

	return ngsild.NewMergeEntityResult(responseBody)
}

func (c cbClient) UpdateEntityAttributes(ctx context.Context, entityID string, fragment types.EntityFragment, headers map[string][]string) (*ngsild.UpdateEntityAttributesResult, error) {
	var err error

	ctx, span := tracer.Start(ctx, "update-entity-attributes",
		trace.WithAttributes(attribute.String(TraceAttributeNGSILDTenant, c.tenant)),
		trace.WithAttributes(attribute.String(TraceAttributeEntityID, entityID)),
	)
	defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

	json, err := fragment.MarshalJSON()
	body := bytes.NewBuffer(json)

	response, responseBody, err := c.callContextSource(
		ctx, http.MethodPatch, c.baseURL+"/ngsi-ld/v1/entities/"+url.QueryEscape(entityID)+"/attrs/", body, headers,
	)

	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusMultiStatus {
		contentType := response.Header.Get("Content-Type")
		if response.StatusCode >= http.StatusBadRequest && response.StatusCode <= http.StatusInternalServerError {
			return nil, errors.NewErrorFromProblemReport(response.StatusCode, contentType, responseBody)
		}

		return nil, fmt.Errorf("context source returned status code %d (content-type: %s, body: %s)", response.StatusCode, contentType, string(responseBody))
	}

	return ngsild.NewUpdateEntityAttributesResult(responseBody)
}

func (c cbClient) QueryEntities(ctx context.Context, entityTypes, entityAttributes []string, query string, headers map[string][]string) (*ngsild.QueryEntitiesResult, error) {
	var err error

	ctx, span := tracer.Start(ctx, "query-entities",
		trace.WithAttributes(attribute.String(TraceAttributeNGSILDTenant, c.tenant)),
	)
	defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

	queryValues, err := url.ParseQuery(query[strings.Index(query, "?")+1:])
	if err != nil {
		return nil, fmt.Errorf("invalid query parameter")
	}

	endpoint := fmt.Sprintf("%s/ngsi-ld/v1/entities?%s", c.baseURL, queryValues.Encode())
	response, responseBody, err := c.callContextSource(ctx, http.MethodGet, endpoint, nil, headers)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		contentType := response.Header.Get("Content-Type")
		if response.StatusCode >= http.StatusBadRequest && response.StatusCode <= http.StatusInternalServerError {
			return nil, errors.NewErrorFromProblemReport(response.StatusCode, contentType, responseBody)
		}
		return nil, fmt.Errorf("context source returned status code %d (content-type: %s, body: %s)", response.StatusCode, contentType, string(responseBody))
	}

	var entities []entities.EntityImpl
	err = json.Unmarshal(responseBody, &entities)
	if err != nil {
		if c.debug && len(responseBody) < 1000 {
			err = fmt.Errorf("unmarshaling of %s failed with err %s", string(responseBody), err.Error())
		}

		return nil, err
	}

	qer := ngsild.NewQueryEntitiesResult()

	if totalCount, ok := extractNGSILDResultsCount(response); ok {
		qer.TotalCount = totalCount
	}

	qer.Count = len(entities)

	if queryValues.Has("offset") {
		if i, err := strconv.ParseInt(queryValues.Get("offset"), 0, 64); err == nil {
			qer.Offset = int(i)
		}
	}

	if queryValues.Has("limit") {
		if i, err := strconv.ParseInt(queryValues.Get("limit"), 0, 64); err == nil {
			qer.Limit = int(i)
		}
	}

	qer.PartialResult = qer.Count == qer.Limit || qer.Offset != 0

	go func() {
		for idx := range entities {
			qer.Found <- entities[idx]
		}
		qer.Found <- nil
	}()

	return qer, nil
}

func (c cbClient) DeleteEntity(ctx context.Context, entityID string) (*ngsild.DeleteEntityResult, error) {
	var err error

	ctx, span := tracer.Start(ctx, "delete-entity",
		trace.WithAttributes(attribute.String(TraceAttributeNGSILDTenant, c.tenant)),
		trace.WithAttributes(attribute.String(TraceAttributeEntityID, entityID)),
	)
	defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

	response, responseBody, err := c.callContextSource(
		ctx, http.MethodDelete, c.baseURL+"/ngsi-ld/v1/entities/"+url.QueryEscape(entityID), nil, nil,
	)

	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusNoContent {
		contentType := response.Header.Get("Content-Type")
		if response.StatusCode >= http.StatusBadRequest && response.StatusCode <= http.StatusInternalServerError {
			return nil, errors.NewErrorFromProblemReport(response.StatusCode, contentType, responseBody)
		}

		return nil, fmt.Errorf("context source returned status code %d (content-type: %s, body: %s)", response.StatusCode, contentType, string(responseBody))
	}

	return ngsild.NewDeleteEntityResult(), nil
}

func (c cbClient) callContextSource(ctx context.Context, method, endpoint string, body io.Reader, headers map[string][]string) (*http.Response, []byte, error) {

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %s (%w)", err.Error(), errors.ErrInternal)
	}

	if c.tenant != entities.DefaultNGSITenant {
		req.Header.Add("NGSILD-Tenant", c.tenant)
	}

	for header, headerValue := range c.requestHeaders {
		for _, val := range headerValue {
			req.Header.Add(header, val)
		}
	}

	for header, headerValue := range headers {
		for _, val := range headerValue {
			req.Header.Add(header, val)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to send request: %s (%w)", err.Error(), errors.ErrRequest)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response body: %s (%w)", err.Error(), errors.ErrBadResponse)
	}

	if c.debug {
		if resp.StatusCode == http.StatusPartialContent || resp.StatusCode >= http.StatusBadRequest {
			if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusNotFound {
				reqbytes, _ := httputil.DumpRequest(req, false)
				respbytes, _ := httputil.DumpResponse(resp, false)

				log := logging.GetFromContext(ctx)
				if resp.StatusCode >= http.StatusBadRequest {
					log.Error("request failed", "request", string(reqbytes), "response", string(respbytes))
				} else {
					log.Warn("unexpected response", "request", string(reqbytes), "response", string(respbytes))
				}
			}
		}
	}

	return resp, respBody, nil
}

func extractNGSILDResultsCount(r *http.Response) (int64, bool) {
	val, ok := r.Header[http.CanonicalHeaderKey("NGSILD-Results-Count")]
	if !ok || len(val) == 0 {
		return -1, false
	}

	count, err := strconv.ParseInt(val[0], 10, 64)
	if err != nil {
		return -1, false
	}

	return count, true
}
