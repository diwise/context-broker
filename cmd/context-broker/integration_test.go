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
	"github.com/rs/zerolog"
)

var Expects = testutils.Expects
var Returns = testutils.Returns
var anyInput = expects.AnyInput
var method = expects.RequestMethod
var path = expects.RequestPath
var body = expects.RequestBody

func TestIntegration(t *testing.T) {
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

	app, r := initialize(ctx, zerolog.Logger{}, newTestConfig(ms.URL()), newAuthConfig())
	app.Start()
	defer app.Stop()

	api := httptest.NewServer(r)
	defer api.Close()

	response, responseBody := testRequest(api.URL, http.MethodGet, "/ngsi-ld/v1/temporal/entities/urn:ngsi-ld:Vehicle:B9211?timerel=after&timeAt=2022-02-13T21:33:42Z", nil)

	is.Equal(response.StatusCode, http.StatusOK)
	is.Equal(responseBody, temporalResponseBody)
}

func testRequest(url, method, path string, body io.Reader) (*http.Response, string) {
	req, _ := http.NewRequest(method, url+path, body)
	resp, _ := http.DefaultClient.Do(req)
	respBody, _ := io.ReadAll(resp.Body)
	defer resp.Body.Close()

	return resp, string(respBody)
}

func newAuthConfig() io.Reader {
	return bytes.NewBufferString(opaModule)
}

func newTestConfig(url string) io.Reader {
	return bytes.NewBufferString(fmt.Sprintf(configFileFmt, url, url))
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
