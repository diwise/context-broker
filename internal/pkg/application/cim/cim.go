package cim

import (
	"context"
	"encoding/json"
	"io"
)

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

type Entity interface {
	ID() string
	Type() string

	ForEachAttribute(func(attributeType, attributeName string, contents interface{})) error
}

func NewEntity(body string) Entity {
	return &EntityImpl{
		contents: []byte(body),
	}
}

type EntityImpl struct {
	contents []byte
}

func (e EntityImpl) ID() string {
	value := struct {
		ID string `json:"id"`
	}{}

	if json.Unmarshal(e.contents, &value) != nil {
		return ""
	}

	return value.ID
}

func (e EntityImpl) Type() string {
	value := struct {
		Type string `json:"type"`
	}{}

	if json.Unmarshal(e.contents, &value) != nil {
		return ""
	}

	return value.Type
}

func (e EntityImpl) ForEachAttribute(callback func(attributeType, attributeName string, contents interface{})) error {
	props := map[string]interface{}{}
	err := json.Unmarshal(e.contents, &props)
	if err != nil {
		return err
	}

	for k, v := range props {
		obj, ok := v.(map[string]interface{})
		if !ok {
			continue
		}

		objType, ok := obj["type"].(string)
		if !ok {
			continue
		}

		if objType == "Property" || objType == "Relationship" || objType == "GeoProperty" {
			callback(objType, k, v)
		}
	}

	return nil
}

func (e *EntityImpl) MarshalJSON() ([]byte, error) {
	return e.contents, nil
}

func (e *EntityImpl) UnmarshalJSON(data []byte) error {
	e.contents = data
	return nil
}
