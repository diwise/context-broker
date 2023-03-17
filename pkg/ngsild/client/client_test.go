package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

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

func TestDeleteEntityNotFound(t *testing.T) {
	is := is.New(t)

	pr := ngsierrors.NewNotFound("not found", "traceID")
	b, _ := json.Marshal(pr)

	s := testutils.NewMockServiceThat(
		Expects(is, anyInput()),
		Returns(
			response.ContentType("application/problem+json"),
			response.Code(http.StatusNotFound),
			response.Body(b),
		),
	)
	defer s.Close()

	c := NewContextBrokerClient(s.URL())

	_, err := c.DeleteEntity(context.Background(), "id")

	is.True(err != nil)
	is.True(errors.Is(err, ngsierrors.ErrNotFound))
}

func TestRetrieveTemporalEvolutionOfAnEntity(t *testing.T) {
	is := is.New(t)

	timeStr := "2023-01-22T11:59:43Z"

	s := testutils.NewMockServiceThat(
		Expects(
			is,
			expects.RequestPath("/ngsi-ld/v1/temporal/entities/id"),
			QueryParamEquals("timerel", "after"),
			QueryParamEquals("timeAt", timeStr),
		),
		Returns(
			response.ContentType("application/ld+json"),
			response.Code(http.StatusOK),
			response.Body([]byte(temporalEntityResponse)),
		),
	)
	defer s.Close()

	headers := map[string][]string{"Accept": {"application/ld+json"}}
	timeAt, _ := time.Parse(time.RFC3339, timeStr)

	c := NewContextBrokerClient(s.URL())
	_, err := c.RetrieveTemporalEvolutionOfEntity(context.Background(), "id", headers, After(timeAt))

	is.NoErr(err)
}

func TestRetrieveTemporalEvolutionOfAnEntityWithSingleValue(t *testing.T) {
	is := is.New(t)

	timeStr := "2023-01-22T11:59:43Z"

	s := testutils.NewMockServiceThat(
		Expects(is, expects.AnyInput()),
		Returns(
			response.ContentType("application/ld+json"),
			response.Code(http.StatusOK),
			response.Body([]byte(temporalEntityResponseWithSingleValue)),
		),
	)
	defer s.Close()

	headers := map[string][]string{"Accept": {"application/ld+json"}}
	timeAt, _ := time.Parse(time.RFC3339, timeStr)

	c := NewContextBrokerClient(s.URL())
	et, err := c.RetrieveTemporalEvolutionOfEntity(context.Background(), "id", headers, After(timeAt))
	is.NoErr(err)

	etBytes, err := json.Marshal(et)
	is.NoErr(err)

	const expectation string = `{"@context":["http://example.org/ngsi-ld/latest/vehicle.jsonld","https://uri.etsi.org/ngsi-ld/v1/ngsi-ld-core-context-v1.5.jsonld"],"id":"urn:ngsi-ld:Vehicle:B9211","speed":[{"type":"Property","value":120,"observedAt":"2018-08-01T12:03:00Z"}],"type":"Vehicle"}`
	is.Equal(string(etBytes), expectation)
}

func TestRetrieveAggregatedTemporalEvolutionOfAnEntity(t *testing.T) {
	is := is.New(t)

	s := testutils.NewMockServiceThat(
		Expects(
			is,
			expects.RequestPath("/ngsi-ld/v1/temporal/entities/id"),
			QueryParamContains("aggrMethods", "max"),
			QueryParamEquals("aggrPeriodDuration", "P1D"),
			QueryParamEquals("options", "aggregatedValues"),
		),
		Returns(
			response.ContentType("application/ld+json"),
			response.Code(http.StatusOK),
			response.Body([]byte(temporalEntityResponse)),
		),
	)
	defer s.Close()

	headers := map[string][]string{"Accept": {"application/ld+json"}}

	c := NewContextBrokerClient(s.URL())
	_, err := c.RetrieveTemporalEvolutionOfEntity(context.Background(), "id", headers,
		Aggregation(
			[]AggregationMethod{AggregatedMax, AggregatedMin},
			ByDay(),
		))

	is.NoErr(err)
}

func testEntity(entityType, entityID string) types.Entity {
	e, _ := entities.New(entityID, entityType)
	return e
}

const temporalEntityResponse string = `{
	"id":"urn:ngsi-ld:Vehicle:B9211", "type":"Vehicle",
"speed":[
{
"type":"Property",
"value":120, "observedAt":"2018-08-01T12:03:00Z"
}, {
"type":"Property",
"value":80, "observedAt":"2018-08-01T12:05:00Z"
}, {
"type":"Property",
"value":100, "observedAt":"2018-08-01T12:07:00Z"
} ],
"@context":[
"http://example.org/ngsi-ld/latest/vehicle.jsonld", "https://uri.etsi.org/ngsi-ld/v1/ngsi-ld-core-context-v1.5.jsonld"
] }`

const temporalEntityResponseWithSingleValue string = `{
	"id":"urn:ngsi-ld:Vehicle:B9211", "type":"Vehicle",
	"speed":{
		"type":"Property",
		"value":120, "observedAt":"2018-08-01T12:03:00Z"
	},
	"@context":[
		"http://example.org/ngsi-ld/latest/vehicle.jsonld",
		"https://uri.etsi.org/ngsi-ld/v1/ngsi-ld-core-context-v1.5.jsonld"
	]
}`

func QueryParamContains(name, value string) func(*is.I, *http.Request) {
	return func(is *is.I, r *http.Request) {
		is.True(r.URL.Query().Has(name)) // query param should exist

		for _, v := range strings.Split(r.URL.Query().Get(name), ",") {
			if v == value {
				return // it is a match!
			}
		}

		is.Fail() // query params did not contain expected value
	}
}

func QueryParamEquals(name, value string) func(*is.I, *http.Request) {
	return func(is *is.I, r *http.Request) {
		is.True(r.URL.Query().Has(name))         // query param should exist
		is.Equal(r.URL.Query().Get(name), value) // query param should match
	}
}
