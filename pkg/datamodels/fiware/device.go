package fiware

import (
	"fmt"
	"strings"

	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
)

//NewDevice creates a new instance of Device
func NewDevice(entityID string, decorators ...entities.EntityDecoratorFunc) (types.Entity, error) {

	if len(decorators) == 0 {
		return nil, fmt.Errorf("at least one property must be set in a device entity")
	}

	if !strings.HasPrefix(entityID, DeviceIDPrefix) {
		entityID = DeviceIDPrefix + entityID
	}

	decorators = append(decorators, entities.DefaultContext())

	e, err := entities.New(
		entityID, DeviceTypeName,
		decorators...,
	)

	return e, err
}
