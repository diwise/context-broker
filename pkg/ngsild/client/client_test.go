package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	ngsierrors "github.com/diwise/context-broker/pkg/ngsild/errors"
	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	"github.com/matryer/is"
)

func TestCreateEntity(t *testing.T) {
	is := is.New(t)

	locationHeader := "/ngsi-ld/v1/entities/id"
	s := setupMockServiceThatReturns(http.StatusCreated, "", contenttype("application/ld+json"), location(locationHeader))
	c := NewContextBrokerClient(s.URL)

	result, err := c.CreateEntity(context.Background(), testEntity("Road", "id"), nil)

	is.NoErr(err)
	is.Equal(result.Location(), locationHeader)
}

func TestCreateEntityHandlesMissingLocationheader(t *testing.T) {
	is := is.New(t)

	s := setupMockServiceThatReturns(http.StatusCreated, "", contenttype("application/ld+json"))
	c := NewContextBrokerClient(s.URL)

	result, err := c.CreateEntity(context.Background(), testEntity("Road", "id"), nil)

	is.NoErr(err)
	is.Equal(result.Location(), "/ngsi-ld/v1/entities/id")
}

func TestCreateEntityThrowsErrorOnNon201Success(t *testing.T) {
	is := is.New(t)

	s := setupMockServiceThatReturns(http.StatusNoContent, "")
	c := NewContextBrokerClient(s.URL)

	_, err := c.CreateEntity(context.Background(), testEntity("Road", "id"), nil)

	is.True(err != nil)
	is.Equal(err.Error(), "unexpected response code 204 (internal error)")
}

func TestCreateEntityHandlesBadRequestError(t *testing.T) {
	is := is.New(t)

	pr := ngsierrors.NewBadRequestData("bad request", "traceID")
	b, _ := json.Marshal(pr)
	s := setupMockServiceThatReturns(http.StatusBadRequest, string(b), contenttype("application/problem+json"))
	c := NewContextBrokerClient(s.URL)

	_, err := c.CreateEntity(context.Background(), testEntity("A", "id"), nil)

	is.True(err != nil)
	is.True(errors.Is(err, ngsierrors.ErrBadRequest))
}

func TestMergeEntity(t *testing.T) {
	is := is.New(t)

	s := setupMockServiceThatReturns(http.StatusNoContent, "")
	c := NewContextBrokerClient(s.URL)

	_, err := c.MergeEntity(context.Background(), "id", testEntity("Road", "id"), nil)

	is.NoErr(err)
}

func setupMockServiceThatReturns(responseCode int, body string, headers ...func(w http.ResponseWriter)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, applyHeaderTo := range headers {
			applyHeaderTo(w)
		}

		w.WriteHeader(responseCode)

		if body != "" {
			w.Write([]byte(body))
		}
	}))
}

func testEntity(entityType, entityID string) types.Entity {
	e, _ := entities.New(entityID, entityType)
	return e
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
