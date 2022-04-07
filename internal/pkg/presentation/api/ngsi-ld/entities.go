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

	"github.com/diwise/context-broker/internal/pkg/application/cim"
	"github.com/diwise/context-broker/internal/pkg/infrastructure/logging"
	"github.com/diwise/context-broker/internal/pkg/infrastructure/tracing"
	ngsierrors "github.com/diwise/context-broker/internal/pkg/presentation/api/ngsi-ld/errors"
	"github.com/diwise/context-broker/internal/pkg/presentation/api/ngsi-ld/geojson"
	"github.com/diwise/context-broker/internal/pkg/presentation/api/ngsi-ld/types"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("context-broker/ngsi-ld/entities")

const (
	TraceAttributeEntityID     string = "entity-id"
	TraceAttributeNGSILDTenant string = "ngsild-tenant"
)

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

		ctx, span := tracer.Start(ctx, "create-entity",
			trace.WithAttributes(attribute.String(TraceAttributeNGSILDTenant, tenant)),
		)
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

		traceID, ctx, log := addTraceIDToLoggerAndStoreInContext(span, logger, ctx)

		entity := &types.BaseEntity{}
		// copy the body from the request and restore it for later use
		body, _ := ioutil.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewBuffer(body))

		err = json.NewDecoder(io.NopCloser(bytes.NewBuffer(body))).Decode(entity)

		if err != nil {
			ngsierrors.ReportNewInvalidRequest(
				w,
				fmt.Sprintf("unable to decode request payload: %s", err.Error()),
				traceID,
			)
			return
		}

		var result *cim.CreateEntityResult

		result, err = contextInformationManager.CreateEntity(ctx, tenant, entity.Type, entity.ID, r.Body)
		if err != nil {
			log.Error().Err(err).Msg("create entity failed")
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		log.Info().Str("entityID", entity.ID).Str("tenant", tenant).Msg("entity created")

		onsuccess(ctx, entity.Type, entity.ID, log)

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

		propagatedHeaders := map[string][]string{}
		for _, header := range []string{"Accept", "Link"} {
			headerValue, ok := r.Header[header]
			if ok {
				propagatedHeaders[header] = headerValue
			}
		}

		ctx, span := tracer.Start(ctx, "query-entities",
			trace.WithAttributes(attribute.String(TraceAttributeNGSILDTenant, tenant)),
		)
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

		traceID, ctx, log := addTraceIDToLoggerAndStoreInContext(span, logger, ctx)

		attributeNames := r.URL.Query().Get("attrs")
		entityTypeNames := r.URL.Query().Get("type")
		georel := r.URL.Query().Get("georel")
		q := r.URL.Query().Get("q")
		//TODO: Parse and validate the query

		if entityTypeNames == "" && attributeNames == "" && q == "" && georel == "" {
			err = errors.New("at least one among type, attrs, q, or georel must be present in a request for entities")
			ngsierrors.ReportNewBadRequestData(w, err.Error(), traceID)
			return
		}

		entityTypes := strings.Split(entityTypeNames, ",")
		attributes := strings.Split(attributeNames, ",")

		result, err := contextInformationManager.QueryEntities(ctx, tenant, entityTypes, attributes, r.URL.Path+"?"+r.URL.RawQuery, propagatedHeaders)
		if err != nil {
			log.Error().Err(err).Msg("query entities failed")
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		contentType := r.Header.Get("Accept")
		if contentType == "" {
			contentType = "application/ld+json"
		}

		var entityConverter func(cim.Entity) cim.Entity

		var geoJsonCollection *geojson.GeoJSONFeatureCollection
		var entityCollection []cim.Entity

		if contentType == "application/geo+json" {
			geoJsonCollection = geojson.NewFeatureCollection()
			entityConverter = func(e cim.Entity) cim.Entity {
				gje, err := geojson.ConvertEntity(e)
				if err == nil {
					geoJsonCollection.Features = append(geoJsonCollection.Features, *gje)
				}
				return e
			}
		} else {
			entityCollection = []cim.Entity{}
			entityConverter = func(e cim.Entity) cim.Entity {
				entityCollection = append(entityCollection, e)
				return e
			}
		}

		for e := range result.Found {
			if e == nil {
				break
			}

			entityConverter(e)
		}

		var responseBody []byte

		if geoJsonCollection != nil {
			responseBody, err = json.Marshal(geoJsonCollection)
		} else {
			responseBody, err = json.Marshal(entityCollection)
		}

		if err != nil {
			log.Error().Err(err).Msg("query entities: failed to marshal entity collection to json")
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		w.Header().Add("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		// TODO: Add a RFC 8288 Link header with information about previous and/or next page if they exist
		w.Write(responseBody)
	})
}

//NewRetrieveEntityHandler retrieves entity by ID.
func NewRetrieveEntityHandler(
	contextInformationManager cim.EntityRetriever,
	logger zerolog.Logger) http.HandlerFunc {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx := r.Context()
		tenant := GetTenantFromContext(ctx)
		entityID := chi.URLParam(r, "entityId")

		ctx, span := tracer.Start(ctx, "retrieve-entity",
			trace.WithAttributes(
				attribute.String(TraceAttributeNGSILDTenant, tenant),
				attribute.String(TraceAttributeEntityID, entityID),
			),
		)
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

		traceID, ctx, log := addTraceIDToLoggerAndStoreInContext(span, logger, ctx)

		var entity cim.Entity
		entity, err = contextInformationManager.RetrieveEntity(ctx, tenant, entityID)

		if err != nil {
			log.Error().Err(err).Msg("retrieve entity failed")
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		responseContentType := r.Header.Get("Accept")
		if responseContentType == "" {
			responseContentType = "application/ld+json"
		}

		var responseBody []byte

		if responseContentType == "application/geo+json" {
			var gjf *geojson.GeoJSONFeature
			gjf, err = geojson.ConvertEntity(entity)
			if err == nil {
				responseBody, err = json.Marshal(gjf)
			}
		} else {
			responseBody, err = json.Marshal(entity)
		}

		if err != nil {
			log.Error().Err(err).Msg("failed to convert or marshal response entity")
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		w.Header().Add("Content-Type", responseContentType)
		w.Write(responseBody)
	})
}

//NewUpdateEntityAttributesHandler handles PATCH requests for NGSI entitity attributes
func NewUpdateEntityAttributesHandler(
	contextInformationManager cim.EntityAttributesUpdater,
	logger zerolog.Logger) http.HandlerFunc {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx := r.Context()
		tenant := GetTenantFromContext(ctx)
		entityID := chi.URLParam(r, "entityId")

		ctx, span := tracer.Start(ctx, "update-entity-attributes",
			trace.WithAttributes(
				attribute.String(TraceAttributeNGSILDTenant, tenant),
				attribute.String(TraceAttributeEntityID, entityID),
			),
		)
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

		traceID, ctx, log := addTraceIDToLoggerAndStoreInContext(span, logger, ctx)

		entity := &types.BaseEntity{}
		// copy the body from the request and restore it for later use
		body, _ := ioutil.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewBuffer(body))

		err = json.NewDecoder(io.NopCloser(bytes.NewBuffer(body))).Decode(entity)
		if err != nil {
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		err = contextInformationManager.UpdateEntityAttributes(ctx, tenant, entityID, r.Body)

		if err != nil {
			log.Error().Err(err).Str("entityID", entityID).Str("tenant", tenant).Msg("failed to update entity attributes")
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		log.Info().Str("entityID", entityID).Str("tenant", tenant).Msg("entity attributes updated")

		w.WriteHeader(http.StatusNoContent)
	})
}

func addTraceIDToLoggerAndStoreInContext(span trace.Span, logger zerolog.Logger, ctx context.Context) (string, context.Context, zerolog.Logger) {

	log := logger
	traceID := span.SpanContext().TraceID()
	traceIDStr := ""

	if traceID.IsValid() {
		traceIDStr = traceID.String()
		log = log.With().Str("traceID", traceIDStr).Logger()
	}

	ctx = logging.NewContextWithLogger(ctx, log)
	return traceIDStr, ctx, log
}

func mapCIMToNGSILDError(w http.ResponseWriter, err error, traceID string) {
	switch e := err.(type) {
	case cim.AlreadyExistsError:
		ngsierrors.ReportNewAlreadyExistsError(w, e.Error(), traceID)
	case cim.BadRequestDataError:
		ngsierrors.ReportNewBadRequestData(w, e.Error(), traceID)
	case cim.InvalidRequestError:
		ngsierrors.ReportNewInvalidRequest(w, e.Error(), traceID)
	case cim.NotFoundError:
		ngsierrors.ReportNotFoundError(w, e.Error(), traceID)
	case cim.UnknownTenantError:
		ngsierrors.ReportUnknownTenantError(w, e.Error(), traceID)
	default:
		ngsierrors.ReportNewInternalError(w, e.Error(), traceID)
	}
}
