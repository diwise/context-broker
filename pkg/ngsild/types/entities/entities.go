package entities

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/diwise/context-broker/pkg/ngsild/geojson"
	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/properties"
	"github.com/diwise/context-broker/pkg/ngsild/types/relationships"
)

type EntityDecoratorFunc func(e *EntityImpl)

func New(entityID, entityType string, decorators ...EntityDecoratorFunc) (types.Entity, error) {
	e := &EntityImpl{
		entityID:      &entityID,
		entityType:    &entityType,
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

func NewTemporalFromJSON(body []byte) (types.EntityTemporal, error) {
	et := &EntityTemporalImpl{}
	err := json.Unmarshal(body, et)

	return et, err
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
	entityID   *string
	entityType *string

	context       []string
	properties    map[string]types.Property
	relationships map[string]types.Relationship
}

func (e EntityImpl) ID() string {
	if e.entityID != nil {
		return *e.entityID
	}
	return ""
}

func (e EntityImpl) Type() string {
	if e.entityType != nil {
		return *e.entityType
	}
	return ""
}

func (e *EntityImpl) RemoveAttribute(predicate func(attributeType, attributeName string, contents any) bool) {
	props := make(map[string]types.Property, len(e.properties))

	for k, v := range e.properties {
		if predicate(v.Type(), k, v) {
			continue
		}
		props[k] = v
	}

	e.properties = props
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

	contents := map[string]any{}

	// Only marshal id if non empty
	if entityID := e.ID(); len(entityID) > 0 {
		contents["id"] = entityID
	}

	// Only marshal type if non empty
	if entityType := e.Type(); len(entityType) > 0 {
		contents["type"] = entityType
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
		ID      *string         `json:"id,omitempty"`
		Type    *string         `json:"type,omitempty"`
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
		ctxString := string(header.Context[1 : ctxLength-1])
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

type EntityTemporalImpl struct {
	entityID   *string
	entityType *string

	context       []string
	properties    map[string][]types.TemporalProperty
	relationships map[string][]types.Relationship
}

func (e EntityTemporalImpl) ID() string {
	if e.entityID != nil {
		return *e.entityID
	}
	return ""
}

func (e EntityTemporalImpl) Type() string {
	if e.entityType != nil {
		return *e.entityType
	}
	return ""
}

func (e EntityTemporalImpl) Property(name string) []types.TemporalProperty {
	return e.properties[name]
}

func (e EntityTemporalImpl) MarshalJSON() ([]byte, error) {

	contents := map[string]any{}

	contents["id"] = e.entityID
	contents["type"] = e.entityType

	for k, p := range e.properties {
		contents[k] = p
	}

	for k, r := range e.relationships {
		contents[k] = r
	}

	contents["@context"] = e.context

	return json.Marshal(&contents)
}

func (e *EntityTemporalImpl) UnmarshalJSON(data []byte) error {
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

	e.entityID = &header.ID
	e.entityType = &header.Type

	ctxLength := len(header.Context)

	if ctxLength < 2 {
		return fmt.Errorf("invalid context (too short)")
	}

	if bytes.HasPrefix(header.Context, []byte("\"")) && bytes.HasSuffix(header.Context, []byte("\"")) {
		ctxString := string(header.Context[1 : ctxLength-1])
		e.context = []string{ctxString}
	} else if bytes.HasPrefix(header.Context, []byte("[")) && bytes.HasSuffix(header.Context, []byte("]")) {
		e.context = []string{}
		json.Unmarshal(header.Context, &e.context)
	} else {
		return fmt.Errorf("unsupported context: %s", string(header.Context))
	}

	e.properties = map[string][]types.TemporalProperty{}
	e.relationships = map[string][]types.Relationship{}

	for k, v := range contents {
		arr, ok := v.([]any)
		if !ok {
			// If type assertion fails it may be because the data source encoded a single
			// item array as an object instead. Add this object to a new slice and continue ...
			obj, ok := v.(map[string]any)
			if !ok {
				continue
			}

			arr = append([]any{}, obj)
		}

		if len(arr) == 0 {
			continue
		}

		for _, tv := range arr {
			obj, ok := tv.(map[string]any)
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
				e.properties[k] = append(e.properties[k], p.(types.TemporalProperty))
			} else if objType == "GeoProperty" {
				p, err := geojson.UnmarshalG(obj)
				if err != nil {
					return err
				}
				e.properties[k] = append(e.properties[k], p.(types.TemporalProperty))
			} else if objType == "Relationship" {
				r, err := relationships.UnmarshalR(obj)
				if err != nil {
					return err
				}
				e.relationships[k] = append(e.relationships[k], r)
			}
		}
	}

	return nil
}

func (e EntityImpl) KeyValues() types.EntityKeyValueMapper {
	return kvMapper{
		e: e,
	}
}

func ValidateFragmentAttributes(fragment types.EntityFragment, expectations map[string]any) (err error) {
	fragment.ForEachAttribute(func(attributeType, attributeName string, contents any) {
		if expect, ok := expectations[attributeName]; ok {
			// Remove the matched expectation from the map
			delete(expectations, attributeName)

			switch v := contents.(type) {
			case *properties.DateTimeProperty:
				{
					expectValue := expect.(string)
					if strings.Compare(expectValue, v.Val.Value) != 0 {
						err = errors.Join(err, fmt.Errorf("attribute %s value \"%s\" != \"%s\"", attributeName, v.Val.Value, expectValue))
					}
				}
			case *properties.NumberProperty:
				{
					decimalPrecision := 3
					var expectValue float64

					switch ev := expect.(type) {
					case int:
						expectValue = float64(ev)
						decimalPrecision = 1
					case float64:
						expectValue = ev
					default:
						err = errors.Join(err, fmt.Errorf("unable to match expected value of %s (unknown type %T)", attributeName, v))
						return
					}

					divider := int64(math.Pow(10, float64(decimalPrecision)))
					delta := 1.0 / float64(divider)

					if math.Abs(v.Val-expectValue) >= delta {
						err = errors.Join(err, fmt.Errorf("attribute %s value %s != %s",
							attributeName,
							strconv.FormatFloat(v.Val, 'f', decimalPrecision, 64),
							strconv.FormatFloat(expectValue, 'f', decimalPrecision, 64)),
						)
					}
				}
			case *properties.TextProperty:
				{
					expectValue := expect.(string)
					if strings.Compare(expectValue, v.Val) != 0 {
						err = errors.Join(err, fmt.Errorf("attribute %s value \"%s\" != \"%s\"", attributeName, v.Val, expectValue))
					}
				}
			default:
				err = errors.Join(err, fmt.Errorf("unable to match expected value of %s (unknown type %T)", attributeName, v))
			}
		}
	})

	for expectedAttribute := range expectations {
		err = errors.Join(fmt.Errorf("expected attribute %s not found in fragment", expectedAttribute))
	}

	return
}

type kvMapper struct {
	e EntityImpl
}

func (mapper kvMapper) MarshalJSON() ([]byte, error) {
	contents := map[string]any{
		"id":   mapper.e.ID(),
		"type": mapper.e.Type(),
	}

	for k, p := range mapper.e.properties {
		contents[k] = p.Value()
	}

	for k, r := range mapper.e.relationships {
		contents[k] = r.Object()
	}

	contents["@context"] = mapper.e.context

	return json.Marshal(&contents)
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
const DefaultNGSITenant string = ""

const LinkHeader string = `<` + DefaultContextURL + `>; rel="http://www.w3.org/ns/json-ld#context"; type="application/ld+json"`

func DefaultContext() EntityDecoratorFunc {
	return Context([]string{DefaultContextURL})
}

func P(name string, value types.Property) EntityDecoratorFunc {
	return func(e *EntityImpl) { e.properties[name] = value }
}

func R(name string, value types.Relationship) EntityDecoratorFunc {
	return func(e *EntityImpl) { e.relationships[name] = value }
}
