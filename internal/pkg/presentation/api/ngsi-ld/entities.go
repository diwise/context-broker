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
	"github.com/diwise/ngsi-ld-context-broker/internal/pkg/presentation/api/ngsi-ld/geojson"
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
			mapCIMToNGSILDError(w, err)
			return
		}

		onsuccess(ctx, entity.Type, entity.ID, logger)

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
			mapCIMToNGSILDError(w, err)
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
			responseBody, err = json.Marshal(&entityCollection)
		}

		w.Header().Add("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		// TODO: Add a RFC 8288 Link header with information about previous and/or next page if they exist
		w.Write(responseBody)
	})
}

func mapCIMToNGSILDError(w http.ResponseWriter, err error) {
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
}
