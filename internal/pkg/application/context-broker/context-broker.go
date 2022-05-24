package contextbroker

import (
	"context"
	"fmt"
	"io"
	"regexp"

	"github.com/diwise/context-broker/internal/pkg/application/cim"
	"github.com/diwise/context-broker/pkg/ngsild"
	"github.com/diwise/context-broker/pkg/ngsild/client"
	"github.com/diwise/context-broker/pkg/ngsild/errors"
	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/rs/zerolog"
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

func (app *contextBrokerApp) CreateEntity(ctx context.Context, tenant, entityType, entityID string, body io.Reader, headers map[string][]string) (*ngsild.CreateEntityResult, error) {
	sources, ok := app.tenants[tenant]
	if !ok {
		return nil, errors.NewUnknownTenantError(tenant)
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

				cbClient := client.NewContextBrokerClient(src.Endpoint)
				result, err := cbClient.CreateEntity(ctx, entityID, body, headers)
				if err != nil {
					return nil, err
				}

				return result, nil
			}
		}
	}

	return nil, errors.NewNotFoundError(fmt.Sprintf("no context source found that could create type %s with id %s", entityType, entityID))
}

func notInSlice(find string, slice []string) bool {
	for idx := range slice {
		if slice[idx] == find {
			return false
		}
	}
	return true
}

func (app *contextBrokerApp) QueryEntities(ctx context.Context, tenant string, entityTypes, entityAttributes []string, query string, headers map[string][]string) (*ngsild.QueryEntitiesResult, error) {
	sources, ok := app.tenants[tenant]
	if !ok {
		return nil, errors.NewUnknownTenantError(tenant)
	}

	for _, src := range sources {
		for _, reginfo := range src.Information {
			for _, entityInfo := range reginfo.Entities {
				if notInSlice(entityInfo.Type, entityTypes) {
					continue
				}

				cbClient := client.NewContextBrokerClient(src.Endpoint)
				return cbClient.QueryEntities(ctx, entityTypes, entityAttributes, query, headers)
			}
		}
	}

	return nil, errors.NewNotFoundError(fmt.Sprintf("no context source found that could handle query %s", query))
}

func (app *contextBrokerApp) RetrieveEntity(ctx context.Context, tenant, entityID string, headers map[string][]string) (types.Entity, error) {
	sources, ok := app.tenants[tenant]
	if !ok {
		return nil, errors.NewUnknownTenantError(tenant)
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

				cbClient := client.NewContextBrokerClient(src.Endpoint)
				return cbClient.RetrieveEntity(ctx, entityID, headers)
			}
		}
	}

	return nil, errors.NewNotFoundError(fmt.Sprintf("no context source found that could provide entity %s", entityID))
}

func (app *contextBrokerApp) UpdateEntityAttributes(ctx context.Context, tenant, entityID string, body io.Reader, headers map[string][]string) (*ngsild.UpdateEntityAttributesResult, error) {
	sources, ok := app.tenants[tenant]
	if !ok {
		return nil, errors.NewUnknownTenantError(tenant)
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

				cbClient := client.NewContextBrokerClient(src.Endpoint)
				return cbClient.UpdateEntityAttributes(ctx, entityID, body, headers)
			}
		}
	}

	return nil, errors.NewNotFoundError(fmt.Sprintf("no context source found that could update attributes for entity %s", entityID))
}
