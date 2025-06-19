package ngsild

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/diwise/context-broker/internal/pkg/application/cim"
	"github.com/diwise/context-broker/internal/pkg/presentation/api/ngsi-ld/auth"
	"github.com/diwise/context-broker/pkg/ngsild"
	ngsierrors "github.com/diwise/context-broker/pkg/ngsild/errors"
	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func NewQueryTemporalEvolutionOfEntitiesHandler(
	contextInformationManager cim.EntityTemporalQuerier,
	authenticator auth.Enticator,
	logger *slog.Logger) http.HandlerFunc {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx := r.Context()
		tenant := GetTenantFromContext(ctx)

		propagatedHeaders := extractHeaders(r, "Accept", "Link")

		ctx, span := tracer.Start(ctx, "query-temporal-entities",
			trace.WithAttributes(
				attribute.String(TraceAttributeNGSILDTenant, tenant),
			),
		)
		defer func() { tracing.RecordAnyErrorAndEndSpan(err, span) }()

		contentType := r.Header.Get("Accept")
		if contentType == "" {
			contentType = "application/ld+json"
		}

		traceID, ctx, log := o11y.AddTraceIDToLoggerAndStoreInContext(
			span,
			logger.With(slog.String("tenant", tenant)),
			ctx)

		err = authenticator.CheckAccess(ctx, r, tenant, []string{})
		if err != nil {
			log.Warn("access not granted", "err", err.Error())
			messageToSendToNonAuthenticatedClients := "not found"
			ngsierrors.ReportNotFoundError(w, messageToSendToNonAuthenticatedClients, traceID)
			return
		}

		var params cim.TemporalQueryParams
		params, err = NewTemporalQueryParamsFromRequest(r)

		if err != nil {
			log.Error("failed to create rteoe query parameters from request", "err", err.Error())
			ngsierrors.ReportNewBadRequestData(w, err.Error(), traceID)
			return
		}

		entityIDs, _ := params.IDs()
		entityTypes, _ := params.Types()

		var result *ngsild.QueryTemporalEntitiesResult
		result, err = contextInformationManager.QueryTemporalEvolutionOfEntities(ctx, tenant, entityIDs, entityTypes, params, propagatedHeaders)

		if err != nil {
			log.Error("failed to retrieve temporal evolution of entities", "err", err.Error())
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		temporals := make([]types.EntityTemporal, 0, 200)

		for e := range result.Found {
			if e == nil {
				break
			}

			temporals = append(temporals, e)
		}

		responseBody, err := json.Marshal(temporals)

		if err != nil {
			log.Error("failed to convert or marshal response entity", "err", err.Error())
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		w.Header().Add("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		w.Write(responseBody)
	})
}

func NewRetrieveTemporalEvolutionOfAnEntityHandler(
	contextInformationManager cim.EntityTemporalRetriever,
	authenticator auth.Enticator,
	logger *slog.Logger) http.HandlerFunc {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		ctx := r.Context()
		tenant := GetTenantFromContext(ctx)
		entityID, _ := url.QueryUnescape(r.PathValue("entityId"))

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

		traceID, ctx, log := o11y.AddTraceIDToLoggerAndStoreInContext(
			span,
			logger.With(slog.String("entityID", entityID), slog.String("tenant", tenant)),
			ctx)

		err = authenticator.CheckAccess(ctx, r, tenant, []string{})
		if err != nil {
			log.Warn("access not granted", "err", err.Error())
			messageToSendToNonAuthenticatedClients := "not found"
			ngsierrors.ReportNotFoundError(w, messageToSendToNonAuthenticatedClients, traceID)
			return
		}

		var params cim.TemporalQueryParams

		params, err = NewTemporalQueryParamsFromRequest(r)
		if err != nil {
			log.Error("failed to create rteoe query parameters from request", "err", err.Error())
			ngsierrors.ReportNewBadRequestData(w, err.Error(), traceID)
			return
		}

		result, err := contextInformationManager.RetrieveTemporalEvolutionOfEntity(ctx, tenant, entityID, params, propagatedHeaders)

		if err != nil {
			log.Error("failed to retrieve temporal evolution of an entity", "err", err.Error())
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		responseBody, err := json.Marshal(result.Found)

		if err != nil {
			log.Error("failed to convert or marshal response entity", "err", err.Error())
			mapCIMToNGSILDError(w, err, traceID)
			return
		}

		w.Header().Add("Content-Type", contentType)

		if result.PartialResult {
			if result.ContentRange == nil {
				mapCIMToNGSILDError(w, fmt.Errorf("content range missing for partial result"), traceID)
				return
			}

			startTime := result.ContentRange.StartTime.UTC().Format(time.RFC3339)
			endTime := result.ContentRange.EndTime.UTC().Format(time.RFC3339)
			contentRangeHeader := fmt.Sprintf("DateTime %s-%s/*", startTime, endTime)

			w.Header().Add("Content-Range", contentRangeHeader)
			if _, ok := params.LastN(); !ok {
				w.Header().Add("Link", createLinkHeader(params, result, r, entityID, contentType, "self"))
				w.Header().Add("Link", createLinkHeader(params, result, r, entityID, contentType, "next"))
			}

			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		w.Write(responseBody)
	})
}

const beforeLinkResponse string = `%s/ngsi-ld/v1/temporal/entities/%s?timeAt=%s&timerel=before`
const betweenLinkResponse string = `%s/ngsi-ld/v1/temporal/entities/%s?endTimeAt=%s&timeAt=%s&timerel=between`
const afterLinkResponse string = `%s/ngsi-ld/v1/temporal/entities/%s?timeAt=%s&timerel=after`

func createLinkHeader(params cim.TemporalQueryParams, result *ngsild.RetrieveTemporalEvolutionOfEntityResult, r *http.Request, entityID, contentType, pageName string) string {
	rel, _ := params.TemporalRelation()
	resultStartTime := result.ContentRange.StartTime.Format(time.RFC3339)
	resultEndTime := result.ContentRange.EndTime.Format(time.RFC3339)

	requestStartTime, _ := params.TimeAt()
	requestEndTime, _ := params.EndTimeAt()

	hostName := r.Header.Get("X-Forwarded-For")

	switch rel {
	case "before":
		link := ""
		if pageName == "self" {
			link = fmt.Sprintf(beforeLinkResponse, hostName, entityID, resultStartTime)
		} else if pageName == "next" {
			link = fmt.Sprintf(beforeLinkResponse, hostName, entityID, resultEndTime)
		}
		return fmt.Sprintf(`<%s>; rel="%s"; type="%s"`, link, pageName, contentType)
	case "between":
		link := ""
		if pageName == "self" {
			link = fmt.Sprintf(betweenLinkResponse, hostName, entityID, resultEndTime, resultStartTime)
		} else if pageName == "next" {
			link = fmt.Sprintf(betweenLinkResponse, hostName, entityID, requestEndTime.Format(time.RFC3339), resultEndTime)
		}
		return fmt.Sprintf(`<%s>; rel="%s"; type="%s"`, link, pageName, contentType)
	case "after":
		link := ""
		if pageName == "self" {
			link = fmt.Sprintf(afterLinkResponse, hostName, entityID, requestStartTime.Format(time.RFC3339))
		} else if pageName == "next" {
			link = fmt.Sprintf(afterLinkResponse, hostName, entityID, resultEndTime)
		}
		return fmt.Sprintf(`<%s>; rel="%s"; type="%s"`, link, pageName, contentType)
	}

	return ""
}

func NewTemporalQueryParamsFromRequest(r *http.Request) (cim.TemporalQueryParams, error) {
	qp := &queryParams{
		ids:          []string{},
		types:        []string{},
		attributes:   []string{},
		timeProperty: "observedAt",
	}
	var err error

	timeproperty := r.URL.Query().Get("timeproperty")
	if timeproperty != "" {
		qp.timeProperty = timeproperty
	}

	qp.temporalRelation = r.URL.Query().Get("timerel")
	if qp.temporalRelation != "" {
		if qp.temporalRelation != "before" && qp.temporalRelation != "between" && qp.temporalRelation != "after" {
			return nil, errors.New("temporal relation timerel must be one of ['before', 'between', 'after']")
		}

		parseTimeParamValueByName := func(name string) (time.Time, error) {
			return parseTimeParamValue(r.URL.Query().Get(name), name)
		}

		qp.timeAt, err = parseTimeParamValueByName("timeAt")
		if err != nil {
			return nil, err
		}

		if qp.timeAt.IsZero() {
			return nil, errors.New("temporal queries with a relation must include a timeAt parameter")
		}

		if qp.temporalRelation == "between" {
			qp.endTimeAt, err = parseTimeParamValueByName("endTimeAt")
			if err != nil {
				return nil, err
			}

			if qp.endTimeAt.IsZero() {
				return nil, errors.New("temporal queries with relation 'between' must include an endTimeAt parameter")
			}
		}
	}

	ids := r.URL.Query().Get("id")
	if ids != "" {
		qp.ids = strings.Split(ids, ",")
	}

	types := r.URL.Query().Get("type")
	if types != "" {
		qp.types = strings.Split(types, ",")
	}

	attributes := r.URL.Query().Get("attributes")
	if attributes != "" {
		qp.attributes = strings.Split(attributes, ",")
	}

	lastNStr := r.URL.Query().Get("lastN")
	if lastNStr != "" {
		qp.lastN, err = strconv.ParseUint(lastNStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("unable to parse lastN query parameter: %w", err)
		}
	}

	options := r.URL.Query().Get("options")
	if options != "" {
		if strings.Contains(options, "aggregatedValues") {
			aggrMethods := r.URL.Query().Get("aggrMethods")
			if aggrMethods == "" {
				return nil, fmt.Errorf("aggregation of temporal values requires that the aggregation method is specified")
			}

			qp.aggregationMethods = strings.Split(aggrMethods, ",")
			qp.aggregationperiodDuration = "P0D"

			duration := r.URL.Query().Get("aggrPeriodDuration")
			if duration != "" {
				// TODO: Validate that it is a valid ISO 8601 duration
				qp.aggregationperiodDuration = duration
			}
		}
	}

	return qp, nil
}

type queryParams struct {
	ids              []string
	types            []string
	attributes       []string
	timeProperty     string
	temporalRelation string
	timeAt           time.Time
	endTimeAt        time.Time
	lastN            uint64

	aggregationMethods        []string
	aggregationperiodDuration string
}

func (qp *queryParams) IDs() ([]string, bool) {
	return qp.ids, (len(qp.ids) > 0)
}

func (qp *queryParams) Types() ([]string, bool) {
	return qp.types, (len(qp.types) > 0)
}

func (qp *queryParams) Attributes() ([]string, bool) {
	return qp.attributes, (len(qp.attributes) > 0)
}

func (qp *queryParams) TemporalRelation() (string, bool) {
	if qp.temporalRelation == "" {
		return "", false
	}

	return qp.temporalRelation, true
}

func (qp *queryParams) EndTimeAt() (time.Time, bool) {
	return qp.endTimeAt, !qp.endTimeAt.IsZero()
}

func (qp *queryParams) TimeAt() (time.Time, bool) {
	return qp.timeAt, !qp.timeAt.IsZero()
}

func (qp *queryParams) LastN() (uint64, bool) {
	return qp.lastN, (qp.lastN > 0)
}

func parseTimeParamValue(t, paramName string) (time.Time, error) {
	if t == "" {
		return time.Time{}, nil
	}

	timeAt, err := time.Parse(time.RFC3339, t)
	if err != nil {
		return time.Time{}, fmt.Errorf("unable to parse %s from query parameter: %w", paramName, err)
	}

	return timeAt, nil
}
