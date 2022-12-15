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
		return entities.NewTemporalFromJSON([]byte(indentedTemporalEvolutionOfEntity))
	}

	resp, respBody := testRequest(is, ts, http.MethodGet, acceptJSONLD, "/ngsi-ld/v1/temporal/entities/someid", nil)

	is.Equal(resp.StatusCode, http.StatusOK)
	is.Equal(respBody, temporalEvolutionOfEntity)
}

const temporalEvolutionOfEntity string = `{"@context":["http://example.org/ngsi-ld/latest/vehicle.jsonld","https://uri.etsi.org/ngsi-ld/v1/ngsi-ld-core-context-v1.5.jsonld"],"id":"urn:ngsi-ld:Vehicle:B9211","speed":[{"type":"Property","value":120,"observedAt":"2018-08-01T12:03:00Z"},{"type":"Property","value":80,"observedAt":"2018-08-01T12:05:00Z"},{"type":"Property","value":100,"observedAt":"2018-08-01T12:07:00Z"}],"type":"Vehicle"}`

const indentedTemporalEvolutionOfEntity string = `{
	"id": "urn:ngsi-ld:Vehicle:B9211",
	"type": "Vehicle",
	"speed":[
		{
			"type": "Property",
			"value": 120,
			"observedAt": "2018-08-01T12:03:00Z"
		},
		{
			"type": "Property",
			"value": 80,
			"observedAt": "2018-08-01T12:05:00Z"
		},
		{
			"type": "Property",
			"value": 100,
			"observedAt": "2018-08-01T12:07:00Z"
		}
	],
	"@context":[
		"http://example.org/ngsi-ld/latest/vehicle.jsonld",
		"https://uri.etsi.org/ngsi-ld/v1/ngsi-ld-core-context-v1.5.jsonld"
	]
}
`
