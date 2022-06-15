package relationships

import (
	"fmt"

	"github.com/diwise/context-broker/pkg/ngsild/types"
)

//Relationship is a base type for all types of relationships
type RelationshipImpl struct {
	Type string `json:"type"`
}

//SingleObjectRelationship stores information about an entity's relation to a single object
type SingleObjectRelationship struct {
	RelationshipImpl
	Obj string `json:"object"`
}

func (sor *SingleObjectRelationship) Type() string {
	return sor.RelationshipImpl.Type
}

func (sor *SingleObjectRelationship) Object() any {
	return sor.Obj
}

//NewSingleObjectRelationship accepts an object ID as a string and returns a new SingleObjectRelationship
func NewSingleObjectRelationship(object string) *SingleObjectRelationship {
	return &SingleObjectRelationship{
		RelationshipImpl: RelationshipImpl{Type: "Relationship"},
		Obj:              object,
	}
}

//MultiObjectRelationship stores information about an entity's relation to multiple objects
type MultiObjectRelationship struct {
	RelationshipImpl
	Obj []string `json:"object"`
}

func (mor *MultiObjectRelationship) Type() string {
	return mor.RelationshipImpl.Type
}

func (mor *MultiObjectRelationship) Object() any {
	return mor.Obj
}

//NewMultiObjectRelationship accepts an array of object ID:s and returns a new MultiObjectRelationship
func NewMultiObjectRelationship(objects []string) *MultiObjectRelationship {
	p := &MultiObjectRelationship{
		RelationshipImpl: RelationshipImpl{Type: "Relationship"},
	}

	p.Obj = objects

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
		return NewSingleObjectRelationship(fmt.Sprintf("support for type %T not implemented", typedObject)), nil
	}
}
