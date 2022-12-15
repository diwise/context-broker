package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	ngsierrors "github.com/diwise/context-broker/pkg/ngsild/errors"
	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities/decorators"
	"github.com/diwise/context-broker/pkg/ngsild/types/properties"
	testutils "github.com/diwise/service-chassis/pkg/test/http"
	"github.com/diwise/service-chassis/pkg/test/http/expects"
	"github.com/diwise/service-chassis/pkg/test/http/response"

	"github.com/matryer/is"
)

var Expects = testutils.Expects
var Returns = testutils.Returns
var anyInput = expects.AnyInput
var method = expects.RequestMethod
var path = expects.RequestPath
var body = expects.RequestBody

func TestCreateEntity(t *testing.T) {
	is := is.New(t)

	locationHeader := "/ngsi-ld/v1/entities/id"
	s := testutils.NewMockServiceThat(
		Expects(
			is,
			method(http.MethodPost),
			path("/ngsi-ld/v1/entities"),
			body("{\"@context\":[\"https://raw.githubusercontent.com/diwise/context-broker/main/assets/jsonldcontexts/default-context.jsonld\"],\"id\":\"id\",\"type\":\"Road\"}"),
		),
		Returns(
			response.ContentType("application/ld+json"),
			response.Location(locationHeader),
			response.Code(http.StatusCreated),
		),
	)
	defer s.Close()

	c := NewContextBrokerClient(s.URL())

	result, err := c.CreateEntity(context.Background(), testEntity("Road", "id"), nil)

	is.NoErr(err)
	is.Equal(result.Location(), locationHeader)
}

func TestCreateEntityHandlesMissingLocationheader(t *testing.T) {
	is := is.New(t)

	s := testutils.NewMockServiceThat(
		Expects(is, anyInput()),
		Returns(
			response.ContentType("application/ld+json"),
			response.Code(http.StatusCreated),
		),
	)
	defer s.Close()

	c := NewContextBrokerClient(s.URL())

	result, err := c.CreateEntity(context.Background(), testEntity("Road", "id"), nil)

	is.NoErr(err)
	is.Equal(result.Location(), "/ngsi-ld/v1/entities/id")
}

func TestCreateEntityThrowsErrorOnNon201Success(t *testing.T) {
	is := is.New(t)

	s := testutils.NewMockServiceThat(
		Expects(is, anyInput()),
		Returns(response.Code(http.StatusNoContent)),
	)
	defer s.Close()

	c := NewContextBrokerClient(s.URL())

	_, err := c.CreateEntity(context.Background(), testEntity("Road", "id"), nil)

	is.True(err != nil)
	is.Equal(err.Error(), "unexpected response code 204 (internal error)")
}

func TestCreateEntityHandlesBadRequestError(t *testing.T) {
	is := is.New(t)

	pr := ngsierrors.NewBadRequestData("bad request", "traceID")
	b, _ := json.Marshal(pr)

	s := testutils.NewMockServiceThat(
		Expects(is, anyInput()),
		Returns(
			response.ContentType("application/problem+json"),
			response.Code(http.StatusBadRequest),
			response.Body(b),
		),
	)
	defer s.Close()

	c := NewContextBrokerClient(s.URL())

	_, err := c.CreateEntity(context.Background(), testEntity("A", "id"), nil)

	is.True(err != nil)
	is.True(errors.Is(err, ngsierrors.ErrBadRequest))
}

func TestMergeEntity(t *testing.T) {
	is := is.New(t)

	s := testutils.NewMockServiceThat(
		Expects(
			is,
			method(http.MethodPatch),
			path("/ngsi-ld/v1/entities/id"),
			body("{\"@context\":[\"https://raw.githubusercontent.com/diwise/context-broker/main/assets/jsonldcontexts/default-context.jsonld\"],\"id\":\"id\",\"type\":\"Road\"}"),
		),
		Returns(response.Code(http.StatusNoContent)),
	)
	defer s.Close()

	c := NewContextBrokerClient(s.URL())

	_, err := c.MergeEntity(context.Background(), "id", testEntity("Road", "id"), nil)

	is.NoErr(err)
	is.Equal(s.RequestCount(), 1)
}

func TestUpdateEntityAttributesWithMetaData(t *testing.T) {
	is := is.New(t)

	s := testutils.NewMockServiceThat(
		Expects(
			is,
			method(http.MethodPatch),
			path("/ngsi-ld/v1/entities/id/attrs/"),
			body(
				"{\"@context\":[\"https://raw.githubusercontent.com/diwise/context-broker/main/assets/jsonldcontexts/default-context.jsonld\"],\"waterConsumption\":{\"type\":\"Property\",\"value\":100,\"observedAt\":\"2006-01-02T15:04:05Z\",\"observedBy\":{\"type\":\"Relationship\",\"object\":\"some_device\"},\"unitCode\":\"LTR\"}}",
			),
		),
		Returns(
			response.Code(http.StatusNoContent),
		),
	)
	defer s.Close()

	c := NewContextBrokerClient(s.URL())

	props := []entities.EntityDecoratorFunc{
		decorators.Number("waterConsumption", 100.0, properties.UnitCode("LTR"), properties.ObservedAt("2006-01-02T15:04:05Z"), properties.ObservedBy("some_device")),
	}

	fragment, _ := entities.NewFragment(props...)
	_, err := c.UpdateEntityAttributes(context.Background(), "id", fragment, nil)
	is.NoErr(err)
	is.Equal(s.RequestCount(), 1)
}

func TestDeleteEntity(t *testing.T) {
	is := is.New(t)

	s := testutils.NewMockServiceThat(
		Expects(
			is,
			method(http.MethodDelete),
			path("/ngsi-ld/v1/entities/id"),
			body(""),
		),
		Returns(response.Code(http.StatusNoContent)),
	)
	defer s.Close()

	c := NewContextBrokerClient(s.URL())

	_, err := c.DeleteEntity(context.Background(), "id")

	is.NoErr(err)
	is.Equal(s.RequestCount(), 1)	
}

func testEntity(entityType, entityID string) types.Entity {
	e, _ := entities.New(entityID, entityType)
	return e
}
