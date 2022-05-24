package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/diwise/context-broker/internal/pkg/application/cim"
	"github.com/diwise/context-broker/internal/pkg/infrastructure/logging"
	"github.com/diwise/context-broker/internal/pkg/infrastructure/tracing"
	"github.com/diwise/context-broker/pkg/errors"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type ContextBrokerClient interface {
	CreateEntity(ctx context.Context, entityID string, body io.Reader, headers map[string][]string) (*cim.CreateEntityResult, error)
	QueryEntities(ctx context.Context, entityTypes, entityAttributes []string, query string, headers map[string][]string) (*cim.QueryEntitiesResult, error)
	RetrieveEntity(ctx context.Context, entityID string, headers map[string][]string) (cim.Entity, error)
	UpdateEntityAttributes(ctx context.Context, entityID string, body io.Reader, headers map[string][]string) (*cim.UpdateEntityAttributesResult, error)
}

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
		tenant:  "default",
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

func (c cbClient) CreateEntity(ctx context.Context, entityID string, body io.Reader, headers map[string][]string) (*cim.CreateEntityResult, error) {
	var err error

	ctx, span := tracer.Start(ctx, "create-entity",
		trace.WithAttributes(attribute.String(TraceAttributeNGSILDTenant, c.tenant)),
		trace.WithAttributes(attribute.String(TraceAttributeEntityID, entityID)),
	)
	defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

	resp, respBody, err := c.callContextSource(
		ctx, http.MethodPost, c.baseURL+"/ngsi-ld/v1/entities", body, headers,
	)

	contentType := resp.Header.Get("Content-Type")
	log := logging.GetFromContext(ctx)

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, errors.NewErrorFromProblemReport(resp.StatusCode, contentType, respBody)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("context source returned status code %d (content-type: %s, body: %s)", resp.StatusCode, contentType, string(respBody))
	}

	location := resp.Header.Get("Location")
	if location == "" {
		log.Warn().Msg("downstream context source failed to provide a location header with created response")
		location = "/ngsi-ld/v1/entities/" + url.QueryEscape(entityID)
	}

	return cim.NewCreateEntityResult(location), nil
}

func (c cbClient) RetrieveEntity(ctx context.Context, entityID string, headers map[string][]string) (cim.Entity, error) {
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
			return nil, errors.NewErrorFromProblemReport(response.StatusCode, contentType, responseBody)
		}
		return nil, fmt.Errorf("context source returned status code %d (content-type: %s, body: %s)", response.StatusCode, contentType, string(responseBody))
	}

	var entity cim.EntityImpl
	err = json.Unmarshal(responseBody, &entity)
	if err != nil {
		return nil, err
	}

	return entity, nil
}

func (c cbClient) UpdateEntityAttributes(ctx context.Context, entityID string, body io.Reader, headers map[string][]string) (*cim.UpdateEntityAttributesResult, error) {
	var err error

	ctx, span := tracer.Start(ctx, "update-entity-attributes",
		trace.WithAttributes(attribute.String(TraceAttributeNGSILDTenant, c.tenant)),
		trace.WithAttributes(attribute.String(TraceAttributeEntityID, entityID)),
	)
	defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

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

	return cim.NewUpdateEntityAttributesResult(responseBody)
}

func (c cbClient) QueryEntities(ctx context.Context, entityTypes, entityAttributes []string, query string, headers map[string][]string) (*cim.QueryEntitiesResult, error) {
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

	var entities []cim.EntityImpl
	err = json.Unmarshal(responseBody, &entities)
	if err != nil {
		return nil, err
	}

	qer := cim.NewQueryEntitiesResult()
	go func() {
		for idx := range entities {
			qer.Found <- entities[idx]
		}
		qer.Found <- nil
	}()
	return qer, nil
}

func (c cbClient) callContextSource(ctx context.Context, method, endpoint string, body io.Reader, headers map[string][]string) (*http.Response, []byte, error) {
	httpClient := http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		err = fmt.Errorf("failed to create request: %s (%w)", err.Error(), errors.ErrInternal)
		return nil, nil, err
	}

	for header, headerValue := range headers {
		for _, val := range headerValue {
			req.Header.Add(header, val)
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("failed to send request: %s (%w)", err.Error(), errors.ErrRequest)
		return nil, nil, err
	}

	defer resp.Body.Close()
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("failed to read response body: %s (%w)", err.Error(), errors.ErrBadResponse)
		return nil, nil, err
	}

	if c.debug && resp.StatusCode >= http.StatusBadRequest {
		reqbytes, _ := httputil.DumpRequest(req, false)
		respbytes, _ := httputil.DumpResponse(resp, false)

		log := logging.GetFromContext(ctx)
		log.Error().Str("request", string(reqbytes)).Str("response", string(respbytes)).Msg("request failed")
	}

	return resp, respBody, nil
}
