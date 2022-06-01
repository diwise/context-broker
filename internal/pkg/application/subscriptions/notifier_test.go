package subscriptions

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	. "github.com/diwise/context-broker/pkg/ngsild/types/entities/decorators"
	"github.com/diwise/context-broker/pkg/ngsild/types/subscriptions"
	"github.com/matryer/is"
)

func TestSingleNotificationOnCreate(t *testing.T) {
	is := is.New(t)
	const entityID string = "entityid"

	var calls int = 0
	s := setupMockService(&calls,
		notificationEntityCount(is, 1), notificationEntityID(0, is, entityID),
		responseCode(http.StatusOK),
	)

	ctx := context.Background()
	n, _ := NewNotifier(ctx, s.URL)

	n.Start()

	e, err := entities.New(entityID, "EntityType", entities.DefaultContext(), Status("off"))
	is.NoErr(err)

	n.EntityCreated(ctx, e)

	n.Stop()

	is.Equal(calls, 1)
}

type ValidatorFunc func(w http.ResponseWriter, r *http.Request, body []byte)

func setupMockService(callCounter *int, validators ...ValidatorFunc) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*callCounter++
		b, _ := ioutil.ReadAll(r.Body)

		for _, f := range validators {
			f(w, r, b)
		}
	}))
}

func notificationEntityCount(is *is.I, count int) ValidatorFunc {
	return func(w http.ResponseWriter, r *http.Request, b []byte) {
		n := subscriptions.Notification{}
		json.Unmarshal(b, &n)

		is.Equal(count, len(n.Data)) // entity count should match
	}
}

func notificationEntityID(idx int, is *is.I, entityID string) ValidatorFunc {
	return func(w http.ResponseWriter, r *http.Request, b []byte) {
		n := subscriptions.Notification{}
		json.Unmarshal(b, &n)

		is.Equal(entityID, n.Data[idx].ID()) // entity id should match
	}
}

/*func requestBody(is *is.I, body string) ValidatorFunc {
	return func(w http.ResponseWriter, r *http.Request, b []byte) {
		is.Equal(string(b), body) // request body should match
	}
}*/

func responseCode(response int) ValidatorFunc {
	return func(w http.ResponseWriter, r *http.Request, b []byte) {
		w.WriteHeader(response)
	}
}
