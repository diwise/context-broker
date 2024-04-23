package ngsild

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/diwise/context-broker/internal/pkg/application/cim"
	"github.com/diwise/context-broker/internal/pkg/presentation/api/ngsi-ld/auth"
	"github.com/diwise/context-broker/pkg/ngsild"
	ngsierrors "github.com/diwise/context-broker/pkg/ngsild/errors"
	"github.com/diwise/context-broker/pkg/ngsild/geojson"
	ngsitypes "github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/tracing"
	"github.com/go-chi/chi/v5"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("context-broker/ngsi-ld/entities")

const (
	TraceAttributeEntityID     string = "entity-id"
	TraceAttributeNGSILDTenant string = "ngsild-tenant"
)

type CreateEntityCompletionCallback func(ctx context.Context, entityType, entityID string, logger *slog.Logger)

// NewCreateEntityHandler handles incoming POST requests for NGSI entities
func NewCreateEntityHandler(
	contextInformationManager cim.EntityCreator,
	authenticator auth.Enticator,
	logger *slog.Logger,
	onsuccess CreateEntityCompletionCallback) http.HandlerFunc {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx := r.Context()
		tenant := GetTenantFromContext(ctx)

		propagatedHeaders := extractHeaders(r, "Content-Type", "Link")

		ctx, span := tracer.Start(ctx, "create-entity",
			trace.WithAttributes(attribute.String(TraceAttributeNGSILDTenant, tenant)),
		)
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

		traceID, ctx, log := o11y.AddTraceIDToLoggerAndStoreInContext(span, logger, ctx)

		// copy the body from the request and restore it for later use
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewBuffer(body))

		entity, err := entities.NewFromJSON(body)

		if err != nil {
			ngsierrors.ReportNewInvalidRequest(
				w,
				fmt.Sprintf("unable to decode request payload: %s", err.Error()),
				traceID,
			)
			return
		}

		entityID := entity.ID()
		entityType := entity.Type()

		// decorate the logger with info about the current tenant and entity id
		log = log.With(slog.String("entityID", entityID), slog.String("tenant", tenant))
		ctx = logging.NewContextWithLogger(ctx, log)

		err = authenticator.CheckAccess(ctx, r, tenant, []string{entityType})
		if err != nil {
			log.Warn("access not granted", "err", err.Error())
			ngsierrors.ReportUnauthorizedRequest(w, "not authorized", traceID)
			return
		}

		var result *ngsild.CreateEntityResult

		result, err = contextInformationManager.CreateEntity(ctx, tenant, entity, propagatedHeaders)
		if err != nil {
			log.Error("create entity failed", "err", err.Error())
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		log.Info("entity created")

		onsuccess(ctx, entityType, entityID, log)

		w.Header().Add("Location", result.Location())
		w.WriteHeader(http.StatusCreated)
	})
}

// NewQueryEntitiesHandler handles GET requests for NGSI entities
func NewQueryEntitiesHandler(
	contextInformationManager cim.EntityQuerier,
	authenticator auth.Enticator,
	logger *slog.Logger) http.HandlerFunc {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx := r.Context()
		tenant := GetTenantFromContext(ctx)

		propagatedHeaders := extractHeaders(r, "Accept", "Link")

		ctx, span := tracer.Start(ctx, "query-entities",
			trace.WithAttributes(attribute.String(TraceAttributeNGSILDTenant, tenant)),
		)
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

		traceID, ctx, log := o11y.AddTraceIDToLoggerAndStoreInContext(span, logger, ctx)

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

		options := r.URL.Query().Get("options")
		keyValueFormatRequested := false

		if options != "" && strings.Contains(options, "keyValues") {
			opts := strings.Split(options, ",")
			numOptions := len(opts)
			if numOptions == 1 {
				q := r.URL.Query()
				q.Del("options")
				r.URL.RawQuery = q.Encode()

				keyValueFormatRequested = true
			} else {
				err = errors.New("no options besides keyValues are supported")
				ngsierrors.ReportNewBadRequestData(w, err.Error(), traceID)
				return
			}
		}

		entityTypes := strings.Split(entityTypeNames, ",")
		attributes := strings.Split(attributeNames, ",")

		err = authenticator.CheckAccess(ctx, r, tenant, entityTypes)
		if err != nil {
			log.Warn("access not granted", "err", err.Error())
			messageToSendToNonAuthenticatedClients := "not found"
			ngsierrors.ReportNotFoundError(w, messageToSendToNonAuthenticatedClients, traceID)
			return
		}

		result, err := contextInformationManager.QueryEntities(ctx, tenant, entityTypes, attributes, r.URL.Path+"?"+r.URL.RawQuery, propagatedHeaders)
		if err != nil {
			log.Error("query entities failed", "err", err.Error())
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		contentType := r.Header.Get("Accept")
		if contentType == "" {
			contentType = "application/ld+json"
		}

		var entityConverter func(ngsitypes.Entity) ngsitypes.Entity

		var geoJsonCollection *geojson.GeoJSONFeatureCollection
		var entityCollection []ngsitypes.Entity
		var entityKeyValues []ngsitypes.EntityKeyValueMapper

		if contentType == "application/geo+json" {
			geoJsonCollection = geojson.NewFeatureCollection()
			entityConverter = func(e ngsitypes.Entity) ngsitypes.Entity {
				gje, err := geojson.ConvertEntity(e)
				if err == nil {
					geoJsonCollection.Features = append(geoJsonCollection.Features, *gje)
				}
				return e
			}
		} else if !keyValueFormatRequested {
			entityCollection = []ngsitypes.Entity{}
			entityConverter = func(e ngsitypes.Entity) ngsitypes.Entity {
				entityCollection = append(entityCollection, e)
				return e
			}
		} else {
			entityKeyValues = []ngsitypes.EntityKeyValueMapper{}
			entityConverter = func(e ngsitypes.Entity) ngsitypes.Entity {
				entityKeyValues = append(entityKeyValues, e.KeyValues())
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
		} else if entityCollection != nil {
			responseBody, err = json.Marshal(entityCollection)
		} else {
			responseBody, err = json.Marshal(entityKeyValues)
		}

		if err != nil {
			log.Error("query entities: failed to marshal entity collection to json", "err", err.Error())
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		w.Header().Add("Content-Type", contentType)
		if result.TotalCount >= 0 {
			w.Header().Add("NGSILD-Results-Count", fmt.Sprintf("%d", result.TotalCount))
		}

		if result.PartialResult {
			reqUrl := r.URL.Path
			query, _ := url.ParseQuery(r.URL.RawQuery)
			query.Set("limit", fmt.Sprintf("%d", result.Limit))

			offset := result.Offset - result.Limit
			if offset >= 0 {
				query.Set("offset", fmt.Sprintf("%d", offset))
				w.Header().Add("Link", fmt.Sprintf(`<%s?%s>; rel="prev"; type="%s"`, reqUrl, query.Encode(), contentType))
			}

			offset = result.Offset + result.Limit
			if offset < int(result.TotalCount) {
				query.Set("offset", fmt.Sprintf("%d", offset))
				w.Header().Add("Link", fmt.Sprintf(`<%s?%s>; rel="next"; type="%s"`, reqUrl, query.Encode(), contentType))
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write(responseBody)
	})
}

// NewRetrieveEntityHandler retrieves entity by ID.
func NewRetrieveEntityHandler(
	contextInformationManager cim.EntityRetriever,
	authenticator auth.Enticator,
	logger *slog.Logger) http.HandlerFunc {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx := r.Context()
		tenant := GetTenantFromContext(ctx)
		entityID, _ := url.QueryUnescape(chi.URLParam(r, "entityId"))

		propagatedHeaders := extractHeaders(r, "Accept", "Link")

		ctx, span := tracer.Start(ctx, "retrieve-entity",
			trace.WithAttributes(
				attribute.String(TraceAttributeNGSILDTenant, tenant),
				attribute.String(TraceAttributeEntityID, entityID),
			),
		)
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

		traceID, ctx, log := o11y.AddTraceIDToLoggerAndStoreInContext(
			span,
			logger.With(slog.String("entityID", entityID), slog.String("tenant", tenant)),
			ctx)

		options := r.URL.Query().Get("options")
		keyValueFormatRequested := false

		if options != "" && strings.Contains(options, "keyValues") {
			opts := strings.Split(options, ",")
			numOptions := len(opts)
			if numOptions == 1 {
				q := r.URL.Query()
				q.Del("options")
				r.URL.RawQuery = q.Encode()

				keyValueFormatRequested = true
			} else {
				err = errors.New("no options besides keyValues are supported")
				ngsierrors.ReportNewBadRequestData(w, err.Error(), traceID)
				return
			}
		}

		var entity ngsitypes.Entity
		entity, err = contextInformationManager.RetrieveEntity(ctx, tenant, entityID, propagatedHeaders)

		if err == nil {
			// Checking access after we have retrieved the entity allows us to use the type
			// information when we decide if the client is allowed to retrieve this entity or not
			autherr := authenticator.CheckAccess(ctx, r, tenant, []string{entity.Type()})
			if autherr != nil {
				err = autherr
				log.Warn("access not granted", "err", err.Error())
				messageToSendToNonAuthenticatedClients := "not found"
				ngsierrors.ReportNotFoundError(w, messageToSendToNonAuthenticatedClients, traceID)
				return
			}
		}

		if err != nil {
			log.Error("retrieve entity failed", "err", err.Error())
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
		} else if !keyValueFormatRequested {
			responseBody, err = json.Marshal(entity)
		} else {
			responseBody, err = json.Marshal(entity.KeyValues())
		}

		if err != nil {
			log.Error("failed to convert or marshal response entity", "err", err.Error())
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		w.Header().Add("Content-Type", responseContentType)
		w.Write(responseBody)
	})
}

// NewMergeEntityHandler handles PATCH requests for NGSI entitities
func NewMergeEntityHandler(
	contextInformationManager cim.EntityMerger,
	authenticator auth.Enticator,
	logger *slog.Logger) http.HandlerFunc {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx := r.Context()
		tenant := GetTenantFromContext(ctx)
		entityID, _ := url.QueryUnescape(chi.URLParam(r, "entityId"))

		propagatedHeaders := extractHeaders(r, "Content-Type", "Link")

		ctx, span := tracer.Start(ctx, "merge-entity",
			trace.WithAttributes(
				attribute.String(TraceAttributeNGSILDTenant, tenant),
				attribute.String(TraceAttributeEntityID, entityID),
			),
		)
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

		traceID, ctx, log := o11y.AddTraceIDToLoggerAndStoreInContext(
			span,
			logger.With(slog.String("entityID", entityID), slog.String("tenant", tenant)),
			ctx)

		var entity ngsitypes.EntityFragment
		body, _ := io.ReadAll(r.Body)
		entity, err = entities.NewFragmentFromJSON(body)

		if err != nil {
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		err = authenticator.CheckAccess(ctx, r, tenant, []string{})
		if err != nil {
			log.Warn("access not granted", "err", err.Error())
			messageToSendToNonAuthenticatedClients := "not found"
			ngsierrors.ReportNotFoundError(w, messageToSendToNonAuthenticatedClients, traceID)
			return
		}

		_, err = contextInformationManager.MergeEntity(ctx, tenant, entityID, entity, propagatedHeaders)

		if err != nil {
			log.Error("failed to merge entity attributes", "err", err.Error())
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		log.Info("entities merged")

		w.WriteHeader(http.StatusNoContent)
	})
}

// NewUpdateEntityAttributesHandler handles PATCH requests for NGSI entitity attributes
func NewUpdateEntityAttributesHandler(
	contextInformationManager cim.EntityAttributesUpdater,
	authenticator auth.Enticator,
	logger *slog.Logger) http.HandlerFunc {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx := r.Context()
		tenant := GetTenantFromContext(ctx)
		entityID, _ := url.QueryUnescape(chi.URLParam(r, "entityId"))

		propagatedHeaders := extractHeaders(r, "Content-Type", "Link")

		ctx, span := tracer.Start(ctx, "update-entity-attributes",
			trace.WithAttributes(
				attribute.String(TraceAttributeNGSILDTenant, tenant),
				attribute.String(TraceAttributeEntityID, entityID),
			),
		)
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

		traceID, ctx, log := o11y.AddTraceIDToLoggerAndStoreInContext(
			span,
			logger.With(slog.String("entityID", entityID), slog.String("tenant", tenant)),
			ctx)

		var entity ngsitypes.EntityFragment
		body, _ := io.ReadAll(r.Body)
		entity, err = entities.NewFragmentFromJSON(body)

		if err != nil {
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		err = authenticator.CheckAccess(ctx, r, tenant, []string{})
		if err != nil {
			log.Warn("access not granted", "err", err.Error())
			messageToSendToNonAuthenticatedClients := "not found"
			ngsierrors.ReportNotFoundError(w, messageToSendToNonAuthenticatedClients, traceID)
			return
		}

		updateResult, err := contextInformationManager.UpdateEntityAttributes(ctx, tenant, entityID, entity, propagatedHeaders)

		if err != nil {
			log.Error("failed to update entity attributes", "err", err.Error())
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		log.Info("entity attributes updated")

		if !updateResult.IsMultiStatus() {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.WriteHeader(http.StatusMultiStatus)
			w.Write(updateResult.Bytes())
		}
	})
}

func NewDeleteEntityHandler(
	contextInformationManager cim.EntityDeleter,
	authenticator auth.Enticator,
	logger *slog.Logger) http.HandlerFunc {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx := r.Context()
		tenant := GetTenantFromContext(ctx)
		entityID, _ := url.QueryUnescape(chi.URLParam(r, "entityId"))

		ctx, span := tracer.Start(ctx, "delete-entity",
			trace.WithAttributes(
				attribute.String(TraceAttributeNGSILDTenant, tenant),
				attribute.String(TraceAttributeEntityID, entityID),
			),
		)
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

		traceID, ctx, log := o11y.AddTraceIDToLoggerAndStoreInContext(
			span,
			logger.With(slog.String("entityID", entityID), slog.String("tenant", tenant)),
			ctx)

		err = authenticator.CheckAccess(ctx, r, tenant, []string{})
		if err != nil {
			log.Warn("access not granted", "err", err.Error())
			ngsierrors.ReportUnauthorizedRequest(w, "not authorized", traceID)
			return
		}

		_, err = contextInformationManager.DeleteEntity(ctx, tenant, entityID)

		if err != nil {
			log.Error("failed to delete entity", "err", err.Error())
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		log.Info("entity deleted")

		w.WriteHeader(http.StatusNoContent)
	})
}

func extractHeaders(r *http.Request, headers ...string) map[string][]string {
	extractedHeaders := map[string][]string{}

	for _, header := range headers {
		headerValue, ok := r.Header[header]
		if ok {
			if header == "Content-Type" {
				headerValue[0] = strings.Split(headerValue[0], ";")[0]
			}
			extractedHeaders[header] = headerValue
		}
	}

	return extractedHeaders
}

func mapCIMToNGSILDError(w http.ResponseWriter, err error, traceID string) {

	switch {
	case errors.Is(err, ngsierrors.ErrAlreadyExists):
		ngsierrors.ReportNewAlreadyExistsError(w, err.Error(), traceID)
	case errors.Is(err, ngsierrors.ErrBadRequest):
		ngsierrors.ReportNewBadRequestData(w, err.Error(), traceID)
	case errors.Is(err, ngsierrors.ErrInvalidRequest):
		ngsierrors.ReportNewInvalidRequest(w, err.Error(), traceID)
	case errors.Is(err, ngsierrors.ErrNotFound):
		ngsierrors.ReportNotFoundError(w, err.Error(), traceID)
	case errors.Is(err, ngsierrors.ErrUnknownTenant):
		ngsierrors.ReportUnknownTenantError(w, err.Error(), traceID)
	default:
		ngsierrors.ReportNewInternalError(w, err.Error(), traceID)
	}
}
