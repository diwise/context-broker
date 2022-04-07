package contextbroker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matryer/is"
	"github.com/rs/zerolog/log"
)

func TestNewWithEmptyConfig(t *testing.T) {
	is := is.New(t)

	_, err := New(log.Logger, withEmptyConfig())

	is.NoErr(err)
}

func TestNewWithDefaultConfig(t *testing.T) {
	is := is.New(t)

	_, err := New(log.Logger, withDefaultTestConfig(""))
	is.NoErr(err)
}

func TestThatCreateEntityWithUnknownTenantFails(t *testing.T) {
	is := is.New(t)

	broker, err := New(log.Logger, withDefaultTestConfig(""))
	is.NoErr(err)

	_, err = broker.CreateEntity(context.Background(), "unknown", "Device", "testid", nil, nil)
	is.True(err != nil) // should have returned an error
}

func TestThatCreateEntityWithUnknownEntityTypeFails(t *testing.T) {
	is := is.New(t)

	broker, err := New(log.Logger, withDefaultTestConfig(""))
	is.NoErr(err)

	_, err = broker.CreateEntity(context.Background(), "testtenant", "Unknown", "testid", nil, nil)
	is.True(err != nil) // should have returned an error
}

func TestThatCreateEntityWithMismatchingIDFails(t *testing.T) {
	is := is.New(t)

	broker, err := New(log.Logger, withDefaultTestConfig(""))
	is.NoErr(err)

	_, err = broker.CreateEntity(context.Background(), "testtenant", "Device", "testid", nil, nil)
	is.True(err != nil) // should have returned an error
}

func TestThatCreateEntityWithMatchingTypeAndIDWorks(t *testing.T) {
	is := is.New(t)
	ts := setupMockContextSourceResponse(http.StatusCreated, [][2]string{
		{"Content-Type", "application/ld+json"}, {"Location", "testlocation"},
	}, "")
	defer ts.Close()

	broker, err := New(log.Logger, withDefaultTestConfig(ts.URL))
	is.NoErr(err)

	_, err = broker.CreateEntity(context.Background(), "testtenant", "Device", "urn:ngsi-ld:Device:testid", nil, nil)
	is.NoErr(err) // should not return an error
}

func setupMockContextSourceResponse(responseCode int, headers [][2]string, responseBody string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, hdr := range headers {
			w.Header().Add(hdr[0], hdr[1])
		}

		w.WriteHeader(responseCode)
		w.Write([]byte(responseBody))
	}))
}

func withDefaultTestConfig(endpoint string) Config {
	cfg := Config{
		Tenants: []Tenant{
			{
				ID: "testtenant",
				ContextSources: []ContextSourceConfig{
					{
						Endpoint: endpoint,
						Information: []RegistrationInfo{
							{
								Entities: []EntityInfo{
									{
										IDPattern: "^urn:ngsi-ld:Device:.+",
										Type:      "Device",
									},
								},
							},
							{
								Entities: []EntityInfo{
									{
										IDPattern: "^urn:ngsi-ld:DeviceModel:.+",
										Type:      "DeviceModel",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return cfg
}

func withEmptyConfig() Config {
	return Config{}
}
