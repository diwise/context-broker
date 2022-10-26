package ngsild

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diwise/context-broker/internal/pkg/application/cim"
	"github.com/diwise/context-broker/pkg/datamodels/fiware"
	"github.com/diwise/context-broker/pkg/ngsild"
	"github.com/diwise/context-broker/pkg/ngsild/errors"
	ngsitypes "github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	. "github.com/diwise/context-broker/pkg/ngsild/types/entities/decorators"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/matryer/is"
	"github.com/rs/zerolog/log"
)

const (
	ld_json  string = "application/ld+json"
	geo_json string = "application/geo+json"
)

var acceptJSONLD = [][]string{{"Accept", ld_json}}
var acceptJSON = [][]string{{"Accept", "application/json"}}
var jsonLDContent = [][]string{{"Content-Type", ld_json}}

func TestCreateEntity(t *testing.T) {
	is, ts, _ := setupTest(t)
	defer ts.Close()

	resp, _ := testRequest(is, ts, http.MethodPost, jsonLDContent, "/ngsi-ld/v1/entities", bytes.NewBuffer([]byte(entityJSON)))

	is.Equal(resp.StatusCode, http.StatusCreated) // Check status code
}

func TestCreateEntityWithWrongContentTypeReturnsUnsupportedMediaType(t *testing.T) {
	is, ts, _ := setupTest(t)
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/ngsi-ld/v1/entities", bytes.NewBuffer([]byte(entityJSON)))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	is.NoErr(err) // http request failed
	defer resp.Body.Close()

	is.Equal(resp.StatusCode, http.StatusUnsupportedMediaType) // Check status code
}

func TestCreateEntityWithBadDataReturnsInvalidRequest(t *testing.T) {
	is, ts, _ := setupTest(t)
	defer ts.Close()

	resp, _ := testRequest(is, ts, http.MethodPost, jsonLDContent, "/ngsi-ld/v1/entities", bytes.NewBuffer([]byte("this is not my json")))

	is.Equal(resp.StatusCode, http.StatusBadRequest) // Check status code
}

func TestCreateEntityCanHandleAlreadyExistsError(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	app.CreateEntityFunc = func(context.Context, string, ngsitypes.Entity, map[string][]string) (*ngsild.CreateEntityResult, error) {
		return nil, errors.NewAlreadyExistsError("already exists")
	}

	resp, _ := testRequest(is, ts, http.MethodPost, jsonLDContent, "/ngsi-ld/v1/entities", bytes.NewBuffer([]byte(entityJSON)))

	is.Equal(resp.StatusCode, http.StatusConflict) // Check status code
}

func TestCreateEntityCanHandleInternalError(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	app.CreateEntityFunc = func(context.Context, string, ngsitypes.Entity, map[string][]string) (*ngsild.CreateEntityResult, error) {
		return nil, fmt.Errorf("some unknown error")
	}

	resp, _ := testRequest(is, ts, http.MethodPost, jsonLDContent, "/ngsi-ld/v1/entities", bytes.NewBuffer([]byte(entityJSON)))

	is.Equal(resp.StatusCode, http.StatusInternalServerError) // Check status code
}

func TestQueryEntitiesWithNoTypesOrAttrsReturnsBadRequest(t *testing.T) {
	is, ts, _ := setupTest(t)
	defer ts.Close()

	resp, _ := testRequest(is, ts, http.MethodGet, acceptJSONLD, "/ngsi-ld/v1/entities", nil)

	is.Equal(resp.StatusCode, http.StatusBadRequest) // Check status code
}

func TestQueryEntitiesForwardsSingleType(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	_, _ = testRequest(is, ts, http.MethodGet, acceptJSONLD, "/ngsi-ld/v1/entities?type=A", nil)

	is.Equal(len(app.QueryEntitiesCalls()), 1)
	is.Equal(len(app.QueryEntitiesCalls()[0].EntityTypes), 1)
	is.Equal(app.QueryEntitiesCalls()[0].EntityTypes[0], "A")
}

func TestQueryEntitiesForwardsMultipleTypes(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	_, _ = testRequest(is, ts, http.MethodGet, acceptJSONLD, "/ngsi-ld/v1/entities?type=A,B,C", nil)

	is.Equal(len(app.QueryEntitiesCalls()), 1)
	is.Equal(len(app.QueryEntitiesCalls()[0].EntityTypes), 3)
	is.Equal(app.QueryEntitiesCalls()[0].EntityTypes[2], "C")
}

func TestQueryEntitiesForwardsCorrectPathAndQuery(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	pathAndQuery := "/ngsi-ld/v1/entities?type=A,B,C"
	_, _ = testRequest(is, ts, http.MethodGet, acceptJSONLD, pathAndQuery, nil)

	is.Equal(len(app.QueryEntitiesCalls()), 1)
	is.Equal(app.QueryEntitiesCalls()[0].Query, pathAndQuery)
}

func TestQueryEntities(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	app.QueryEntitiesFunc = func(ctx context.Context, tenant string, types []string, attrs []string, q string, h map[string][]string) (*ngsild.QueryEntitiesResult, error) {
		qer := ngsild.NewQueryEntitiesResult()
		go func() {
			e, _ := fiware.NewWeatherObserved(
				"Spain-WeatherObserved-Valladolid-2016-11-30T07:00:00.00Z",
				41.640833333, -4.754444444,
				"2016-11-30T07:00:00.00Z",
				Temperature(3.3),
			)
			qer.Found <- e
			qer.Found <- nil
		}()
		return qer, nil
	}

	_, _ = testRequest(is, ts, http.MethodGet, acceptJSONLD, "/ngsi-ld/v1/entities?type=A", nil)

	is.Equal(len(app.QueryEntitiesCalls()), 1)
	//is.Equal(responseBody, "test")
}

func TestQueryEntitiesAsGeoJSON(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	app.QueryEntitiesFunc = func(ctx context.Context, tenant string, types []string, attrs []string, q string, h map[string][]string) (*ngsild.QueryEntitiesResult, error) {
		qer := ngsild.NewQueryEntitiesResult()
		go func() {
			e, _ := fiware.NewWeatherObserved(
				"Spain-WeatherObserved-Valladolid-2016-11-30T07:00:00.00Z",
				41.640833333, -4.754444444,
				"2016-11-30T07:00:00.00Z",
				Temperature(3.3),
			)
			qer.Found <- e
			qer.Found <- nil
		}()
		return qer, nil
	}

	_, responseBody := testRequest(is, ts, http.MethodGet, [][]string{{"Accept", geo_json}}, "/ngsi-ld/v1/entities?type=A", nil)

	is.Equal(responseBody, weatherObservedGeoJson)
}

func TestUpdateEntityAttributes(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	app.UpdateEntityAttributesFunc = func(ctx context.Context, tenant, entityID string, fragment ngsitypes.EntityFragment, h map[string][]string) (*ngsild.UpdateEntityAttributesResult, error) {
		return &ngsild.UpdateEntityAttributesResult{
			Updated: []string{entityID},
		}, nil
	}

	fragment, err := entities.NewFragment(Status("off"))
	is.NoErr(err)

	body, err := fragment.MarshalJSON()
	is.NoErr(err)

	resp, _ := testRequest(is, ts, http.MethodPatch, jsonLDContent, "/ngsi-ld/v1/entities/idtobepatched/attrs/", bytes.NewBuffer(body))

	is.Equal(resp.StatusCode, http.StatusNoContent) // should return 204 No Content
}

func TestUpdateEntityAttributesWithPropertyMetadata(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	app.UpdateEntityAttributesFunc = func(ctx context.Context, tenant, entityID string, fragment ngsitypes.EntityFragment, h map[string][]string) (*ngsild.UpdateEntityAttributesResult, error) {
		const expectedFragmentJSON string = `{"@context":["https://raw.githubusercontent.com/diwise/context-broker/main/assets/jsonldcontexts/default-context.jsonld"],"waterConsumption":{"type":"Property","value":100,"observedAt":"2006-01-02T15:04:05Z","observedBy":{"type":"Relationship","object":"some_device"},"unitCode":"LTR"}}`

		reqb, err := json.Marshal(fragment)
		is.NoErr(err)
		is.Equal(string(reqb), expectedFragmentJSON)

		return &ngsild.UpdateEntityAttributesResult{
			Updated: []string{entityID},
		}, nil
	}

	body := []byte("{\"@context\":[\"https://raw.githubusercontent.com/diwise/context-broker/main/assets/jsonldcontexts/default-context.jsonld\"],\"waterConsumption\":{\"type\":\"Property\",\"value\":100,\"observedAt\":\"2006-01-02T15:04:05Z\",\"observedBy\":{\"type\":\"Relationship\",\"object\":\"some_device\"},\"unitCode\":\"LTR\"}}")

	resp, _ := testRequest(is, ts, http.MethodPatch, jsonLDContent, "/ngsi-ld/v1/entities/idtobepatched/attrs/", bytes.NewBuffer(body))

	is.Equal(resp.StatusCode, http.StatusNoContent) // should return 204 No Content
}
func TestRequestDefaultContext(t *testing.T) {
	is, ts, _ := setupTest(t)
	defer ts.Close()

	resp, _ := testRequest(is, ts, http.MethodGet, acceptJSON, "/ngsi-ld/v1/jsonldContexts/default-context.jsonld", nil)

	is.Equal(resp.StatusCode, http.StatusOK)
}

func TestRequestUnknownContextFailsWith404(t *testing.T) {
	is, ts, _ := setupTest(t)
	defer ts.Close()

	resp, _ := testRequest(is, ts, http.MethodGet, acceptJSON, "/ngsi-ld/v1/jsonldContexts/unknown-context.jsonld", nil)

	is.Equal(resp.StatusCode, http.StatusNotFound)
}

func testRequest(is *is.I, ts *httptest.Server, method string, headers [][]string, path string, body io.Reader) (*http.Response, string) {
	req, err := http.NewRequest(method, ts.URL+path, body)
	is.NoErr(err)

	for _, hdr := range headers {
		req.Header.Add(hdr[0], hdr[1])
	}

	resp, err := http.DefaultClient.Do(req)
	is.NoErr(err) // http request failed
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	is.NoErr(err) // failed to read response body

	return resp, string(respBody)
}

func setupTest(t *testing.T) (*is.I, *httptest.Server, *cim.ContextInformationManagerMock) {
	is := is.New(t)
	r := chi.NewRouter()
	ts := httptest.NewServer(r)

	log := log.Logger
	app := &cim.ContextInformationManagerMock{
		CreateEntityFunc: func(ctx context.Context, tenant string, entity ngsitypes.Entity, h map[string][]string) (*ngsild.CreateEntityResult, error) {
			return ngsild.NewCreateEntityResult("somewhere"), nil
		},
		QueryEntitiesFunc: func(ctx context.Context, tenant string, types []string, attrs []string, q string, h map[string][]string) (*ngsild.QueryEntitiesResult, error) {
			qer := ngsild.NewQueryEntitiesResult()
			go func() { qer.Found <- nil }()
			return qer, nil
		},
	}

	policies := bytes.NewBufferString(opaModule)
	RegisterHandlers(r, policies, app, log)

	return is, ts, app
}

func createJWTWithTenants(tenants []string) string {
	tokenAuth := jwtauth.New("HS256", []byte("secret"), nil)
	_, tokenString, _ := tokenAuth.Encode(map[string]any{"user_id": 123, "azp": "diwise-frontend", "tenants": tenants})
	return "Bearer " + tokenString
}

var entityJSON string = `{
    "id": "urn:ngsi-ld:Device:testdevice",
    "type": "Device",
    "@context": [
        "https://schema.lab.fiware.org/ld/context",
        "https://uri.etsi.org/ngsi-ld/v1/ngsi-ld-core-context.jsonld"
    ]
}`

var weatherObservedGeoJson string = `{"type":"FeatureCollection","features":[{"id":"urn:ngsi-ld:WeatherObserved:Spain-WeatherObserved-Valladolid-2016-11-30T07:00:00.00Z","type":"Feature","geometry":{"type":"Point","coordinates":[-4.754444444,41.640833333]},"properties":{"dateObserved":{"type":"Property","value":{"@type":"DateTime","@value":"2016-11-30T07:00:00.00Z"}},"location":{"type":"GeoProperty","value":{"type":"Point","coordinates":[-4.754444444,41.640833333]}},"temperature":{"type":"Property","value":3.3},"type":"WeatherObserved"}}],"@context":["https://schema.lab.fiware.org/ld/context","https://uri.etsi.org/ngsi-ld/v1/ngsi-ld-core-context.jsonld"]}`

const opaModule string = `
#
# Use https://play.openpolicyagent.org for easier editing/validation of this policy file
#

package example.authz

default allow := false

allow = response {
	response := {
		"ok": true
	}
}
`
