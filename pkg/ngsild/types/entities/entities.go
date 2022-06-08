package entities

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/diwise/context-broker/pkg/ngsild/geojson"
	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/properties"
	"github.com/diwise/context-broker/pkg/ngsild/types/relationships"
)

type EntityDecoratorFunc func(e *EntityImpl)

func New(entityID, entityType string, decorators ...EntityDecoratorFunc) (types.Entity, error) {
	e := &EntityImpl{
		entityID:      entityID,
		entityType:    entityType,
		properties:    map[string]types.Property{},
		relationships: map[string]types.Relationship{},
	}

	for _, decorator := range decorators {
		decorator(e)
	}

	// Set the default context if it wasnt decorated by the creator
	if e.context == nil {
		e.context = []string{DefaultContextURL}
	}

	return e, nil
}

func NewFragment(decorators ...EntityDecoratorFunc) (types.EntityFragment, error) {
	e := &EntityImpl{
		properties:    map[string]types.Property{},
		relationships: map[string]types.Relationship{},
	}

	for _, decorator := range decorators {
		decorator(e)
	}

	// Set the default context if it wasnt decorated by the creator
	if e.context == nil {
		e.context = []string{DefaultContextURL}
	}

	return e, nil
}

func NewFragmentFromJSON(body []byte) (types.EntityFragment, error) {
	e := &EntityImpl{}
	err := json.Unmarshal(body, e)

	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal entity: %w", err)
	}

	return e, nil
}

func NewFromJSON(body []byte) (types.Entity, error) {
	e := &EntityImpl{}
	err := json.Unmarshal(body, e)

	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal entity: %w", err)
	}

	if e.ID() == "" || e.Type() == "" {
		return nil, fmt.Errorf("failed to parse entity")
	}

	return e, nil
}

func NewFromSlice(body []byte) ([]types.Entity, error) {
	impls := []EntityImpl{}
	err := json.Unmarshal(body, &impls)
	if err != nil {
		return nil, err
	}

	arr := make([]types.Entity, 0, len(impls))

	for _, e := range impls {
		arr = append(arr, e)
	}

	return arr, nil
}

type EntityImpl struct {
	entityID   string
	entityType string

	context       []string
	properties    map[string]types.Property
	relationships map[string]types.Relationship
}

func (e EntityImpl) ID() string {
	return e.entityID
}

func (e EntityImpl) Type() string {
	return e.entityType
}

func (e EntityImpl) ForEachAttribute(callback func(attributeType, attributeName string, contents any)) error {

	for k, v := range e.properties {
		callback(v.Type(), k, v)
	}

	for k, v := range e.relationships {
		callback(v.Type(), k, v)
	}

	return nil
}

func (e EntityImpl) MarshalJSON() ([]byte, error) {
	contents := map[string]any{
		"id":   e.ID(),
		"type": e.Type(),
	}

	for k, p := range e.properties {
		contents[k] = p
	}

	for k, r := range e.relationships {
		contents[k] = r
	}

	contents["@context"] = e.context

	return json.Marshal(&contents)
}

func (e *EntityImpl) UnmarshalJSON(data []byte) error {
	var contents map[string]any
	json.Unmarshal(data, &contents)

	header := struct {
		ID      string          `json:"id"`
		Type    string          `json:"type"`
		Context json.RawMessage `json:"@context"`
	}{}

	err := json.Unmarshal(data, &header)
	if err != nil {
		return fmt.Errorf("failed to unmarshal entity: %w", err)
	}

	// Delete the properties we have already dealt with
	delete(contents, "id")
	delete(contents, "type")
	delete(contents, "@context")

	e.entityID = header.ID
	e.entityType = header.Type

	ctxLength := len(header.Context)

	if ctxLength < 2 {
		return fmt.Errorf("invalid context (too short)")
	}

	if bytes.HasPrefix(header.Context, []byte("\"")) && bytes.HasSuffix(header.Context, []byte("\"")) {
		ctxString := string(header.Context[1 : ctxLength-2])
		e.context = []string{ctxString}
	} else if bytes.HasPrefix(header.Context, []byte("[")) && bytes.HasSuffix(header.Context, []byte("]")) {
		e.context = []string{}
		json.Unmarshal(header.Context, &e.context)
	} else {
		return fmt.Errorf("unsupported context: %s", string(header.Context))
	}

	e.properties = map[string]types.Property{}
	e.relationships = map[string]types.Relationship{}

	for k, v := range contents {
		obj, ok := v.(map[string]any)
		if !ok {
			continue
		}

		objType, ok := obj["type"].(string)
		if !ok {
			continue
		}

		if objType == "Property" {
			p, err := properties.UnmarshalP(obj)
			if err != nil {
				return err
			}
			e.properties[k] = p
		} else if objType == "GeoProperty" {
			p, err := geojson.UnmarshalG(obj)
			if err != nil {
				return err
			}
			e.properties[k] = p
		} else if objType == "Relationship" {
			r, err := relationships.UnmarshalR(obj)
			if err != nil {
				return err
			}
			e.relationships[k] = r
		}
	}

	return nil
}

func Context(ctx []string) EntityDecoratorFunc {
	return func(e *EntityImpl) {
		e.context = ctx
	}
}

func DefaultBrokerContext(brokerURL string) EntityDecoratorFunc {
	return Context([]string{brokerURL + "/ngsi-ld/v1/jsonldContexts/default-context.jsonld"})
}

const DefaultContextURL string = "https://raw.githubusercontent.com/diwise/context-broker/main/assets/jsonldcontexts/default-context.jsonld"

func DefaultContext() EntityDecoratorFunc {
	return Context([]string{DefaultContextURL})
}

func P(name string, value types.Property) EntityDecoratorFunc {
	return func(e *EntityImpl) { e.properties[name] = value }
}

func R(name string, value types.Relationship) EntityDecoratorFunc {
	return func(e *EntityImpl) { e.relationships[name] = value }
}
