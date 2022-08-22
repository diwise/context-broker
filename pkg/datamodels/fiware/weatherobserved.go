package fiware

import (
	"fmt"
	"strings"

	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	. "github.com/diwise/context-broker/pkg/ngsild/types/entities/decorators"
)

// NewWeatherObserved creates a new instance of WeatherObserved
func NewWeatherObserved(observationID string, latitude float64, longitude float64, observedAt string, decorators ...entities.EntityDecoratorFunc) (types.Entity, error) {

	if len(decorators) == 0 {
		return nil, fmt.Errorf("at least one property must be set in a weatherobserved entity")
	}

	if !strings.HasPrefix(observationID, WeatherObservedIDPrefix) {
		observationID = WeatherObservedIDPrefix + observationID
	}

	decorators = append(decorators, DateObserved(observedAt), Location(latitude, longitude))

	e, err := entities.New(
		observationID, WeatherObservedTypeName,
		decorators...,
	)

	return e, err
}
