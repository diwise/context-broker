package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/matryer/is"
)

func TestCreateEntity(t *testing.T) {
	is := is.New(t)

	s := setupMockServiceThatReturns(http.StatusCreated, "")
	c := NewContextBrokerClient(s.URL)

	result, err := c.CreateEntity(context.Background(), testEntity("Road", "id"), nil)

	is.NoErr(err)
	is.Equal(result.Location(), "/ngsi-ld/v1/entities/id")
}

func setupMockServiceThatReturns(responseCode int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(responseCode)
		w.Header().Add("Content-Type", "application/ld+json")
		if body != "" {
			w.Write([]byte(body))
		}
	}))
}

func testEntity(entityType, entityID string) types.Entity {
	var entityJSON string = `{
		"id": "%s",
		"type": "%s",
		"@context": [
			"https://schema.lab.fiware.org/ld/context",
			"https://uri.etsi.org/ngsi-ld/v1/ngsi-ld-core-context.jsonld"
		]
	}`

	json := fmt.Sprintf(entityJSON, entityID, entityType)
	e, _ := types.NewEntity([]byte(json))
	return e
}
