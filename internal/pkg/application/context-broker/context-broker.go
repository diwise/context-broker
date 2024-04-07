package contextbroker

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	"github.com/diwise/context-broker/internal/pkg/application/cim"
	"github.com/diwise/context-broker/internal/pkg/application/config"
	"github.com/diwise/context-broker/internal/pkg/application/subscriptions"
	"github.com/diwise/context-broker/pkg/ngsild"
	"github.com/diwise/context-broker/pkg/ngsild/client"
	"github.com/diwise/context-broker/pkg/ngsild/errors"
	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	"github.com/diwise/service-chassis/pkg/infrastructure/env"
)

type contextBrokerApp struct {
	tenants     map[string][]config.ContextSourceConfig
	notifier    subscriptions.Notifier
	debugClient string
}

func New(ctx context.Context, cfg config.Config) (cim.ContextInformationManager, error) {

	notifier, _ := subscriptions.NewNotifier(ctx, cfg)

	app := &contextBrokerApp{
		tenants:     make(map[string][]config.ContextSourceConfig),
		notifier:    notifier,
		debugClient: env.GetVariableOrDefault(ctx, "CONTEXT_BROKER_CLIENT_DEBUG", "false"),
	}

	for _, tenant := range cfg.Tenants {
		app.tenants[tenant.ID] = tenant.ContextSources
	}

	return app, nil
}

func (app *contextBrokerApp) CreateEntity(ctx context.Context, tenant string, entity types.Entity, headers map[string][]string) (*ngsild.CreateEntityResult, error) {
	sources, ok := app.tenants[tenant]
	if !ok {
		return nil, errors.NewUnknownTenantError(tenant)
	}

	entityID := entity.ID()
	entityType := entity.Type()

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

				cbClient := client.NewContextBrokerClient(src.Endpoint, client.Debug(app.debugClient))
				result, err := cbClient.CreateEntity(ctx, entity, headers)
				if err != nil {
					return nil, err
				}

				if app.notifier != nil {
					app.notifier.EntityCreated(ctx, entity, tenant)
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

				cbClient := client.NewContextBrokerClient(src.Endpoint, client.Debug(app.debugClient))
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

				cbClient := client.NewContextBrokerClient(src.Endpoint, client.Debug(app.debugClient))
				return cbClient.RetrieveEntity(ctx, entityID, headers)
			}
		}
	}

	return nil, errors.NewNotFoundError(fmt.Sprintf("no context source found that could provide entity %s", entityID))
}

func (app *contextBrokerApp) QueryTemporalEvolutionOfEntities(ctx context.Context, tenant string, entityIDs, entityTypes []string, params cim.TemporalQueryParams, headers map[string][]string) (*ngsild.QueryTemporalEntitiesResult, error) {
	sources, ok := app.tenants[tenant]
	if !ok {
		return nil, errors.NewUnknownTenantError(tenant)
	}

	matchesIDPattern := func(ids []string, reg *regexp.Regexp) bool {
		for _, id := range ids {
			if reg.MatchString(id) {
				return true
			}
		}

		return (len(ids) == 0) // true if there were no ids to match, false otherwise
	}

	for _, src := range sources {
		for _, reginfo := range src.Information {
			for _, entityInfo := range reginfo.Entities {

				if !src.Temporal.Enabled {
					continue
				}

				if len(entityTypes) > 0 && notInSlice(entityInfo.Type, entityTypes) {
					continue
				}

				regexpForID, err := regexp.CompilePOSIX(entityInfo.IDPattern)
				if err != nil {
					continue
				}

				// TODO: This might be a partial match and require dispatching the
				// query to more than one context source. This implementation will
				// only query the first match though, so do not mix ids from different
				// entity types!
				if !matchesIDPattern(entityIDs, regexpForID) {
					continue
				}

				cbClient := client.NewContextBrokerClient(src.TemporalEndpoint(), client.Debug(app.debugClient))
				queryParams := make([]client.RequestDecoratorFunc, 0, 10)

				if len(entityIDs) > 0 {
					queryParams = append(queryParams, client.IDs(entityIDs))
				}

				if len(entityTypes) > 0 {
					queryParams = append(queryParams, client.Types(entityTypes))
				}

				attrs, ok := params.Attributes()
				if ok {
					queryParams = append(queryParams, client.Attributes(attrs))
				}

				temprel, ok := params.TemporalRelation()
				if ok {
					if temprel == "after" {
						t, _ := params.TimeAt()
						queryParams = append(queryParams, client.After(t))
					} else if temprel == "between" {
						st, _ := params.TimeAt()
						et, _ := params.EndTimeAt()
						queryParams = append(queryParams, client.Between(st, et))
					} else if temprel == "before" {
						t, _ := params.TimeAt()
						queryParams = append(queryParams, client.Before(t))
					}
				}

				count, ok := params.LastN()
				if ok {
					queryParams = append(queryParams, client.LastN(count))
				}

				return cbClient.QueryTemporalEvolutionOfEntities(ctx, headers, queryParams...)
			}
		}
	}

	return nil, errors.NewNotFoundError("no context source found that could provide temporal evolution of entities")
}

func (app *contextBrokerApp) RetrieveTemporalEvolutionOfEntity(ctx context.Context, tenant, entityID string, params cim.TemporalQueryParams, headers map[string][]string) (types.EntityTemporal, error) {
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

				if !src.Temporal.Enabled {
					return nil, errors.NewNotFoundError("matching context source does not support temporal evolution")
				}

				cbClient := client.NewContextBrokerClient(src.TemporalEndpoint(), client.Debug(app.debugClient))
				queryParams := make([]client.RequestDecoratorFunc, 0, 10)

				attrs, ok := params.Attributes()
				if ok {
					queryParams = append(queryParams, client.Attributes(attrs))
				}

				temprel, ok := params.TemporalRelation()
				if ok {
					if temprel == "after" {
						t, _ := params.TimeAt()
						queryParams = append(queryParams, client.After(t))
					} else if temprel == "between" {
						st, _ := params.TimeAt()
						et, _ := params.EndTimeAt()
						queryParams = append(queryParams, client.Between(st, et))
					} else if temprel == "before" {
						t, _ := params.TimeAt()
						queryParams = append(queryParams, client.Before(t))
					}
				}

				count, ok := params.LastN()
				if ok {
					queryParams = append(queryParams, client.LastN(count))
				}

				return cbClient.RetrieveTemporalEvolutionOfEntity(ctx, entityID, headers, queryParams...)
			}
		}
	}

	return nil, errors.NewNotFoundError(fmt.Sprintf("no context source found that could provide temporal evolution of entity %s", entityID))
}

func (app *contextBrokerApp) RetrieveTypes(ctx context.Context, tenant string, headers map[string][]string) ([]string, error) {
	sources, ok := app.tenants[tenant]
	if !ok {
		return nil, errors.NewUnknownTenantError(tenant)
	}

	availableTypes := map[string]struct{}{}

	for _, src := range sources {
		for _, reginfo := range src.Information {
			for _, entityInfo := range reginfo.Entities {
				availableTypes[entityInfo.Type] = struct{}{}
			}
		}
	}

	typeList := make([]string, 0, len(availableTypes))

	for k := range availableTypes {
		typeList = append(typeList, k)
	}

	sort.Strings(typeList)

	return typeList, nil
}

func (app *contextBrokerApp) MergeEntity(ctx context.Context, tenant, entityID string, fragment types.EntityFragment, headers map[string][]string) (*ngsild.MergeEntityResult, error) {
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

				cbClient := client.NewContextBrokerClient(src.Endpoint, client.Debug(app.debugClient))

				current, err := cbClient.RetrieveEntity(ctx, entityID, map[string][]string{
					"Accept": {"application/ld+json"},
					"Link":   {entities.LinkHeader},
				})
				if err != nil {
					return nil, err
				}

				fragmentImpl, ok := fragment.(*entities.EntityImpl)
				if ok {
					current.ForEachAttribute(func(ct, cn string, cc any) {
						fragmentImpl.RemoveAttribute(func(ft, fn string, fc any) bool {
							if ct == ft && cn == fn {
								c, _ := json.Marshal(cc)
								f, _ := json.Marshal(fc)
								return string(c) == string(f)
							}
							return false
						})
					})
				}

				result, err := cbClient.MergeEntity(ctx, entityID, fragment, headers)
				if err != nil {
					return result, err
				}

				if app.notifier != nil {
					// Spawn a go routine to fetch the updated entity in its entirety
					go func() {
						delete(headers, "Content-Type")
						headers["Accept"] = []string{"application/ld+json"}
						headers["Link"] = []string{entities.LinkHeader}

						ctx := context.WithoutCancel(ctx)

						entity, err := cbClient.RetrieveEntity(ctx, entityID, headers)
						if err == nil {
							app.notifier.EntityUpdated(ctx, entity, tenant)
						}
					}()
				}

				return result, err
			}
		}
	}

	return nil, errors.NewNotFoundError(fmt.Sprintf("no context source found that could update attributes for entity %s", entityID))
}

func (app *contextBrokerApp) UpdateEntityAttributes(ctx context.Context, tenant, entityID string, fragment types.EntityFragment, headers map[string][]string) (*ngsild.UpdateEntityAttributesResult, error) {
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

				cbClient := client.NewContextBrokerClient(src.Endpoint, client.Debug(app.debugClient))
				result, err := cbClient.UpdateEntityAttributes(ctx, entityID, fragment, headers)
				if err != nil {
					return result, err
				}

				if app.notifier != nil {
					// Spawn a go routine to fetch the updated entity in its entirety
					go func() {
						delete(headers, "Content-Type")
						headers["Accept"] = []string{"application/ld+json"}
						headers["Link"] = []string{entities.LinkHeader}

						ctx := context.WithoutCancel(ctx)

						entity, err := cbClient.RetrieveEntity(ctx, entityID, headers)
						if err == nil {
							app.notifier.EntityUpdated(ctx, entity, tenant)
						}
					}()
				}

				return result, err
			}
		}
	}

	return nil, errors.NewNotFoundError(fmt.Sprintf("no context source found that could update attributes for entity %s", entityID))
}

func (app *contextBrokerApp) DeleteEntity(ctx context.Context, tenant, entityID string) (*ngsild.DeleteEntityResult, error) {
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

				cbClient := client.NewContextBrokerClient(src.Endpoint, client.Debug(app.debugClient))
				return cbClient.DeleteEntity(ctx, entityID)
			}
		}
	}

	return nil, errors.NewNotFoundError(fmt.Sprintf("no context source found that could delete entity with id %s", entityID))
}

func (app *contextBrokerApp) Start() error {
	if app.notifier != nil {
		return app.notifier.Start()
	}

	return nil
}

func (app *contextBrokerApp) Stop() error {
	if app.notifier != nil {
		return app.notifier.Stop()
	}

	return nil
}
