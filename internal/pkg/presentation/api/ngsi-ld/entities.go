package ngsild

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/diwise/ngsi-ld-context-broker/internal/pkg/application/cim"
	ngsierrors "github.com/diwise/ngsi-ld-context-broker/internal/pkg/presentation/api/ngsi-ld/errors"
	"github.com/diwise/ngsi-ld-context-broker/internal/pkg/presentation/api/ngsi-ld/types"
	"github.com/rs/zerolog"

	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("context-broker/ngsi-ld/entities")

type CreateEntityCompletionCallback func(ctx context.Context, entityType, entityID string, logger zerolog.Logger)

//NewCreateEntityHandler handles incoming POST requests for NGSI entities
func NewCreateEntityHandler(
	contextInformationManager cim.EntityCreator,
	logger zerolog.Logger,
	onsuccess CreateEntityCompletionCallback) http.HandlerFunc {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx := r.Context()
		tenant := GetTenantFromContext(ctx)

		ctx, span := tracer.Start(ctx, "create-entity")
		defer func() {
			if err != nil {
				span.RecordError(err)
			}
			span.End()
		}()

		entity := &types.BaseEntity{}
		// copy the body from the request and restore it for later use
		body, _ := ioutil.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewBuffer(body))

		err = json.NewDecoder(io.NopCloser(bytes.NewBuffer(body))).Decode(entity)

		if err != nil {
			ngsierrors.ReportNewInvalidRequest(
				w,
				fmt.Sprintf("unable to decode request payload: %s", err.Error()),
			)
			return
		}

		var result *cim.CreateEntityResult

		result, err = contextInformationManager.CreateEntity(ctx, tenant, entity.Type, entity.ID, r.Body)

		if err != nil {
			switch e := err.(type) {
			case cim.AlreadyExistsError:
				ngsierrors.ReportNewAlreadyExistsError(w, e.Error())
			case cim.BadRequestDataError:
				ngsierrors.ReportNewBadRequestData(w, e.Error())
			case cim.InvalidRequestError:
				ngsierrors.ReportNewInvalidRequest(w, e.Error())
			case cim.NotFoundError:
				ngsierrors.ReportNotFoundError(w, e.Error())
			case cim.UnknownTenantError:
				ngsierrors.ReportUnknownTenantError(w, e.Error())
			default:
				ngsierrors.ReportNewInternalError(w, e.Error())
			}

			return
		}

		onsuccess(ctx, entity.Type, entity.ID, logger)

		// FIXME: Make sure that the Location header can propagate up the federation tree properly
		w.Header().Add("Location", result.Location())
		w.WriteHeader(http.StatusCreated)
	})
}

//NewQueryEntitiesHandler handles GET requests for NGSI entities
func NewQueryEntitiesHandler(
	contextInformationManager cim.EntityQuerier,
	logger zerolog.Logger) http.HandlerFunc {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx := r.Context()
		tenant := GetTenantFromContext(ctx)

		ctx, span := tracer.Start(ctx, "query-entities")
		defer func() {
			if err != nil {
				span.RecordError(err)
			}
			span.End()
		}()

		attributeNames := r.URL.Query().Get("attrs")
		entityTypeNames := r.URL.Query().Get("type")
		georel := r.URL.Query().Get("georel")
		q := r.URL.Query().Get("q")
		//TODO: Parse and validate the query

		if entityTypeNames == "" && attributeNames == "" && q == "" && georel == "" {
			err = errors.New("at least one among type, attrs, q, or georel must be present in a request for entities")
			ngsierrors.ReportNewBadRequestData(w, err.Error())
			return
		}

		entityTypes := strings.Split(entityTypeNames, ",")
		attributes := strings.Split(attributeNames, ",")

		result, err := contextInformationManager.QueryEntities(ctx, tenant, entityTypes, attributes, r.URL.Path+"?"+r.URL.RawQuery)
		if err != nil {
			switch e := err.(type) {
			case cim.BadRequestDataError:
				ngsierrors.ReportNewBadRequestData(w, e.Error())
			case cim.UnknownTenantError:
				ngsierrors.ReportUnknownTenantError(w, e.Error())
			default:
				ngsierrors.ReportNewInternalError(w, e.Error())
			}

			return
		}

		w.Header().Add("Content-Type", "application/ld+json")
		w.WriteHeader(http.StatusOK)
		// TODO: Add a RFC 8288 Link header with information about previous and/or next page if they exist

		w.Write([]byte("["))

		numFound := 0
		for e := range result.Found {
			if e == nil {
				break
			}

			numFound++
			if numFound > 1 {
				w.Write([]byte(","))
			}

			bytes, err := json.Marshal(e)
			if err != nil {
				logger.Error().Err(err).Msg("failed to marshal entity to json")
			}
			w.Write(bytes)
		}

		w.Write([]byte("]"))
	})
}
