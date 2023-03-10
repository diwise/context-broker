package ngsild

import (
	"context"
	"net/http"
	"testing"

	"github.com/diwise/context-broker/internal/pkg/application/cim"
	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	"github.com/matryer/is"
)

func TestRetrieveTemporalEvolutionOfAnEntity(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	app.RetrieveTemporalEvolutionOfEntityFunc = func(ctx context.Context, tenant string, entityID string, params cim.TemporalQueryParams, headers map[string][]string) (types.EntityTemporal, error) {
		return entities.NewTemporalFromJSON([]byte(indentedTemporalEvolutionOfEntity))
	}

	resp, respBody := testRequest(is, ts, http.MethodGet, acceptJSONLD, "/ngsi-ld/v1/temporal/entities/someid", nil)

	is.Equal(resp.StatusCode, http.StatusOK)
	is.Equal(respBody, temporalEvolutionOfEntity)
}

func TestTemporalQueryParamsRequiresValidTimeRel(t *testing.T) {
	is := is.New(t)
	req, _ := http.NewRequest(http.MethodGet, "?timerel=invalid", nil)

	_, err := NewTemporalQueryParamsFromRequest(req)
	is.True(err != nil)
	is.Equal(err.Error(), "temporal relation timerel must be one of ['before', 'between', 'after']")
}

func TestTemporalQueryParamsTimeRelBetweenRequiresEndTime(t *testing.T) {
	is := is.New(t)
	req, _ := http.NewRequest(http.MethodGet, "?timerel=between&timeAt=2023-02-13T15:38:12Z", nil)

	_, err := NewTemporalQueryParamsFromRequest(req)
	is.True(err != nil)
	is.Equal(err.Error(), "temporal queries with relation 'between' must include an endTimeAt parameter")
}

func TestTemporalQueryParamsOptionsAggregatedRequiresSpecifiedMethod(t *testing.T) {
	is := is.New(t)
	req, _ := http.NewRequest(http.MethodGet, "?options=aggregatedValues", nil)

	_, err := NewTemporalQueryParamsFromRequest(req)
	is.True(err != nil)
	is.Equal(err.Error(), "aggregation of temporal values requires that the aggregation method is specified")
}

func TestTemporalQueryParamsTimeRelAfter(t *testing.T) {
	is := is.New(t)
	req, _ := http.NewRequest(http.MethodGet, "?timerel=after&timeAt=2023-02-13T15:38:12Z", nil)

	params, err := NewTemporalQueryParamsFromRequest(req)
	is.NoErr(err)

	relation, found := params.TemporalRelation()
	is.True(found)
	is.Equal(relation, "after")
}

func TestTemporalQueryParamsLastN(t *testing.T) {
	is := is.New(t)
	req, _ := http.NewRequest(http.MethodGet, "?lastN=20", nil)

	params, err := NewTemporalQueryParamsFromRequest(req)
	is.NoErr(err)

	lastN, found := params.LastN()
	is.True(found)
	is.Equal(lastN, uint64(20))
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
