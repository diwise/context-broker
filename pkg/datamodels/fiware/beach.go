package fiware

import (
	"fmt"
	"strings"

	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	dec "github.com/diwise/context-broker/pkg/ngsild/types/entities/decorators"
)

// NewBeach creates a new instance of Beach
func NewBeach(entityID string, name string, decorators ...entities.EntityDecoratorFunc) (types.Entity, error) {

	if len(decorators) == 0 {
		return nil, fmt.Errorf("at least one property must be set in a beach entity")
	}

	if !strings.HasPrefix(entityID, BeachIDPrefix) {
		entityID = BeachIDPrefix + entityID
	}

	decorators = append(decorators, dec.Name(name))

	e, err := entities.New(
		entityID, BeachTypeName,
		decorators...,
	)

	return e, err
}
