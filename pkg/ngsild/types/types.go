package types

type EntityFragment interface {
	ForEachAttribute(func(attributeType, attributeName string, contents any)) error
	MarshalJSON() ([]byte, error)
}

type Entity interface {
	EntityFragment

	ID() string
	Type() string
}

type Property interface {
	Type() string
}

type Relationship interface {
	Type() string
}
