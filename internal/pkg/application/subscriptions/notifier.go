package subscriptions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

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
	started bool
	cfg     config.Config
	queue   chan action
}

func NewNotifier(ctx context.Context, cfg config.Config) (Notifier, error) {
	return &notifier{
		cfg:   cfg,
		queue: make(chan action, 32),
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

func (n *notifier) EntityCreated(ctx context.Context, e types.Entity, tenant string) {
	if n.started {
		var err error

		logger := logging.GetFromContext(ctx)

		ctx, span := tracer.Start(
			tracing.ExtractHeaders(context.Background(), tracing.InjectHeaders(ctx)),
			"post",
		)

		n.queue <- func() {
			defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

			for _, t := range n.cfg.Tenants {
				if strings.EqualFold(t.ID, tenant) {
					for _, not := range t.Notifications {
						for _, ent := range not.Entities {
							if ent.Type != e.Type() {
								continue
							}

							regexpForID, err := regexp.CompilePOSIX(ent.IDPattern)
							if err != nil {
								continue
							}

							if !regexpForID.MatchString(e.ID()) {
								continue
							}

							go func(endpoint string) {
								err = postNotification(ctx, e, endpoint)
								if err != nil {
									logger.Error().Err(err).Msg("failed to post notification")
								}
							}(not.Endpoint)
						}
					}
				}
			}
		}
	}
}

func (n *notifier) EntityUpdated(ctx context.Context, e types.Entity, tenant string) {
	if n.started {
		var err error

		logger := logging.GetFromContext(ctx)

		ctx, span := tracer.Start(
			tracing.ExtractHeaders(context.Background(), tracing.InjectHeaders(ctx)),
			"post",
		)

		n.queue <- func() {
			defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

			for _, t := range n.cfg.Tenants {
				if strings.EqualFold(t.ID, tenant) {
					for _, not := range t.Notifications {
						for _, ent := range not.Entities {
							if ent.Type != e.Type() {
								continue
							}

							regexpForID, err := regexp.CompilePOSIX(ent.IDPattern)
							if err != nil {
								continue
							}

							if !regexpForID.MatchString(e.ID()) {
								continue
							}

							go func(endpoint string) {
								err = postNotification(ctx, e, endpoint)
								if err != nil {
									logger.Error().Err(err).Msg("failed to post notification")
								}
							}(not.Endpoint)
						}
					}
				}
			}
		}
	}
}

func postNotification(ctx context.Context, e types.Entity, endpoint string) error {
	notification := subscriptions.NewNotification(e)
	body, err := json.MarshalIndent(notification, "", " ")
	if err != nil {
		return fmt.Errorf("marshalling error (%w)", err)
	}

	httpClient := http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
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
