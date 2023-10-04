package ngsild

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/diwise/context-broker/internal/pkg/application/cim"
	"github.com/diwise/context-broker/internal/pkg/presentation/api/ngsi-ld/auth"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func RegisterHandlers(ctx context.Context, r chi.Router, policies io.Reader, app cim.ContextInformationManager) error {

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	authenticator, err := auth.NewAuthenticator(ctx, policies)
	if err != nil {
		return fmt.Errorf("failed to create api authenticator: %w", err)
	}

	r.Route("/ngsi-ld/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(middleware.AllowContentType("application/json", "application/ld+json"))
			r.Use(NGSIMiddleware())

			log := logging.GetFromContext(ctx)

			r.Get(
				"/entities",
				NewQueryEntitiesHandler(app, authenticator, log),
			)

			r.Get(
				"/entities/{entityId}",
				NewRetrieveEntityHandler(app, authenticator, log),
			)

			r.Patch(
				"/entities/{entityId}",
				NewMergeEntityHandler(app, authenticator, log),
			)

			r.Patch(
				"/entities/{entityId}/attrs/",
				NewUpdateEntityAttributesHandler(app, authenticator, log),
			)

			r.Post(
				"/entities",
				NewCreateEntityHandler(
					app, authenticator, log,
					func(ctx context.Context, entityType, entityID string, logger *slog.Logger) {},
				),
			)

			r.Delete(
				"/entities/{entityId}",
				NewDeleteEntityHandler(app, authenticator, log),
			)

			r.Get(
				"/temporal/entities",
				NewQueryTemporalEvolutionOfEntitiesHandler(app, authenticator, log),
			)

			r.Get(
				"/temporal/entities/{entityId}",
				NewRetrieveTemporalEvolutionOfAnEntityHandler(app, authenticator, log),
			)

			r.Get(
				"/types",
				NewRetrieveAvailableEntityTypesHandler(app, authenticator, log),
			)

			r.Get(
				"/jsonldContexts/{contextId}",
				NewServeContextHandler(log),
			)
		})
	})

	return nil
}

type tenantContextKey struct {
	name string
}

var tenantCtxKey = &tenantContextKey{"ngsi-tenant"}

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
