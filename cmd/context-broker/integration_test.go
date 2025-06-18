package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/diwise/service-chassis/pkg/infrastructure/servicerunner"
	testutils "github.com/diwise/service-chassis/pkg/test/http"
	"github.com/diwise/service-chassis/pkg/test/http/expects"
	"github.com/diwise/service-chassis/pkg/test/http/response"

	"github.com/matryer/is"
)

var Expects = testutils.Expects
var Returns = testutils.Returns
var method = expects.RequestMethod
var path = expects.RequestPath

var dowork = servicerunner.WithWorker[AppConfig]

func DefaultTestFlags() FlagMap {
	return FlagMap{
		listenAddress: "",  // listen on all ipv4 and ipv6 interfaces
		servicePort:   "0", //
		controlPort:   "",  // control port disabled by default

		logFormat: "json",
	}
}

func TestIntegrateRetriveTemporalEvolutionOfEntity(t *testing.T) {
	is := is.New(t)
	ctx, cancelTest := context.WithCancel(t.Context())

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

	app, err := initialize(ctx, DefaultTestFlags(), &AppConfig{
		brokerConfig: newTestConfig(ms.URL()),
		opaConfig:    newAuthConfig(),
	})
	is.NoErr(err)

	app.Run(ctx, dowork(func(ctx context.Context, appConfig *AppConfig) error {
		defer cancelTest()

		response, responseBody := testRequest(appConfig.publicPort, http.MethodGet, "/ngsi-ld/v1/temporal/entities/urn:ngsi-ld:Vehicle:B9211?timerel=after&timeAt=2022-02-13T21:33:42Z", nil)

		is.True(response != nil)
		is.Equal(response.StatusCode, http.StatusOK)
		is.Equal(responseBody, temporalResponseBody)

		return nil
	}))
}

func TestIntegrateQueryTemporalEvolutionOfEntities(t *testing.T) {
	is := is.New(t)
	ctx, cancelTest := context.WithCancel(t.Context())

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

	app, err := initialize(ctx, DefaultTestFlags(), &AppConfig{
		brokerConfig: newTestConfig(ms.URL()),
		opaConfig:    newAuthConfig(),
	})
	is.NoErr(err)

	app.Run(ctx, dowork(func(ctx context.Context, appConfig *AppConfig) error {
		defer cancelTest()

		response, responseBody := testRequest(appConfig.publicPort, http.MethodGet, "/ngsi-ld/v1/temporal/entities?timerel=between&timeAt=2022-01-01T00:00:00Z&endTimeAt=2022-02-01T00:00:00Z", nil)

		is.True(response != nil)
		is.Equal(response.StatusCode, http.StatusOK)
		is.Equal(responseBody, responseBody)

		return nil
	}))
}

func testRequest(port, method, path string, body io.Reader) (*http.Response, string) {
	req, _ := http.NewRequest(method, "http://127.0.0.1:"+port+path, body)
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
