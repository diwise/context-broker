package subscriptions

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diwise/context-broker/internal/pkg/application/config"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	. "github.com/diwise/context-broker/pkg/ngsild/types/entities/decorators"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"
	testutils "github.com/diwise/service-chassis/pkg/test/http"
	"github.com/diwise/service-chassis/pkg/test/http/expects"
	"github.com/diwise/service-chassis/pkg/test/http/response"
	"github.com/matryer/is"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

var Expects = testutils.Expects
var Returns = testutils.Returns

var method = expects.RequestMethod
var bodyContaining = expects.RequestBodyContaining

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

// Keep one HTTP request active so the worker cannot consume the queued jobs.
func newBlockedNotifier(t *testing.T) (*notifier, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseRequest) }) }

	originalHTTPClient := httpClient
	httpClient = http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		close(requestStarted)
		select {
		case <-releaseRequest:
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	})}
	notifierInstance, err := NewNotifier(ctx, config.Config{Tenants: []config.Tenant{{
		ID:            "default",
		Notifications: []config.Notification{{Endpoint: "http://endpoint"}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	n := notifierInstance.(*notifier)
	t.Cleanup(func() {
		release()
		cancel()
		select {
		case <-n.done:
		case <-time.After(time.Second):
			t.Error("notifier did not stop during cleanup")
		}
		httpClient = originalHTTPClient
	})
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	e, err := entities.New("urn:ngsi-ld:Lifebuoy:blocked", "Lifebuoy", Status("off"))
	if err != nil {
		t.Fatal(err)
	}
	n.EntityCreated(ctx, e, "default")
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("notification request did not start")
	}
	return n, release
}

func TestFullNotificationQueueDoesNotBlockCaller(t *testing.T) {
	n, _ := newBlockedNotifier(t)
	for range cap(n.queue) {
		if result := n.enqueue(func() {}); result != enqueueAccepted {
			t.Fatalf("could not fill queue: %v", result)
		}
	}
	e, err := entities.New("urn:ngsi-ld:Lifebuoy:overflow", "Lifebuoy", Status("off"))
	if err != nil {
		t.Fatal(err)
	}
	returned := make(chan struct{})
	go func() {
		n.EntityUpdated(context.Background(), e, "default")
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("full notification queue blocked the caller")
	}
}

func TestTenantWithoutEndpointsDoesNotUseQueueOrCreateSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	originalTracer := tracer
	tracer = provider.Tracer("test")
	t.Cleanup(func() { tracer = originalTracer })
	n := &notifier{
		ctx: context.Background(), state: notifierRunning,
		queue:         make(chan action, 1),
		notifications: map[string][]config.Notification{"default": {{Endpoint: "http://endpoint"}}},
	}
	n.EntityCreated(context.Background(), nil, "other")
	n.EntityUpdated(context.Background(), nil, "other")
	if len(n.queue) != 0 {
		t.Fatal("tenant without endpoints used queue space")
	}
	if len(recorder.Started()) != 0 {
		t.Fatal("tenant without endpoints created a span")
	}
}

func TestRejectedNotificationsEndSpansAndOnlyWarnWhenFull(t *testing.T) {
	for _, reason := range []string{"full", "stopped", "cancelled"} {
		t.Run(reason, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			originalTracer := tracer
			tracer = provider.Tracer("test")
			t.Cleanup(func() { tracer = originalTracer })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			n := &notifier{
				ctx: ctx, state: notifierRunning, queue: make(chan action, 1),
				notifications: map[string][]config.Notification{"default": {{Endpoint: "http://endpoint"}}},
			}
			switch reason {
			case "full":
				n.queue <- func() {}
			case "stopped":
				n.state = notifierStopped
			case "cancelled":
				cancel()
			}
			e, err := entities.New("urn:ngsi-ld:Lifebuoy:rejected", "Lifebuoy", Status("off"))
			if err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			logCtx := logging.NewContextWithLogger(ctx, slog.New(slog.NewJSONHandler(&logs, nil)))
			n.EntityCreated(logCtx, e, "default")
			if len(recorder.Started()) != 1 || len(recorder.Ended()) != 1 {
				t.Fatal("rejected notification did not end its span")
			}
			if reason == "full" {
				for _, want := range []string{`"level":"WARN"`, `"tenant":"default"`, `"entity":"urn:ngsi-ld:Lifebuoy:rejected"`} {
					if !strings.Contains(logs.String(), want) {
						t.Errorf("queue overflow log missing %s: %s", want, logs.String())
					}
				}
			} else if logs.Len() != 0 {
				t.Errorf("inactive notifier emitted warning: %s", logs.String())
			}
		})
	}
}

func TestStopWithFullQueueAllowsEnqueueToReturnAndDrainsInOrder(t *testing.T) {
	n, release := newBlockedNotifier(t)
	var processed []int
	for i := range cap(n.queue) {
		if result := n.enqueue(func() { processed = append(processed, i) }); result != enqueueAccepted {
			t.Fatalf("could not fill queue: %v", result)
		}
	}
	stopped := make(chan error, 2)
	go func() { stopped <- n.Stop() }()

	// Wait for Stop to enter its queue-space wait while the worker is blocked.
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		state := make(chan notifierState, 1)
		go func() {
			n.mu.Lock()
			defer n.mu.Unlock()
			state <- n.state
		}()
		select {
		case current := <-state:
			if current == notifierStopping {
				goto stopping
			}
		case <-deadline.C:
			t.Fatal("Stop did not release mutex while waiting for queue space")
		}
		time.Sleep(time.Millisecond)
	}

stopping:
	go func() { stopped <- n.Stop() }()
	result := make(chan enqueueResult, 1)
	go func() { result <- n.enqueue(func() { t.Error("accepted job after stopping") }) }()
	select {
	case got := <-result:
		if got != enqueueInactive {
			t.Fatalf("enqueue while stopping returned %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("enqueue blocked behind Stop")
	}
	release()
	for range 2 {
		select {
		case err := <-stopped:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("Stop did not finish after releasing the worker")
		}
	}
	if len(processed) != cap(n.queue) {
		t.Fatalf("processed %d jobs, want %d", len(processed), cap(n.queue))
	}
	for i, got := range processed {
		if got != i {
			t.Fatalf("job %d processed out of order: %d", i, got)
		}
	}
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
