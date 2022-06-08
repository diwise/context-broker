package contextbroker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	"github.com/matryer/is"
)

func TestNewWithEmptyConfig(t *testing.T) {
	is := is.New(t)

	_, err := New(context.Background(), withEmptyConfig())

	is.NoErr(err)
}

func TestNewWithDefaultConfig(t *testing.T) {
	is := is.New(t)

	_, err := New(context.Background(), withDefaultTestConfig(""))
	is.NoErr(err)
}

func TestThatCreateEntityWithUnknownTenantFails(t *testing.T) {
	is := is.New(t)

	broker, err := New(context.Background(), withDefaultTestConfig(""))
	is.NoErr(err)

	_, err = broker.CreateEntity(context.Background(), "unknown", testEntity("", ""), nil)

	is.True(err != nil) // should have returned an error
}

func TestThatCreateEntityWithUnknownEntityTypeFails(t *testing.T) {
	is := is.New(t)

	broker, err := New(context.Background(), withDefaultTestConfig(""))
	is.NoErr(err)

	_, err = broker.CreateEntity(context.Background(), "testtenant", testEntity("Unknown", "id"), nil)
	is.True(err != nil) // should have returned an error
}

func TestThatCreateEntityWithMismatchingIDFails(t *testing.T) {
	is := is.New(t)

	broker, err := New(context.Background(), withDefaultTestConfig(""))
	is.NoErr(err)

	_, err = broker.CreateEntity(context.Background(), "testtenant", testEntity("Device", "badid"), nil)
	is.True(err != nil) // should have returned an error
}

func TestThatCreateEntityWithMatchingTypeAndIDWorks(t *testing.T) {
	is := is.New(t)
	ts := setupMockContextSourceResponse(http.StatusCreated, [][2]string{
		{"Content-Type", "application/ld+json"}, {"Location", "testlocation"},
	}, "")
	defer ts.Close()

	broker, err := New(context.Background(), withDefaultTestConfig(ts.URL))
	is.NoErr(err)

	_, err = broker.CreateEntity(context.Background(), "testtenant", testEntity("Device", "urn:ngsi-ld:Device:testid"), nil)
	is.NoErr(err) // should not return an error
}

func TestThatNotificationsAreSent_ThisTestShouldBeBrokenUp(t *testing.T) {
	is := is.New(t)
	ts := setupMockContextSourceResponse(http.StatusCreated, [][2]string{
		{"Content-Type", "application/ld+json"}, {"Location", "testlocation"},
	}, "")
	defer ts.Close()

	numberOfNotifications := 0
	ns := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		numberOfNotifications++
		w.WriteHeader(http.StatusOK)
	}))
	defer ns.Close()

	t.Setenv("NOTIFIER_ENDPOINT", ns.URL)

	broker, err := New(context.Background(), withDefaultTestConfig(ts.URL))
	is.NoErr(err)

	broker.Start()

	_, err = broker.CreateEntity(context.Background(), "testtenant", testEntity("Device", "urn:ngsi-ld:Device:testid"), nil)
	is.NoErr(err) // should not return an error

	broker.Stop()

	is.Equal(numberOfNotifications, 1)
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

func testEntity(entityType, entityID string) types.Entity {
	e, _ := entities.New(entityID, entityType)
	return e
}
