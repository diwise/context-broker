package types

type EntityFragment interface {
	ForEachAttribute(func(attributeType, attributeName string, contents any)) error
	MarshalJSON() ([]byte, error)
}

type Entity interface {
	EntityFragment

	ID() string
	Type() string

	KeyValues() EntityKeyValueMapper
}

type EntityTemporal interface {
}

type EntityKeyValueMapper interface {
}

type Property interface {
	Type() string
	Value() any
}

type Relationship interface {
	Type() string
	Object() any
}
