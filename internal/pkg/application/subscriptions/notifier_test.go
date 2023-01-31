package subscriptions

import (
	"context"
	"net/http"
	"testing"

	"github.com/diwise/context-broker/internal/pkg/application/config"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	. "github.com/diwise/context-broker/pkg/ngsild/types/entities/decorators"
	testutils "github.com/diwise/service-chassis/pkg/test/http"
	"github.com/diwise/service-chassis/pkg/test/http/expects"
	"github.com/diwise/service-chassis/pkg/test/http/response"
	"github.com/matryer/is"
)

var Expects = testutils.Expects
var Returns = testutils.Returns

var method = expects.RequestMethod
var bodyContaining = expects.RequestBodyContaining

func TestSingleNotificationOnCreate(t *testing.T) {
	is := is.New(t)
	const entityID string = "urn:ngsi-ld:Lifebuoy:mybuoy"

	s := testutils.NewMockServiceThat(
		Expects(
			is,
			method(http.MethodPost),
			bodyContaining("urn:ngsi-ld:Lifebuoy:mybuoy"),
		),
		Returns(
			response.Code(http.StatusOK),
		),
	)
	defer s.Close()

	ctx := context.Background()
	cfg := config.Config{
		Tenants: []config.Tenant{
			{
				ID: "default",
				Notifications: []config.Notification{
					{
						Endpoint: s.URL(),
					},
				},
			},
		},
	}
	n, _ := NewNotifier(ctx, cfg)

	n.Start()

	e, err := entities.New(entityID, "Lifebuoy", Status("off"))
	is.NoErr(err)

	n.EntityCreated(ctx, e, "default")

	n.Stop()

	is.Equal(s.RequestCount(), 1)
}

func TestNotifierShouldBeNilIfNoNotifyEndpoints(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	cfg := config.Config{
		Tenants: []config.Tenant{
			{
				ID: "default-01",
			},
			{
				ID: "default-02",
			},
		},
	}
	n, _ := NewNotifier(ctx, cfg)
	is.True(n == nil)
}

func TestNotifierShouldBeNotNilWhenConfigured(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	cfg := config.Config{
		Tenants: []config.Tenant{
			{
				ID: "default-01",
			},
			{
				ID: "default-02",
				Notifications: []config.Notification{
					{
						Endpoint: "http://endpoint",
					},
				},
			},
		},
	}
	n, _ := NewNotifier(ctx, cfg)
	is.True(n != nil)
}

func TestNotificationShouldNotBeSentForOtherTenant(t *testing.T) {
	is := is.New(t)
	const entityID string = "urn:ngsi-ld:Lifebuoy:mybuoy"

	s := testutils.NewMockServiceThat(
		Expects(
			is,
			method(http.MethodPost),
			bodyContaining("urn:ngsi-ld:Lifebuoy:mybuoy"),
		),
		Returns(
			response.Code(http.StatusOK),
		),
	)
	defer s.Close()

	ctx := context.Background()
	cfg := config.Config{
		Tenants: []config.Tenant{
			{
				ID: "default",
				Notifications: []config.Notification{
					{
						Endpoint: s.URL(),
					},
				},
			},
		},
	}
	n, _ := NewNotifier(ctx, cfg)

	n.Start()

	e, err := entities.New(entityID, "Lifebuoy", Status("off"))
	is.NoErr(err)

	n.EntityCreated(ctx, e, "some other tenant")

	n.Stop()

	is.Equal(s.RequestCount(), 0)
}
