package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	ngsierrors "github.com/diwise/context-broker/pkg/ngsild/errors"
	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities/decorators"
	"github.com/diwise/context-broker/pkg/ngsild/types/properties"
	"github.com/matryer/is"
)

func TestCreateEntity(t *testing.T) {
	is := is.New(t)

	locationHeader := "/ngsi-ld/v1/entities/id"
	s := setupMockServiceThat(
		expects(
			is,
			requestMethod(http.MethodPost),
			requestPath("/ngsi-ld/v1/entities"),
			requestBody("{\"@context\":[\"https://raw.githubusercontent.com/diwise/context-broker/main/assets/jsonldcontexts/default-context.jsonld\"],\"id\":\"id\",\"type\":\"Road\"}"),
		),
		returns(
			contenttype("application/ld+json"),
			location(locationHeader),
			responseCode(http.StatusCreated),
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

	s := setupMockServiceThat(
		expects(is, anyInput()),
		returns(
			contenttype("application/ld+json"),
			responseCode(http.StatusCreated),
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

	s := setupMockServiceThat(
		expects(is, anyInput()),
		returns(responseCode(http.StatusNoContent)),
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

	s := setupMockServiceThat(
		expects(is, anyInput()),
		returns(
			contenttype("application/problem+json"),
			responseCode(http.StatusBadRequest),
			responseBody(b),
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

	s := setupMockServiceThat(
		expects(
			is,
			requestMethod(http.MethodPatch),
			requestPath("/ngsi-ld/v1/entities/id"),
			requestBody("{\"@context\":[\"https://raw.githubusercontent.com/diwise/context-broker/main/assets/jsonldcontexts/default-context.jsonld\"],\"id\":\"id\",\"type\":\"Road\"}"),
		),
		returns(responseCode(http.StatusNoContent)),
	)
	defer s.Close()

	c := NewContextBrokerClient(s.URL())

	_, err := c.MergeEntity(context.Background(), "id", testEntity("Road", "id"), nil)

	is.NoErr(err)
	is.Equal(s.RequestCount(), 1)
}

func TestUpdateEntityAttributesWithMetaData(t *testing.T) {
	is := is.New(t)

	s := setupMockServiceThat(
		expects(
			is,
			requestMethod(http.MethodPatch),
			requestPath("/ngsi-ld/v1/entities/id/attrs/"),
			requestBody(
				"{\"@context\":[\"https://raw.githubusercontent.com/diwise/context-broker/main/assets/jsonldcontexts/default-context.jsonld\"],\"waterConsumption\":{\"type\":\"Property\",\"value\":100,\"observedAt\":\"2006-01-02T15:04:05Z\",\"observedBy\":{\"type\":\"Relationship\",\"object\":\"some_device\"},\"unitCode\":\"LTR\"}}",
			),
		),
		returns(
			responseCode(http.StatusNoContent),
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

func setupMockServiceThat(expects func(r *http.Request), returns func(w http.ResponseWriter)) MockService {

	mock := &mockSvc{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock.requestCount++
		expects(r)
		returns(w)
	}))

	mock.server = srv

	return mock
}

type MockService interface {
	Close()
	RequestCount() int
	URL() string
}

type mockSvc struct {
	requestCount int
	server       *httptest.Server
	closed       bool
}

func (m *mockSvc) Close() {
	if !m.closed {
		m.server.Close()
	}
	m.closed = true
}

func (m *mockSvc) RequestCount() int {
	return m.requestCount
}

func (m *mockSvc) URL() string {
	return m.server.URL
}

func testEntity(entityType, entityID string) types.Entity {
	e, _ := entities.New(entityID, entityType)
	return e
}

func expects(is *is.I, facts ...func(*is.I, *http.Request)) func(r *http.Request) {
	return func(r *http.Request) {
		for _, checkFact := range facts {
			checkFact(is, r)
		}
	}
}

func anyInput() func(*is.I, *http.Request) {
	return func(*is.I, *http.Request) {}
}

func requestBody(body string) func(*is.I, *http.Request) {
	return func(is *is.I, r *http.Request) {
		reqBytes, err := io.ReadAll(r.Body)
		is.NoErr(err)

		reqString := string(reqBytes)
		is.Equal(reqString, body)
	}
}

func requestMethod(method string) func(*is.I, *http.Request) {
	return func(is *is.I, r *http.Request) {
		is.Equal(r.Method, method)
	}
}

func requestPath(path string) func(*is.I, *http.Request) {
	return func(is *is.I, r *http.Request) {
		is.Equal(r.URL.Path, path)
	}
}

func returns(writers ...func(w http.ResponseWriter)) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		for _, writeResult := range writers {
			writeResult(w)
		}
	}
}

func contenttype(contentType string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Add("Content-Type", contentType)
	}
}

func location(loc string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Add("Location", loc)
	}
}

func responseBody(body []byte) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Write(body)
	}
}

func responseCode(code int) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.WriteHeader(code)
	}
}
