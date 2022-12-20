package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/tracing"
	"github.com/open-policy-agent/opa/rego"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("context-broker/ngsi-ld/authz")

type Enticator interface {
	CheckAccess(ctx context.Context, r *http.Request, tenant string, entityTypes []string) error
}

type enticatorImpl struct {
	preparedQuery rego.PreparedEvalQuery
}

func NewAuthenticator(ctx context.Context, logger zerolog.Logger, policies io.Reader) (Enticator, error) {

	module, err := io.ReadAll(policies)
	if err != nil {
		return nil, fmt.Errorf("unable to read authz policies: %s", err.Error())
	}

	impl := &enticatorImpl{}

	impl.preparedQuery, err = rego.New(
		rego.Query("x = data.example.authz.allow"),
		rego.Module("example.rego", string(module)),
	).PrepareForEval(ctx)

	if err != nil {
		return nil, err
	}

	return impl, nil
}

func (e *enticatorImpl) CheckAccess(ctx context.Context, r *http.Request, tenant string, entityTypes []string) error {
	var err error

	_, span := tracer.Start(ctx, "check-auth")
	defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

	token := r.Header.Get("Authorization")

	if len(token) > 7 {
		token = token[7:]
	}

	path := strings.Split(r.URL.Path, "/")

	input := map[string]any{
		"method": r.Method,
		"path":   path[1:],
		"token":  token,
		"tenant": tenant,
		"types":  entityTypes,
	}

	results, err := e.preparedQuery.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		err = fmt.Errorf("opa eval failed: %w", err)
		return err
	}

	if len(results) == 0 {
		err = fmt.Errorf("auth failed: opa query could not be satisfied")
		return err
	} else {

		binding := results[0].Bindings["x"]

		// If authz fails we will get back a single bool. Check for that first.
		allowed, ok := binding.(bool)
		if ok && !allowed {
			err = errors.New("authorization failed")
			return err
		}

		// If authz succeeds we should expect a result object here
		_, ok = binding.(map[string]any)

		if !ok {
			err = errors.New("opa error: unexpected result type")
			return err
		}
	}

	return nil
}
