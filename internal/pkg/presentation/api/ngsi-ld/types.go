package ngsild

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/diwise/context-broker/internal/pkg/application/cim"
	"github.com/diwise/context-broker/internal/pkg/presentation/api/ngsi-ld/auth"
	ngsierrors "github.com/diwise/context-broker/pkg/ngsild/errors"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities/decorators"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/tracing"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// NewRetrieveAvailableEntityTypesHandler handles GET requests for the
// entity types available in this NGSI-LD system
func NewRetrieveAvailableEntityTypesHandler(
	contextInformationManager cim.TypesRetriever,
	authenticator auth.Enticator,
	logger zerolog.Logger) http.HandlerFunc {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx := r.Context()
		tenant := GetTenantFromContext(ctx)

		propagatedHeaders := extractHeaders(r, "Accept", "Link")

		ctx, span := tracer.Start(ctx, "retrieve-types",
			trace.WithAttributes(attribute.String(TraceAttributeNGSILDTenant, tenant)),
		)
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

		traceID, ctx, log := o11y.AddTraceIDToLoggerAndStoreInContext(span, logger, ctx)

		detailsRequested := r.URL.Query().Get("details")
		if detailsRequested != "" && detailsRequested != "false" {
			err = errors.New("details not supported for /types")
			ngsierrors.ReportNewBadRequestData(w, err.Error(), traceID)
			return
		}

		err = authenticator.CheckAccess(ctx, r, tenant, []string{})
		if err != nil {
			log.Warn().Err(err).Msg("access not granted")
			messageToSendToNonAuthenticatedClients := "not found"
			ngsierrors.ReportNotFoundError(w, messageToSendToNonAuthenticatedClients, traceID)
			return
		}

		contentType := r.Header.Get("Accept")
		if contentType == "" {
			contentType = "application/ld+json"
		}

		availableTypes, err := contextInformationManager.RetrieveTypes(ctx, tenant, propagatedHeaders)
		if err != nil {
			log.Error().Err(err).Msg("query entities failed")
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		response, _ := entities.New(
			"entity-type-list-"+tenant,
			"EntityTypeList",
			decorators.TextList("typeList", availableTypes),
		)

		responseBody, err := json.Marshal(response)

		if err != nil {
			log.Error().Err(err).Msg("retrieve entity types: failed to marshal type list to json")
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		w.Header().Add("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		w.Write(responseBody)
	})
}
