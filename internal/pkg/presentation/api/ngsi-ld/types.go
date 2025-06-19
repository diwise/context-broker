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
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// NewRetrieveAvailableEntityTypesHandler handles GET requests for the
// entity types available in this NGSI-LD system
func NewRetrieveAvailableEntityTypesHandler(
	contextInformationManager cim.TypesRetriever,
	authenticator auth.Enticator) http.HandlerFunc {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx := r.Context()
		tenant := GetTenantFromContext(ctx)

		labeler, _ := otelhttp.LabelerFromContext(ctx)
		defer func() { addLabelIfError(err, labeler) }()

		propagatedHeaders := extractHeaders(r, "Accept", "Link")

		detailsRequested := r.URL.Query().Get("details")
		if detailsRequested != "" && detailsRequested != "false" {
			err = errors.New("details not supported for /types")
			ngsierrors.ReportNewBadRequestData(w, err.Error(), traceID(ctx))
			return
		}

		log := logging.GetFromContext(ctx)

		err = authenticator.CheckAccess(ctx, r, tenant, []string{})
		if err != nil {
			log.Warn("access not granted", "err", err.Error())
			messageToSendToNonAuthenticatedClients := "not found"
			ngsierrors.ReportNotFoundError(w, messageToSendToNonAuthenticatedClients, traceID(ctx))
			return
		}

		contentType := r.Header.Get("Accept")
		if contentType == "" {
			contentType = "application/ld+json"
		}

		availableTypes, err := contextInformationManager.RetrieveTypes(ctx, tenant, propagatedHeaders)
		if err != nil {
			log.Error("query entities failed", "err", err.Error())
			mapCIMToNGSILDError(w, err, traceID(ctx))
			return
		}

		response, _ := entities.New(
			"entity-type-list-"+tenant,
			"EntityTypeList",
			decorators.TextList("typeList", availableTypes),
		)

		responseBody, err := json.Marshal(response)

		if err != nil {
			log.Error("retrieve entity types: failed to marshal type list to json", "err", err.Error())
			mapCIMToNGSILDError(w, err, traceID(ctx))
			return
		}

		w.Header().Add("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		w.Write(responseBody)
	})
}
