package contextbroker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"

	"github.com/diwise/ngsi-ld-context-broker/internal/pkg/application/cim"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type contextBrokerApp struct {
	log zerolog.Logger

	tenants map[string][]ContextSourceConfig
}

func New(log zerolog.Logger, cfg Config) (cim.ContextInformationManager, error) {
	app := &contextBrokerApp{
		log:     log,
		tenants: make(map[string][]ContextSourceConfig),
	}

	for _, tenant := range cfg.Tenants {
		app.tenants[tenant.ID] = tenant.ContextSources
	}

	return app, nil
}

func (app *contextBrokerApp) CreateEntity(ctx context.Context, tenant, entityType, entityID string, body io.Reader) (*cim.CreateEntityResult, error) {
	sources, ok := app.tenants[tenant]
	if !ok {
		return nil, cim.NewUnknownTenantError(tenant)
	}

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

				response, responseBody, err := callContextSource(ctx, http.MethodPost, src.Endpoint+"/ngsi-ld/v1/entities", "application/ld+json", body)
				if err != nil {
					return nil, err
				}

				if response.StatusCode == http.StatusCreated {
					location := response.Header.Get("Location")
					if location == "" {
						app.log.Warn().Msg("downstream context source failed to provide a location header with created response")
						location = "/ngsi-ld/v1/entities/" + entityID
					}
					return cim.NewCreateEntityResult(location), nil
				}

				contentType := response.Header.Get("Content-Type")
				if contentType == "application/problem+json" {
					return nil, cim.NewErrorFromProblemReport(response.StatusCode, responseBody)
				}

				return nil, fmt.Errorf("context source returned status code %d", response.StatusCode)
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

func (app *contextBrokerApp) QueryEntities(ctx context.Context, tenant string, entityTypes, entityAttributes []string, query string) (*cim.QueryEntitiesResult, error) {
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

				response, responseBody, err := callContextSource(ctx, http.MethodGet, src.Endpoint+query, "application/ld+json", nil)
				if err != nil {
					return nil, err
				}

				if response.StatusCode != http.StatusOK {
					contentType := response.Header.Get("Content-Type")
					if contentType == "application/problem+json" {
						return nil, cim.NewErrorFromProblemReport(response.StatusCode, responseBody)
					}
					return nil, fmt.Errorf("context source returned status code %d", response.StatusCode)
				}

				var entities []cim.Entity
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

func callContextSource(ctx context.Context, method, endpoint, contentType string, body io.Reader) (*http.Response, []byte, error) {
	client := http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Add("Content-Type", contentType)

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}

	defer resp.Body.Close()
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	return resp, respBody, nil
}
