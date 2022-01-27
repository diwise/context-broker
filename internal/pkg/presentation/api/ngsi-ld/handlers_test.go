package ngsild

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diwise/ngsi-ld-context-broker/internal/pkg/application/cim"
	"github.com/go-chi/chi/v5"
	"github.com/matryer/is"
	"github.com/rs/zerolog/log"
)

func TestCreateEntity(t *testing.T) {
	is, ts, _ := setupTest(t)
	defer ts.Close()

	resp, _ := newTestRequest(is, ts, "POST", "/ngsi-ld/v1/entities", bytes.NewBuffer([]byte(entityJSON)))

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

	resp, _ := newTestRequest(is, ts, "POST", "/ngsi-ld/v1/entities", bytes.NewBuffer([]byte("this is not my json")))

	is.Equal(resp.StatusCode, http.StatusBadRequest) // Check status code
}

func TestCreateEntityCanHandleAlreadyExistsError(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	app.CreateEntityFunc = func(context.Context, string, string, string, io.Reader) (*cim.CreateEntityResult, error) {
		return nil, cim.NewAlreadyExistsError()
	}

	resp, _ := newTestRequest(is, ts, "POST", "/ngsi-ld/v1/entities", bytes.NewBuffer([]byte(entityJSON)))

	is.Equal(resp.StatusCode, http.StatusConflict) // Check status code
}

func TestCreateEntityCanHandleInternalError(t *testing.T) {
	is, ts, app := setupTest(t)
	defer ts.Close()

	app.CreateEntityFunc = func(context.Context, string, string, string, io.Reader) (*cim.CreateEntityResult, error) {
		return nil, fmt.Errorf("some unknown error")
	}

	resp, _ := newTestRequest(is, ts, "POST", "/ngsi-ld/v1/entities", bytes.NewBuffer([]byte(entityJSON)))

	is.Equal(resp.StatusCode, http.StatusInternalServerError) // Check status code
}

func newTestRequest(is *is.I, ts *httptest.Server, method, path string, body io.Reader) (*http.Response, string) {
	req, _ := http.NewRequest(method, ts.URL+path, body)
	req.Header.Add("Content-Type", "application/ld+json")

	resp, err := http.DefaultClient.Do(req)
	is.NoErr(err) // http request failed
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	is.NoErr(err) // failed to read response body

	return resp, string(respBody)
}

func setupTest(t *testing.T) (*is.I, *httptest.Server, *cim.ContextInformationManagerMock) {
	is := is.New(t)
	r := chi.NewRouter()
	ts := httptest.NewServer(r)

	log := log.Logger
	app := &cim.ContextInformationManagerMock{
		CreateEntityFunc: func(ctx context.Context, tenant, entityType, entityID string, body io.Reader) (*cim.CreateEntityResult, error) {
			return cim.NewCreateEntityResult("somewhere"), nil
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
