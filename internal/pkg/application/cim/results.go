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

type QueryEntitiesResult struct {
	Found chan (Entity)
}

func NewQueryEntitiesResult() *QueryEntitiesResult {
	qer := &QueryEntitiesResult{}
	qer.Found = make(chan Entity)
	return qer
}
