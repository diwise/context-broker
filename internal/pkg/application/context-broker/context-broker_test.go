package contextbroker

import (
	"context"
	"net/http"
	"testing"

	cfg "github.com/diwise/context-broker/internal/pkg/application/config"
	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	testutils "github.com/diwise/service-chassis/pkg/test/http"
	"github.com/diwise/service-chassis/pkg/test/http/expects"
	"github.com/diwise/service-chassis/pkg/test/http/response"
	"github.com/matryer/is"
)

func TestNewWithEmptyConfig(t *testing.T) {
	is := is.New(t)

	_, err := New(context.Background(), withEmptyConfig())

	is.NoErr(err)
}

func TestNewWithDefaultConfig(t *testing.T) {
	is := is.New(t)

	_, err := New(context.Background(), withDefaultTestConfig("", ""))
	is.NoErr(err)
}

func TestThatCreateEntityWithUnknownTenantFails(t *testing.T) {
	is := is.New(t)

	broker, err := New(context.Background(), withDefaultTestConfig("", ""))
	is.NoErr(err)

	_, err = broker.CreateEntity(context.Background(), "unknown", testEntity("", ""), nil)

	is.True(err != nil) // should have returned an error
}

func TestThatCreateEntityWithUnknownEntityTypeFails(t *testing.T) {
	is := is.New(t)

	broker, err := New(context.Background(), withDefaultTestConfig("", ""))
	is.NoErr(err)

	_, err = broker.CreateEntity(context.Background(), "testtenant", testEntity("Unknown", "id"), nil)
	is.True(err != nil) // should have returned an error
}

func TestThatCreateEntityWithMismatchingIDFails(t *testing.T) {
	is := is.New(t)

	broker, err := New(context.Background(), withDefaultTestConfig("", ""))
	is.NoErr(err)

	_, err = broker.CreateEntity(context.Background(), "testtenant", testEntity("Device", "badid"), nil)
	is.True(err != nil) // should have returned an error
}

var Expects = testutils.Expects
var Returns = testutils.Returns
var anyInput = expects.AnyInput

func TestThatCreateEntityWithMatchingTypeAndIDWorks(t *testing.T) {
	is := is.New(t)

	s := testutils.NewMockServiceThat(
		Expects(is, anyInput()),
		Returns(
			response.ContentType("application/ld+json"),
			response.Location("testlocation"),
			response.Code(http.StatusCreated),
		),
	)
	defer s.Close()

	broker, err := New(context.Background(), withDefaultTestConfig(s.URL(), ""))
	is.NoErr(err)

	_, err = broker.CreateEntity(context.Background(), "testtenant", testEntity("Device", "urn:ngsi-ld:Device:testid"), nil)
	is.NoErr(err) // should not return an error
}

func TestThatNotificationsAreSent_ThisTestShouldBeBrokenUp(t *testing.T) {
	is := is.New(t)

	s := testutils.NewMockServiceThat(
		Expects(is, anyInput()),
		Returns(
			response.ContentType("application/ld+json"),
			response.Location("testlocation"),
			response.Code(http.StatusCreated),
		),
	)
	defer s.Close()

	ns := testutils.NewMockServiceThat(Expects(is, anyInput()), Returns(response.Code(http.StatusOK)))
	defer ns.Close()

	broker, err := New(context.Background(), withDefaultTestConfig(s.URL(), ns.URL()))
	is.NoErr(err)

	broker.Start()

	_, err = broker.CreateEntity(context.Background(), "testtenant", testEntity("Device", "urn:ngsi-ld:Device:testid"), nil)
	is.NoErr(err) // should not return an error

	broker.Stop()

	is.Equal(ns.RequestCount(), 1)
}

func withDefaultTestConfig(brokerEndpoint, notificationEndpoint string) cfg.Config {
	cfg := cfg.Config{
		Tenants: []cfg.Tenant{
			{
				ID: "testtenant",
				ContextSources: []cfg.ContextSourceConfig{
					{
						Endpoint: brokerEndpoint,
						Information: []cfg.RegistrationInfo{
							{
								Entities: []cfg.EntityInfo{
									{
										IDPattern: "^urn:ngsi-ld:Device:.+",
										Type:      "Device",
									},
								},
							},
							{
								Entities: []cfg.EntityInfo{
									{
										IDPattern: "^urn:ngsi-ld:DeviceModel:.+",
										Type:      "DeviceModel",
									},
								},
							},
						},
					},
				},
				Notifications: []cfg.Notification{
					{
						Endpoint: notificationEndpoint,
						Entities: []cfg.EntityInfo{
							{
								IDPattern: "^urn:ngsi-ld:Device:.+",
								Type:      "Device",
							},
						},
					},
				},
			},
		},
	}

	return cfg
}

func withEmptyConfig() cfg.Config {
	return cfg.Config{}
}

func testEntity(entityType, entityID string) types.Entity {
	e, _ := entities.New(entityID, entityType)
	return e
}
