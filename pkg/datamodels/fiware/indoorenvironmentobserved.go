package fiware

import (
	"strings"

	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	ed "github.com/diwise/context-broker/pkg/ngsild/types/entities/decorators"
)

func NewIndoorEnvironmentObserved(id, dateObserved string, decorators ...entities.EntityDecoratorFunc) (types.Entity, error) {
	if !strings.HasPrefix(id, IndoorEnvironmentObservedIDPrefix) {
		id = IndoorEnvironmentObservedIDPrefix + id
	}

	decorators = append(decorators, ed.DateObserved(dateObserved))

	return entities.New(id, IndoorEnvironmentObservedTypeName, decorators...)
}
