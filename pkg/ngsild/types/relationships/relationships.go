package relationships

import (
	"fmt"

	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/properties"
)

//Relationship is a base type for all types of relationships
type RelationshipImpl struct {
	Type string `json:"type"`
}

//SingleObjectRelationship stores information about an entity's relation to a single object
type SingleObjectRelationship struct {
	RelationshipImpl
	Object string `json:"object"`
}

func (sor *SingleObjectRelationship) Type() string {
	return sor.RelationshipImpl.Type
}

//NewSingleObjectRelationship accepts an object ID as a string and returns a new SingleObjectRelationship
func NewSingleObjectRelationship(object string) *SingleObjectRelationship {
	return &SingleObjectRelationship{
		RelationshipImpl: RelationshipImpl{Type: "Relationship"},
		Object:           object,
	}
}

//MultiObjectRelationship stores information about an entity's relation to multiple objects
type MultiObjectRelationship struct {
	RelationshipImpl
	Object []string `json:"object"`
}

func (mor *MultiObjectRelationship) Type() string {
	return mor.RelationshipImpl.Type
}

//NewMultiObjectRelationship accepts an array of object ID:s and returns a new MultiObjectRelationship
func NewMultiObjectRelationship(objects []string) *MultiObjectRelationship {
	p := &MultiObjectRelationship{
		RelationshipImpl: RelationshipImpl{Type: "Relationship"},
	}

	p.Object = objects

	return p
}

func UnmarshalR(body map[string]any) (types.Relationship, error) {
	object, ok := body["object"]
	if !ok {
		return nil, fmt.Errorf("relationships without an object attribute are not supported")
	}

	switch typedObject := object.(type) {
	case string:
		return NewSingleObjectRelationship(typedObject), nil
	case []any:
		objects := []string{}
		for _, o := range typedObject {
			str, ok := o.(string)
			if ok {
				objects = append(objects, str)
			}
		}
		return NewMultiObjectRelationship(objects), nil
	default:
		return properties.NewTextProperty(fmt.Sprintf("support for type %T not implemented", typedObject)), nil
	}
}
