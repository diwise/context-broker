package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	testutils "github.com/diwise/service-chassis/pkg/test/http"
	"github.com/diwise/service-chassis/pkg/test/http/expects"
	"github.com/diwise/service-chassis/pkg/test/http/response"

	"github.com/matryer/is"
)

var Expects = testutils.Expects
var Returns = testutils.Returns
var method = expects.RequestMethod
var path = expects.RequestPath

func TestIntegrateRetriveTemporalEvolutionOfEntity(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	ms := testutils.NewMockServiceThat(
		Expects(
			is,
			method(http.MethodGet),
			path("/ngsi-ld/v1/temporal/entities/urn:ngsi-ld:Vehicle:B9211"),
			expects.QueryParamEquals("timerel", "after"),
			expects.QueryParamEquals("timeAt", "2022-02-13T21:33:42Z"),
		),
		Returns(
			response.ContentType("application/ld+json"),
			response.Code(http.StatusOK),
			response.Body([]byte(temporalResponseBody)),
		),
	)

	app, r := initialize(ctx, &AppConfig{
		brokerConfig: newTestConfig(ms.URL()),
		opaConfig:    newAuthConfig(),
	})
	app.Start()
	defer app.Stop()

	api := httptest.NewServer(r)
	defer api.Close()

	response, responseBody := testRequest(api.URL, http.MethodGet, "/ngsi-ld/v1/temporal/entities/urn:ngsi-ld:Vehicle:B9211?timerel=after&timeAt=2022-02-13T21:33:42Z", nil)

	is.Equal(response.StatusCode, http.StatusOK)
	is.Equal(responseBody, temporalResponseBody)
}

func TestIntegrateQueryTemporalEvolutionOfEntities(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	responseBody := "[" + temporalResponseBody + "]"

	ms := testutils.NewMockServiceThat(
		Expects(
			is,
			method(http.MethodGet),
			path("/ngsi-ld/v1/temporal/entities"),
			expects.QueryParamEquals("timerel", "between"),
			expects.QueryParamEquals("timeAt", "2022-01-01T00:00:00Z"),
			expects.QueryParamEquals("endTimeAt", "2022-02-01T00:00:00Z"),
		),
		Returns(
			response.ContentType("application/ld+json"),
			response.Code(http.StatusOK),
			response.Body([]byte(responseBody)),
		),
	)

	app, r := initialize(ctx, &AppConfig{
		brokerConfig: newTestConfig(ms.URL()),
		opaConfig:    newAuthConfig(),
	})
	app.Start()
	defer app.Stop()

	api := httptest.NewServer(r)
	defer api.Close()

	response, responseBody := testRequest(api.URL, http.MethodGet, "/ngsi-ld/v1/temporal/entities?timerel=between&timeAt=2022-01-01T00:00:00Z&endTimeAt=2022-02-01T00:00:00Z", nil)

	is.Equal(response.StatusCode, http.StatusOK)
	is.Equal(responseBody, responseBody)
}

func testRequest(url, method, path string, body io.Reader) (*http.Response, string) {
	req, _ := http.NewRequest(method, url+path, body)
	resp, _ := http.DefaultClient.Do(req)
	respBody, _ := io.ReadAll(resp.Body)
	defer resp.Body.Close()

	return resp, string(respBody)
}

func newAuthConfig() io.ReadCloser {
	return io.NopCloser(bytes.NewBufferString(opaModule))
}

func newTestConfig(url string) io.ReadCloser {
	return io.NopCloser(bytes.NewBufferString(fmt.Sprintf(configFileFmt, url, url)))
}

var configFileFmt string = `
tenants:
  - id: default
    name: Kommunen
    contextSources:
    - endpoint: %s
      temporal:
        enabled: true
        endpoint: %s
      information:
      - entities:
        - idPattern: ^urn:ngsi-ld:Vehicle:.+
          type: Vehicle
`

const opaModule string = `
package example.authz

default allow := false

allow = response {
    response := {
    }
}
`

const temporalResponseBody string = `{"@context":["http://example.org/ngsi-ld/latest/vehicle.jsonld","https://uri.etsi.org/ngsi-ld/v1/ngsi-ld-core-context-v1.5.jsonld"],"id":"urn:ngsi-ld:Vehicle:B9211","speed":[{"type":"Property","value":120,"observedAt":"2018-08-01T12:03:00Z"},{"type":"Property","value":80,"observedAt":"2018-08-01T12:05:00Z"},{"type":"Property","value":100,"observedAt":"2018-08-01T12:07:00Z"}],"type":"Vehicle"}`
