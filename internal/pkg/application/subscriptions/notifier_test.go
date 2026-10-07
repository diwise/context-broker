package subscriptions

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestNotifierRunStopsWhenContextIsDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	notifierInstance, _ := NewNotifier(ctx, config.Config{Tenants: []config.Tenant{{
		ID: "default",
		Notifications: []config.Notification{{
			Endpoint: "http://endpoint",
		}},
	}}})
	n := notifierInstance.(*notifier)

	if err := n.Start(); err != nil {
		t.Fatal(err)
	}

	cancel()

	select {
	case <-n.done:
	case <-time.After(time.Second):
		t.Fatal("notifier did not stop when context was done")
	}

	if err := n.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestNotifierLifecycleCallsAreSafe(t *testing.T) {
	ctx := context.Background()
	notifierInstance, _ := NewNotifier(ctx, config.Config{Tenants: []config.Tenant{{
		ID: "default",
		Notifications: []config.Notification{{
			Endpoint: "http://endpoint",
		}},
	}}})

	var wg sync.WaitGroup
	startErrors := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			startErrors <- notifierInstance.Start()
		})
	}
	wg.Wait()
	close(startErrors)

	var successfulStarts int
	for err := range startErrors {
		if err == nil {
			successfulStarts++
		}
	}
	if successfulStarts != 1 {
		t.Fatalf("got %d successful starts, want 1", successfulStarts)
	}

	if err := notifierInstance.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := notifierInstance.Stop(); err != nil {
		t.Fatal(err)
	}

	e, err := entities.New("urn:ngsi-ld:Lifebuoy:stopped", "Lifebuoy", Status("off"))
	if err != nil {
		t.Fatal(err)
	}
	notifierInstance.EntityCreated(ctx, e, "default")
}

func TestPostNotificationReturnsErrorForHTTPErrorStatus(t *testing.T) {
	originalHTTPClient := httpClient
	httpClient = http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Status:     "500 Internal Server Error",
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})}
	t.Cleanup(func() { httpClient = originalHTTPClient })

	e, err := entities.New("urn:ngsi-ld:Lifebuoy:error", "Lifebuoy", Status("off"))
	if err != nil {
		t.Fatal(err)
	}

	err = postNotification(context.Background(), e, "http://endpoint")
	if err == nil {
		t.Fatal("expected an error for HTTP status 500")
	}
}

func TestContextCancellationCancelsActiveNotification(t *testing.T) {
	originalHTTPClient := httpClient
	requestStarted := make(chan struct{})
	httpClient = http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		close(requestStarted)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	t.Cleanup(func() { httpClient = originalHTTPClient })

	ctx, cancel := context.WithCancel(context.Background())
	notifierInstance, _ := NewNotifier(ctx, config.Config{Tenants: []config.Tenant{{
		ID: "default",
		Notifications: []config.Notification{{
			Endpoint: "http://endpoint",
		}},
	}}})
	n := notifierInstance.(*notifier)
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}

	e, err := entities.New("urn:ngsi-ld:Lifebuoy:cancel", "Lifebuoy", Status("off"))
	if err != nil {
		t.Fatal(err)
	}
	n.EntityCreated(ctx, e, "default")

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("notification request did not start")
	}
	cancel()

	select {
	case <-n.done:
	case <-time.After(time.Second):
		t.Fatal("notifier did not stop after cancelling an active request")
	}
}

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
