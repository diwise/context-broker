package ngsild

import (
	"bytes"
	"context"
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
	"github.com/matryer/is"
	"github.com/rs/zerolog/log"
)

const (
	ld_json  string = "application/ld+json"
	geo_json string = "application/geo+json"
)

func TestCreateEntity(t *testing.T) {
	is, ts, _ := setupTest(t)
	defer ts.Close()

	resp, _ := newPostRequest(is, ts, ld_json, "/ngsi-ld/v1/entities", bytes.NewBuffer([]byte(entityJSON)))

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

	resp, _ := newPostRequest(is, ts, ld_json, "/ngsi-ld/v1/entities", bytes.NewBuffer([]byte("this is not my json")))

	is.Equal(resp.StatusCode, http.StatusBadRequest) // Check status code
}

func TestCreateEntityCanHandleAlreadyExistsError(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	app.CreateEntityFunc = func(context.Context, string, ngsitypes.Entity, map[string][]string) (*ngsild.CreateEntityResult, error) {
		return nil, errors.NewAlreadyExistsError("already exists")
	}

	resp, _ := newPostRequest(is, ts, ld_json, "/ngsi-ld/v1/entities", bytes.NewBuffer([]byte(entityJSON)))

	is.Equal(resp.StatusCode, http.StatusConflict) // Check status code
}

func TestCreateEntityCanHandleInternalError(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	app.CreateEntityFunc = func(context.Context, string, ngsitypes.Entity, map[string][]string) (*ngsild.CreateEntityResult, error) {
		return nil, fmt.Errorf("some unknown error")
	}

	resp, _ := newPostRequest(is, ts, ld_json, "/ngsi-ld/v1/entities", bytes.NewBuffer([]byte(entityJSON)))

	is.Equal(resp.StatusCode, http.StatusInternalServerError) // Check status code
}

func TestQueryEntitiesWithNoTypesOrAttrsReturnsBadRequest(t *testing.T) {
	is, ts, _ := setupTest(t)
	defer ts.Close()

	resp, _ := newGetRequest(is, ts, ld_json, "/ngsi-ld/v1/entities", nil)

	is.Equal(resp.StatusCode, http.StatusBadRequest) // Check status code
}

func TestQueryEntitiesForwardsSingleType(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	_, _ = newGetRequest(is, ts, ld_json, "/ngsi-ld/v1/entities?type=A", nil)

	is.Equal(len(app.QueryEntitiesCalls()), 1)
	is.Equal(len(app.QueryEntitiesCalls()[0].EntityTypes), 1)
	is.Equal(app.QueryEntitiesCalls()[0].EntityTypes[0], "A")
}

func TestQueryEntitiesForwardsMultipleTypes(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	_, _ = newGetRequest(is, ts, ld_json, "/ngsi-ld/v1/entities?type=A,B,C", nil)

	is.Equal(len(app.QueryEntitiesCalls()), 1)
	is.Equal(len(app.QueryEntitiesCalls()[0].EntityTypes), 3)
	is.Equal(app.QueryEntitiesCalls()[0].EntityTypes[2], "C")
}

func TestQueryEntitiesForwardsCorrectPathAndQuery(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	pathAndQuery := "/ngsi-ld/v1/entities?type=A,B,C"
	_, _ = newGetRequest(is, ts, ld_json, pathAndQuery, nil)

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

	_, _ = newGetRequest(is, ts, ld_json, "/ngsi-ld/v1/entities?type=A", nil)

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

	_, responseBody := newGetRequest(is, ts, geo_json, "/ngsi-ld/v1/entities?type=A", nil)

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

	resp, _ := newPatchRequest(is, ts, "application/ld+json", "/ngsi-ld/v1/entities/idtobepatched/attrs/", bytes.NewBuffer(body))

	is.Equal(resp.StatusCode, http.StatusNoContent) // should return 204 No Content
}

func TestRequestDefaultContext(t *testing.T) {
	is, ts, _ := setupTest(t)
	defer ts.Close()

	resp, _ := newGetRequest(is, ts, "application/json", "/ngsi-ld/v1/jsonldContexts/default-context.jsonld", nil)

	is.Equal(resp.StatusCode, http.StatusOK)
}

func TestRequestUnknownContextFailsWith404(t *testing.T) {
	is, ts, _ := setupTest(t)
	defer ts.Close()

	resp, _ := newGetRequest(is, ts, "application/json", "/ngsi-ld/v1/jsonldContexts/unknown-context.jsonld", nil)

	is.Equal(resp.StatusCode, http.StatusNotFound)
}

func newGetRequest(is *is.I, ts *httptest.Server, accept, path string, body io.Reader) (*http.Response, string) {
	req, err := http.NewRequest(http.MethodGet, ts.URL+path, body)
	is.NoErr(err)

	req.Header.Add("Accept", accept)

	resp, err := http.DefaultClient.Do(req)
	is.NoErr(err) // http request failed
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	is.NoErr(err) // failed to read response body

	return resp, string(respBody)
}

func newPatchRequest(is *is.I, ts *httptest.Server, contentType, path string, body io.Reader) (*http.Response, string) {
	req, _ := http.NewRequest(http.MethodPatch, ts.URL+path, body)
	req.Header.Add("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	is.NoErr(err) // http request failed
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	is.NoErr(err) // failed to read response body

	return resp, string(respBody)
}

func newPostRequest(is *is.I, ts *httptest.Server, contentType, path string, body io.Reader) (*http.Response, string) {
	req, _ := http.NewRequest(http.MethodPost, ts.URL+path, body)
	req.Header.Add("Content-Type", contentType)

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

	RegisterHandlers(r, app, log)

	return is, ts, app
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
