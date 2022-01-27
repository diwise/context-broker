package contextbroker

import (
	"io"

	"github.com/diwise/ngsi-ld-context-broker/internal/pkg/application/cim"
	"github.com/rs/zerolog"
)

type contextBrokerApp struct {
	log zerolog.Logger
}

func New(log zerolog.Logger) cim.ContextInformationManager {
	return &contextBrokerApp{
		log: log,
	}
}

func (app *contextBrokerApp) CreateEntity(tenant, entityType, entityID string, body io.Reader) (*cim.CreateEntityResult, error) {
	return cim.NewCreateEntityResult("somelocation"), nil
}
