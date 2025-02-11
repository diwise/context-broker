package subscriptions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/diwise/context-broker/internal/pkg/application/config"
	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/subscriptions"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/tracing"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
)

type Notifier interface {
	Start() error
	Stop() error

	EntityCreated(ctx context.Context, e types.Entity, tenant string)
	EntityUpdated(ctx context.Context, e types.Entity, tenant string)
}

var tracer = otel.Tracer("context-broker/notifier")

type action func()

type notifier struct {
	started       bool
	queue         chan action
	notifications map[string][]config.Notification
}

func NewNotifier(ctx context.Context, cfg config.Config) (Notifier, error) {
	n := &notifier{
		queue:         make(chan action, 32),
		notifications: make(map[string][]config.Notification),
	}

	for _, tenant := range cfg.Tenants {
		if len(tenant.Notifications) > 0 {
			n.notifications[tenant.ID] = tenant.Notifications
		}
	}

	if len(n.notifications) == 0 {
		return nil, nil
	}

	return n, nil
}

func (n *notifier) Start() error {
	if n.started {
		return fmt.Errorf("already started")
	}

	n.started = true

	go n.run()

	return nil
}

func (n *notifier) Stop() error {
	if n.started {
		// Create a result channel so that we can wait for completion
		resultChan := make(chan bool)

		n.queue <- func() {
			// close the queue to signal the consumers that we are going out of business
			close(n.queue)
			resultChan <- true
		}

		// blocking read until our action has been processed
		<-resultChan
	}
	return nil
}

func (n *notifier) EntityCreated(ctx context.Context, e types.Entity, tenant string) {
	if n.started {
		var err error

		logger := logging.GetFromContext(ctx)

		ctx, span := tracer.Start(context.WithoutCancel(ctx), "post")

		n.queue <- func() {
			defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

			var wg sync.WaitGroup
			defer wg.Wait()

			for _, notification := range n.notifications[tenant] {
				wg.Add(1)
				go func(endpoint string) {
					defer wg.Done()

					ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
					defer cancel()

					err = postNotification(ctx, e, endpoint)
					if err != nil {
						logger.Error("failed to post notification", "err", err.Error())
					}
				}(notification.Endpoint)
			}
		}
	}
}

func (n *notifier) EntityUpdated(ctx context.Context, e types.Entity, tenant string) {
	if n.started {
		var err error

		logger := logging.GetFromContext(ctx)

		ctx, span := tracer.Start(context.WithoutCancel(ctx), "post")

		n.queue <- func() {
			defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

			var wg sync.WaitGroup
			defer wg.Wait()

			for _, notification := range n.notifications[tenant] {
				wg.Add(1)
				go func(endpoint string) {
					defer wg.Done()

					ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
					defer cancel()

					err = postNotification(ctx, e, endpoint)
					if err != nil {
						logger.Error("failed to post notification", "err", err.Error())
					}
				}(notification.Endpoint)
			}
		}
	}
}

var httpClient http.Client = http.Client{
	Transport: otelhttp.NewTransport(http.DefaultTransport),
}

func postNotification(ctx context.Context, e types.Entity, endpoint string) error {
	notification := subscriptions.NewNotification(e)
	body, err := json.MarshalIndent(notification, "", " ")
	if err != nil {
		return fmt.Errorf("marshalling error (%w)", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("unable to create new request (%w)", err)
	}

	req.Header.Add("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request (%w)", err)
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	return nil
}

func (n *notifier) run() {
	// repeat until the queue is closed
	for action := range n.queue {
		if action == nil {
			return
		}

		action()
	}
}
