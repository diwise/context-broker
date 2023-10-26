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
	ID() string
	Type() string

	Property(name string) []TemporalProperty
}

type EntityKeyValueMapper interface {
}

type Property interface {
	Type() string
	Value() any
}

type TemporalProperty interface {
	Type() string
	Value() any
	ObservedAt() string
}

type Relationship interface {
	Type() string
	Object() any
}
