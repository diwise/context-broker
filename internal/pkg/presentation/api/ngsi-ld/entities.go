package ngsild

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/diwise/ngsi-ld-context-broker/internal/pkg/application/cim"
	"github.com/diwise/ngsi-ld-context-broker/internal/pkg/presentation/api/ngsi-ld/errors"
	"github.com/diwise/ngsi-ld-context-broker/internal/pkg/presentation/api/ngsi-ld/types"
	"github.com/rs/zerolog"

	"go.opentelemetry.io/otel"
)

type CreateEntityCompletionCallback func(ctx context.Context, entityType, entityID string, logger zerolog.Logger)

//NewCreateEntityHandler handles incoming POST requests for NGSI entities
func NewCreateEntityHandler(
	contextInformationManager cim.EntityCreator,
	logger zerolog.Logger,
	onsuccess CreateEntityCompletionCallback) http.HandlerFunc {

	var tracer = otel.Tracer("context-broker/ngsi-ld/entities")

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
			errors.ReportNewInvalidRequest(
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
				errors.ReportNewAlreadyExistsError(w, e.Error())
			case cim.NotFoundError:
				errors.ReportNotFoundError(w, e.Error())
			case cim.UnknownTenantError:
				errors.ReportUnknownTenantError(w, e.Error())
			default:
				errors.ReportNewInternalError(w, e.Error())
			}

			return
		}

		onsuccess(ctx, entity.Type, entity.ID, logger)

		// FIXME: Make sure that the Location header can propagate up the federation tree properly
		w.Header().Add("Location", result.Location())
		w.WriteHeader(http.StatusCreated)
	})
}
