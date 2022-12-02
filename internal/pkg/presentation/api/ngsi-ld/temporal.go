package ngsild

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/diwise/context-broker/internal/pkg/application/cim"
	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/tracing"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func NewRetrieveTemporalEvolutionOfAnEntityHandler(
	contextInformationManager cim.EntityTemporalRetriever,
	logger zerolog.Logger) http.HandlerFunc {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx := r.Context()
		tenant := GetTenantFromContext(ctx)
		entityID, _ := url.QueryUnescape(chi.URLParam(r, "entityId"))

		propagatedHeaders := extractHeaders(r, "Accept", "Link")

		ctx, span := tracer.Start(ctx, "retrieve-temporal-entity",
			trace.WithAttributes(
				attribute.String(TraceAttributeNGSILDTenant, tenant),
				attribute.String(TraceAttributeEntityID, entityID),
			),
		)
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

		contentType := r.Header.Get("Accept")
		if contentType == "" {
			contentType = "application/ld+json"
		}

		traceID, ctx, log := o11y.AddTraceIDToLoggerAndStoreInContext(span, logger, ctx)

		var entityTemporal types.EntityTemporal
		entityTemporal, err = contextInformationManager.RetrieveTemporalEvolutionOfEntity(ctx, tenant, entityID, propagatedHeaders)

		if err != nil {
			log.Error().Err(err).Msg("failed to retrieve temporal evolution of entity")
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		responseBody, err := json.Marshal(entityTemporal)

		if err != nil {
			log.Error().Err(err).Msg("failed to convert or marshal response entity")
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		w.Header().Add("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		w.Write(responseBody)
	})
}
