package cim

import "io"

type EntityCreator interface {
	CreateEntity(tenant, entityType, entityID string, body io.Reader) (*CreateEntityResult, error)
}

type ContextInformationManager interface {
	EntityCreator
}

type ContextBroker interface {
}

type ContextSource interface {
}
