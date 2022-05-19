package contextbroker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"regexp"

	"github.com/diwise/context-broker/internal/pkg/application/cim"
	"github.com/diwise/context-broker/internal/pkg/infrastructure/logging"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type contextBrokerApp struct {
	tenants map[string][]ContextSourceConfig
}

func New(log zerolog.Logger, cfg Config) (cim.ContextInformationManager, error) {
	app := &contextBrokerApp{
		tenants: make(map[string][]ContextSourceConfig),
	}

	for _, tenant := range cfg.Tenants {
		app.tenants[tenant.ID] = tenant.ContextSources
	}

	return app, nil
}

func (app *contextBrokerApp) CreateEntity(ctx context.Context, tenant, entityType, entityID string, body io.Reader, headers map[string][]string) (*cim.CreateEntityResult, error) {
	sources, ok := app.tenants[tenant]
	if !ok {
		return nil, cim.NewUnknownTenantError(tenant)
	}

	log := logging.GetFromContext(ctx)

	for _, src := range sources {
		for _, reginfo := range src.Information {
			for _, entityInfo := range reginfo.Entities {
				if entityInfo.Type != entityType {
					continue
				}

				regexpForID, err := regexp.CompilePOSIX(entityInfo.IDPattern)
				if err != nil {
					continue
				}

				if !regexpForID.MatchString(entityID) {
					continue
				}

				response, responseBody, err := callContextSource(ctx, http.MethodPost, src.Endpoint+"/ngsi-ld/v1/entities", body, headers)
				if err != nil {
					return nil, err
				}

				if response.StatusCode == http.StatusCreated {
					location := response.Header.Get("Location")
					if location == "" {
						log.Warn().Msg("downstream context source failed to provide a location header with created response")
						location = "/ngsi-ld/v1/entities/" + entityID
					}
					return cim.NewCreateEntityResult(location), nil
				}

				contentType := response.Header.Get("Content-Type")
				if response.StatusCode >= http.StatusBadRequest && response.StatusCode <= http.StatusInternalServerError {
					return nil, cim.NewErrorFromProblemReport(response.StatusCode, contentType, responseBody)
				}

				return nil, fmt.Errorf("context source returned status code %d (content-type: %s, body: %s)", response.StatusCode, contentType, string(responseBody))
			}
		}
	}

	return nil, cim.NewNotFoundError(fmt.Sprintf("no context source found that could create type %s with id %s", entityType, entityID))
}

func notInSlice(find string, slice []string) bool {
	for idx := range slice {
		if slice[idx] == find {
			return false
		}
	}
	return true
}

func (app *contextBrokerApp) QueryEntities(ctx context.Context, tenant string, entityTypes, entityAttributes []string, query string, headers map[string][]string) (*cim.QueryEntitiesResult, error) {
	sources, ok := app.tenants[tenant]
	if !ok {
		return nil, cim.NewUnknownTenantError(tenant)
	}

	for _, src := range sources {
		for _, reginfo := range src.Information {
			for _, entityInfo := range reginfo.Entities {
				if notInSlice(entityInfo.Type, entityTypes) {
					continue
				}

				response, responseBody, err := callContextSource(ctx, http.MethodGet, src.Endpoint+query, nil, headers)
				if err != nil {
					return nil, err
				}

				if response.StatusCode != http.StatusOK {
					contentType := response.Header.Get("Content-Type")
					if response.StatusCode >= http.StatusBadRequest && response.StatusCode <= http.StatusInternalServerError {
						return nil, cim.NewErrorFromProblemReport(response.StatusCode, contentType, responseBody)
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
		}
	}

	return nil, cim.NewNotFoundError(fmt.Sprintf("no context source found that could handle query %s", query))
}

func (app *contextBrokerApp) RetrieveEntity(ctx context.Context, tenant, entityID string, headers map[string][]string) (cim.Entity, error) {
	sources, ok := app.tenants[tenant]
	if !ok {
		return nil, cim.NewUnknownTenantError(tenant)
	}

	for _, src := range sources {
		for _, reginfo := range src.Information {
			for _, entityInfo := range reginfo.Entities {

				regexpForID, err := regexp.CompilePOSIX(entityInfo.IDPattern)
				if err != nil {
					continue
				}

				if !regexpForID.MatchString(entityID) {
					continue
				}

				response, responseBody, err := callContextSource(
					ctx, http.MethodGet, src.Endpoint+"/ngsi-ld/v1/entities/"+entityID, nil, headers,
				)

				if err != nil {
					return nil, err
				}

				if response.StatusCode != http.StatusOK {
					contentType := response.Header.Get("Content-Type")
					if response.StatusCode >= http.StatusBadRequest && response.StatusCode <= http.StatusInternalServerError {
						return nil, cim.NewErrorFromProblemReport(response.StatusCode, contentType, responseBody)
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
		}
	}

	return nil, cim.NewNotFoundError(fmt.Sprintf("no context source found that could provide entity %s", entityID))
}

func (app *contextBrokerApp) UpdateEntityAttributes(ctx context.Context, tenant, entityID string, body io.Reader, headers map[string][]string) error {
	sources, ok := app.tenants[tenant]
	if !ok {
		return cim.NewUnknownTenantError(tenant)
	}

	for _, src := range sources {
		for _, reginfo := range src.Information {
			for _, entityInfo := range reginfo.Entities {

				regexpForID, err := regexp.CompilePOSIX(entityInfo.IDPattern)
				if err != nil {
					continue
				}

				if !regexpForID.MatchString(entityID) {
					continue
				}

				response, responseBody, err := callContextSource(
					ctx, http.MethodPatch, src.Endpoint+"/ngsi-ld/v1/entities/"+entityID+"/attrs/", body, headers,
				)

				if err != nil {
					return err
				}

				if response.StatusCode != http.StatusNoContent {
					contentType := response.Header.Get("Content-Type")
					if response.StatusCode >= http.StatusBadRequest && response.StatusCode <= http.StatusInternalServerError {
						return cim.NewErrorFromProblemReport(response.StatusCode, contentType, responseBody)
					}

					return fmt.Errorf("context source returned status code %d (content-type: %s, body: %s)", response.StatusCode, contentType, string(responseBody))
				}

				return nil
			}
		}
	}

	return cim.NewNotFoundError(fmt.Sprintf("no context source found that could update attributes for entity %s", entityID))
}

func callContextSource(ctx context.Context, method, endpoint string, body io.Reader, headers map[string][]string) (*http.Response, []byte, error) {
	client := http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, nil, err
	}

	for header, headerValue := range headers {
		for _, val := range headerValue {
			req.Header.Add(header, val)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}

	defer resp.Body.Close()
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		reqbytes, _ := httputil.DumpRequest(req, false)
		respbytes, _ := httputil.DumpResponse(resp, false)

		log := logging.GetFromContext(ctx)

		log.Error().Str("request", string(reqbytes)).Str("response", string(respbytes)).Msg("request failed")

	}

	return resp, respBody, nil
}
