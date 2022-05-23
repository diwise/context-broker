package cim

import "encoding/json"

type CreateEntityResult struct {
	location string
}

func NewCreateEntityResult(location string) *CreateEntityResult {
	return &CreateEntityResult{
		location: location,
	}
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

type UpdateEntityAttributesResult struct {
	Updated    []string `json:"updated"`
	NotUpdated []struct {
		AttributeName string `json:"attributeName"`
		Reason        string `json:"reason"`
	} `json:"notUpdated"`
}

func (uear *UpdateEntityAttributesResult) Bytes() []byte {
	b, _ := json.Marshal(uear)
	return b
}

func (uear *UpdateEntityAttributesResult) IsMultiStatus() bool {
	return len(uear.NotUpdated) > 0
}

func NewUpdateEntityAttributesResult(body []byte) (*UpdateEntityAttributesResult, error) {
	uear := &UpdateEntityAttributesResult{}
	if len(body) > 0 {
		err := json.Unmarshal(body, uear)
		if err != nil {
			return nil, err
		}
	}
	return uear, nil
}
