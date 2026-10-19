package subscriptions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

type notifierState uint8

const (
	notifierCreated notifierState = iota
	notifierRunning
	notifierStopping
	notifierStopped
)

type notifier struct {
	ctx           context.Context
	mu            sync.Mutex
	state         notifierState
	queue         chan action
	done          chan struct{}
	notifications map[string][]config.Notification
}

func NewNotifier(ctx context.Context, cfg config.Config) (Notifier, error) {
	n := &notifier{
		ctx:           ctx,
		queue:         make(chan action, 32),
		done:          make(chan struct{}),
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

	log := logging.GetFromContext(ctx)
	log.Debug("notifications configured...", "count", len(n.notifications))

	return n, nil
}

func (n *notifier) Start() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.state == notifierRunning || n.state == notifierStopping {
		return fmt.Errorf("already started")
	}
	if n.state == notifierStopped {
		return fmt.Errorf("already stopped")
	}

	n.state = notifierRunning

	go n.run(n.ctx)

	return nil
}

func (n *notifier) Stop() error {
	n.mu.Lock()

	switch n.state {
	case notifierCreated:
		n.mu.Unlock()
		return nil
	case notifierStopping:
		done := n.done
		n.mu.Unlock()
		<-done
		return nil
	case notifierStopped:
		n.mu.Unlock()
		return nil
	}

	n.state = notifierStopping
	select {
	case n.queue <- nil:
	case <-n.ctx.Done():
	}
	done := n.done
	n.mu.Unlock()

	<-done

	return nil
}

func (n *notifier) EntityCreated(ctx context.Context, e types.Entity, tenant string) {
	n.enqueueNotifications(ctx, e, tenant)
}

func (n *notifier) EntityUpdated(ctx context.Context, e types.Entity, tenant string) {
	n.enqueueNotifications(ctx, e, tenant)
}

func (n *notifier) enqueueNotifications(ctx context.Context, e types.Entity, tenant string) {
	logger := logging.GetFromContext(ctx)
	ctx, span := tracer.Start(context.WithoutCancel(ctx), "post")

	queued := n.enqueue(func() {
		errChan := make(chan error, len(n.notifications[tenant]))
		var wg sync.WaitGroup

		for _, notification := range n.notifications[tenant] {
			wg.Add(1)
			go func(endpoint string) {
				defer wg.Done()

				requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				stopCancellation := context.AfterFunc(n.ctx, cancel)
				defer stopCancellation()
				defer cancel()

				err := postNotification(requestCtx, e, endpoint)
				if err != nil {
					logger.Error("failed to post notification", "err", err.Error())
					errChan <- err
				}
			}(notification.Endpoint)
		}

		wg.Wait()
		close(errChan)

		var notificationErrors []error
		for err := range errChan {
			notificationErrors = append(notificationErrors, err)
		}
		tracing.RecordAnyErrorAndEndSpan(errors.Join(notificationErrors...), span)
	})
	if !queued {
		span.End()
	}

}

func (n *notifier) enqueue(action action) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.state != notifierRunning {
		return false
	}

	select {
	case n.queue <- action:
		return true
	case <-n.ctx.Done():
		return false
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

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("notification endpoint returned %s", resp.Status)
	}

	return nil
}

func (n *notifier) run(ctx context.Context) {
	defer func() {
		n.mu.Lock()
		n.state = notifierStopped
		close(n.done)
		n.mu.Unlock()
	}()

	log := logging.GetFromContext(ctx)
	log.Debug("notifier started...")

	for {
		select {
		case <-ctx.Done():
			return
		case action, ok := <-n.queue:
			if !ok || action == nil {
				return
			}

			action()
		}
	}
}
