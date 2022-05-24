package types

import (
	"encoding/json"
	"fmt"
)

type Entity interface {
	ID() string
	Type() string

	ForEachAttribute(func(attributeType, attributeName string, contents interface{})) error
	MarshalJSON() ([]byte, error)
}

func NewEntity(body []byte) (Entity, error) {
	e := &EntityImpl{
		contents: body,
	}

	e.entityID = e.ID()
	e.entityType = e.Type()

	if e.entityID == "" || e.entityType == "" {
		return nil, fmt.Errorf("failed to parse entity")
	}

	return e, nil
}

type EntityImpl struct {
	entityID   string
	entityType string
	contents   []byte
}

func (e EntityImpl) ID() string {
	if e.entityID != "" {
		return e.entityID
	}

	value := struct {
		ID string `json:"id"`
	}{}

	if json.Unmarshal(e.contents, &value) != nil {
		return ""
	}

	return value.ID
}

func (e EntityImpl) Type() string {
	if e.entityType != "" {
		return e.entityType
	}

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

func (e EntityImpl) MarshalJSON() ([]byte, error) {
	return e.contents, nil
}

func (e *EntityImpl) UnmarshalJSON(data []byte) error {
	e.contents = data
	return nil
}
