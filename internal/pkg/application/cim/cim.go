package cim

import (
	"context"

	"github.com/diwise/context-broker/pkg/ngsild"
	"github.com/diwise/context-broker/pkg/ngsild/types"
)

type EntityAttributesUpdater interface {
	UpdateEntityAttributes(ctx context.Context, tenant, entityID string, fragment types.EntityFragment, headers map[string][]string) (*ngsild.UpdateEntityAttributesResult, error)
}

type EntityCreator interface {
	CreateEntity(ctx context.Context, tenant string, entity types.Entity, headers map[string][]string) (*ngsild.CreateEntityResult, error)
}

type EntityMerger interface {
	MergeEntity(ctx context.Context, tenant, entityID string, fragment types.EntityFragment, headers map[string][]string) (*ngsild.MergeEntityResult, error)
}

type EntityQuerier interface {
	QueryEntities(ctx context.Context, tenant string, entityTypes, entityAttributes []string, query string, headers map[string][]string) (*ngsild.QueryEntitiesResult, error)
}

type EntityRetriever interface {
	RetrieveEntity(ctx context.Context, tenant, entityID string, headers map[string][]string) (types.Entity, error)
}

type EntityTemporalRetriever interface {
	RetrieveTemporalEvolutionOfEntity(ctx context.Context, tenant, entityID string, headers map[string][]string) (types.EntityTemporal, error)
}

type EntityDeleter interface {
	DeleteEntity(ctx context.Context, tenant, entityID string) (*ngsild.DeleteEntityResult, error)
}

//go:generate moq -rm -out cim_mock.go . ContextInformationManager

type ContextInformationManager interface {
	EntityAttributesUpdater
	EntityCreator
	EntityMerger
	EntityQuerier
	EntityRetriever
	EntityDeleter

	EntityTemporalRetriever

	Start() error
	Stop() error
}

type ContextBroker interface {
}

type ContextSource interface {
}
