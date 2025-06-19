package ngsild

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/diwise/context-broker/internal/pkg/application/cim"
	"github.com/diwise/context-broker/internal/pkg/presentation/api/ngsi-ld/auth"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"
)

func RegisterHandlers(ctx context.Context, mux *http.ServeMux, middleware []func(http.Handler) http.Handler, policies io.Reader, app cim.ContextInformationManager) error {

	authenticator, err := auth.NewAuthenticator(ctx, policies)
	if err != nil {
		return fmt.Errorf("failed to create api authenticator: %w", err)
	}

	middleware = append(middleware,
		NGSIMiddleware(),
		RequiredContentTypes([]string{"application/json", "application/ld+json"}),
	)

	log := logging.GetFromContext(ctx)

	r := http.NewServeMux()

	register := func(method, endpoint string, handler http.HandlerFunc) {
		r.HandleFunc(
			fmt.Sprintf("%s /ngsi-ld/v1%s", method, endpoint),
			handler,
		)
	}

	register(http.MethodGet, "/entities", NewQueryEntitiesHandler(app, authenticator, log))
	register(http.MethodGet, "/entities/{entityId}", NewRetrieveEntityHandler(app, authenticator, log))
	register(http.MethodPatch, "/entities/{entityId}", NewMergeEntityHandler(app, authenticator, log))
	register(http.MethodPatch, "/entities/{entityId}/attrs/", NewUpdateEntityAttributesHandler(app, authenticator, log))
	register(
		http.MethodPost, "/entities",
		NewCreateEntityHandler(
			app, authenticator, log,
			func(ctx context.Context, entityType, entityID string, logger *slog.Logger) {},
		),
	)

	register(http.MethodDelete, "/entities/{entityId}", NewDeleteEntityHandler(app, authenticator, log))

	register(http.MethodGet, "/temporal/entities",
		NewQueryTemporalEvolutionOfEntitiesHandler(app, authenticator, log),
	)

	register(http.MethodGet, "/temporal/entities/{entityId}",
		NewRetrieveTemporalEvolutionOfAnEntityHandler(app, authenticator, log),
	)

	register(http.MethodGet, "/types", NewRetrieveAvailableEntityTypesHandler(app, authenticator, log))
	register(http.MethodGet, "/jsonldContexts/{contextId}", NewServeContextHandler(log))

	var handler http.Handler = r

	// wrap the mux with any passed in middleware handlers
	for _, mw := range slices.Backward(middleware) {
		handler = mw(handler)
	}

	mux.Handle("GET /", handler)
	mux.Handle("PATCH /", handler)
	mux.Handle("POST /", handler)
	mux.Handle("DELETE /", handler)

	return nil
}

type tenantContextKey struct {
	name string
}

var tenantCtxKey = &tenantContextKey{"ngsi-tenant"}

func RequiredContentTypes(validTypes []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			contentType := r.Header.Get("Content-Type")
			isValidContentType := true

			if len(contentType) > 0 {
				isValidContentType = false

				for _, t := range validTypes {
					if strings.HasPrefix(contentType, t) {
						isValidContentType = true
						break
					}
				}
			}

			if isValidContentType {
				next.ServeHTTP(w, r)
			} else {
				http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
			}
		})
	}
}

// NGSIMiddleware packs any tenant id into the context
func NGSIMiddleware() func(http.Handler) http.Handler {
	tenantHeaderName := http.CanonicalHeaderKey("NGSILD-Tenant")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenant := "default"

			tenantHeader := r.Header[tenantHeaderName]
			if len(tenantHeader) > 0 {
				tenant = tenantHeader[0]
			}

			ctx := context.WithValue(r.Context(), tenantCtxKey, tenant)
			r = r.WithContext(ctx)

			if tenant != "default" {
				w.Header().Add(tenantHeaderName, tenant)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetTenantFromContext extracts the tenant name, if any, from the provided context
func GetTenantFromContext(ctx context.Context) string {
	tenant, ok := ctx.Value(tenantCtxKey).(string)

	if !ok {
		return ""
	}

	return tenant
}
