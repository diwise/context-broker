package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

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
	RetrieveTemporalEvolutionOfEntity(ctx context.Context, entityID string, headers map[string][]string, parameters ...RequestDecoratorFunc) (types.EntityTemporal, error)
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

func Tenant(tenant string) func(*cbClient) {
	return func(c *cbClient) {
		c.tenant = tenant
	}
}

func NewContextBrokerClient(broker string, options ...func(*cbClient)) ContextBrokerClient {
	c := &cbClient{
		baseURL: broker,
		tenant:  entities.DefaultNGSITenant,
		debug:   false,
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
		log.Warn().Msg("downstream context source failed to provide a location header with created response")
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

func (c cbClient) RetrieveTemporalEvolutionOfEntity(ctx context.Context, entityID string, headers map[string][]string, parameters ...RequestDecoratorFunc) (types.EntityTemporal, error) {
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

	response, responseBody, err := c.callContextSource(
		ctx, http.MethodGet, c.baseURL+"/ngsi-ld/v1/temporal/entities/"+url.QueryEscape(entityID)+urlparams, nil, headers,
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

	return entities.NewTemporalFromJSON(responseBody)
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

	response, responseBody, err := c.callContextSource(ctx, http.MethodGet, c.baseURL+query, nil, headers)
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
	httpClient := http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %s (%w)", err.Error(), errors.ErrInternal)
	}

	if c.tenant != entities.DefaultNGSITenant {
		req.Header.Add("NGSILD-Tenant", c.tenant)
	}

	for header, headerValue := range headers {
		for _, val := range headerValue {
			req.Header.Add(header, val)
		}
	}

	resp, err := httpClient.Do(req)
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
					log.Error().Str("request", string(reqbytes)).Str("response", string(respbytes)).Msg("request failed")
				} else {
					log.Warn().Str("request", string(reqbytes)).Str("response", string(respbytes)).Msg("unexpected response")
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
