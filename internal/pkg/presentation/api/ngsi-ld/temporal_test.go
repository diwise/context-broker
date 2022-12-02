package ngsild

import (
	"context"
	"net/http"
	"testing"

	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
)

func TestRetrieveTemporalEvolutionOfAnEntity(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	app.RetrieveTemporalEvolutionOfEntityFunc = func(ctx context.Context, tenant string, entityID string, headers map[string][]string) (types.EntityTemporal, error) {
		return entities.NewTemporalFromJSON([]byte(""))
	}

	resp, _ := newGetRequest(is, ts, "application/ld+json", "/ngsi-ld/v1/temporal/entities/someid", nil)

	is.Equal(resp.StatusCode, http.StatusInternalServerError)
}
