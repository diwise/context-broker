package fiware

import (
	"fmt"
	"strings"

	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
)

func NewWaterConsumptionObserved(entityID string, decorators ...entities.EntityDecoratorFunc) (types.Entity, error) {
	if len(decorators) == 0 {
		return nil, fmt.Errorf("at least one property must be set in a device entity")
	}

	if !strings.HasPrefix(entityID, WaterConsumptionObservedIDPrefix) {
		entityID = WaterConsumptionObservedIDPrefix + entityID
	}

	e, err := entities.New(
		entityID, WaterConsumptionObservedTypeName,
		decorators...,
	)

	return e, err
}