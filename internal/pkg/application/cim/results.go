package cim

type CreateEntityResult struct {
	location string
}

func NewCreateEntityResult(location string) *CreateEntityResult {
	return &CreateEntityResult{location: location}
}

func (r CreateEntityResult) Location() string {
	return r.location
}
