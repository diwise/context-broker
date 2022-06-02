package subscriptions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/subscriptions"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Notifier interface {
	Start() error
	Stop() error

	EntityCreated(ctx context.Context, e types.Entity)
	EntityUpdated(ctx context.Context, e types.Entity)
}

type action func()

type notifier struct {
	started  bool
	endpoint string

	queue chan action
}

func NewNotifier(ctx context.Context, endpoint string) (Notifier, error) {
	return &notifier{
		endpoint: endpoint,
		queue:    make(chan action, 32),
	}, nil
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

func (n *notifier) EntityCreated(ctx context.Context, e types.Entity) {
	if n.started {
		n.queue <- func() {
			err := postNotification(ctx, e, n.endpoint)
			if err != nil {
				logger := logging.GetFromContext(ctx)
				logger.Error().Err(err).Msg("failed to post notification")
			}
		}
	}
}

func (n *notifier) EntityUpdated(ctx context.Context, e types.Entity) {
	if n.started {
		n.queue <- func() {
			err := postNotification(ctx, e, n.endpoint)
			if err != nil {
				logger := logging.GetFromContext(ctx)
				logger.Error().Err(err).Msg("failed to post notification")
			}
		}
	}
}

func postNotification(ctx context.Context, e types.Entity, endpoint string) error {
	notification := subscriptions.NewNotification(e)
	body, err := json.MarshalIndent(notification, "", " ")
	if err != nil {
		return err
	}

	httpClient := http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

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
