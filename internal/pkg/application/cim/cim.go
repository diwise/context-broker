package cim

import (
	"context"
	"io"

	"github.com/diwise/context-broker/pkg/ngsild"
	"github.com/diwise/context-broker/pkg/ngsild/types"
)

type EntityAttributesUpdater interface {
	UpdateEntityAttributes(ctx context.Context, tenant, entityID string, body io.Reader, headers map[string][]string) (*ngsild.UpdateEntityAttributesResult, error)
}

type EntityCreator interface {
	CreateEntity(ctx context.Context, tenant string, entity types.Entity, headers map[string][]string) (*ngsild.CreateEntityResult, error)
}

type EntityQuerier interface {
	QueryEntities(ctx context.Context, tenant string, entityTypes, entityAttributes []string, query string, headers map[string][]string) (*ngsild.QueryEntitiesResult, error)
}

type EntityRetriever interface {
	RetrieveEntity(ctx context.Context, tenant, entityID string, headers map[string][]string) (types.Entity, error)
}

type ContextInformationManager interface {
	EntityAttributesUpdater
	EntityCreator
	EntityQuerier
	EntityRetriever

	Start() error
	Stop() error
}

type ContextBroker interface {
}

type ContextSource interface {
}
