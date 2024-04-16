package ngsild

import (
	"encoding/json"
	"time"

	"github.com/diwise/context-broker/pkg/ngsild/types"
)

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

type RetrieveTemporalEvolutionOfEntityResult struct {
	Found        types.EntityTemporal
	ContentRange *ContentRange
}

type ContentRange struct {
	StartTime *time.Time
	EndTime   *time.Time
}

func NewRetrieveTemporalEvolutionOfEntityResult(entity types.EntityTemporal) *RetrieveTemporalEvolutionOfEntityResult {
	return &RetrieveTemporalEvolutionOfEntityResult{
		Found:        entity,
		ContentRange: nil,
	}
}

type QueryEntitiesResult struct {
	Found      chan (types.Entity)
	TotalCount int64
}

func NewQueryEntitiesResult() *QueryEntitiesResult {
	qer := &QueryEntitiesResult{
		Found:      make(chan types.Entity),
		TotalCount: -1,
	}
	return qer
}

type QueryTemporalEntitiesResult struct {
	Found      chan (types.EntityTemporal)
	TotalCount int64
}

func NewQueryTemporalEntitiesResult() *QueryTemporalEntitiesResult {
	qer := &QueryTemporalEntitiesResult{
		Found:      make(chan types.EntityTemporal),
		TotalCount: -1,
	}
	return qer
}

type MergeEntityResult struct {
}

func NewMergeEntityResult(body []byte) (*MergeEntityResult, error) {
	mer := &MergeEntityResult{}
	return mer, nil
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

type DeleteEntityResult struct {
}

func NewDeleteEntityResult() *DeleteEntityResult {
	del := &DeleteEntityResult{}
	return del
}
