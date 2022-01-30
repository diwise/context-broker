package cim

import (
	"context"
	"io"
)

type Entity interface {
}

type EntityCreator interface {
	CreateEntity(ctx context.Context, tenant, entityType, entityID string, body io.Reader) (*CreateEntityResult, error)
}

type EntityQuerier interface {
	QueryEntities(ctx context.Context, tenant string, entityTypes, entityAttributes []string, query string) (*QueryEntitiesResult, error)
}

type ContextInformationManager interface {
	EntityCreator
	EntityQuerier
}

type ContextBroker interface {
}

type ContextSource interface {
}
