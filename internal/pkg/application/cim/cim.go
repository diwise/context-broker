package cim

import (
	"context"
	"io"
)

type EntityCreator interface {
	CreateEntity(ctx context.Context, tenant, entityType, entityID string, body io.Reader) (*CreateEntityResult, error)
}

type ContextInformationManager interface {
	EntityCreator
}

type ContextBroker interface {
}

type ContextSource interface {
}
