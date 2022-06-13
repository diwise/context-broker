package diwise

import (
	"strings"

	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	ed "github.com/diwise/context-broker/pkg/ngsild/types/entities/decorators"
)

func NewExerciseTrail(id, name string, length float64, description string, decorators ...entities.EntityDecoratorFunc) (types.Entity, error) {
	if !strings.HasPrefix(id, ExerciseTrailIDPrefix) {
		id = ExerciseTrailIDPrefix + id
	}

	decorators = append(decorators, ed.Name(name), ed.Description(description))

	if length > 0.1 {
		decorators = append(decorators, ed.Number("length", length))
	}

	return entities.New(id, ExerciseTrailTypeName, decorators...)
}
