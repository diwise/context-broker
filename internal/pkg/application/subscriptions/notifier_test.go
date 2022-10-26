package subscriptions

import (
	"context"
	"net/http"
	"testing"

	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	. "github.com/diwise/context-broker/pkg/ngsild/types/entities/decorators"
	testutils "github.com/diwise/service-chassis/pkg/test/http"
	"github.com/diwise/service-chassis/pkg/test/http/expects"
	"github.com/diwise/service-chassis/pkg/test/http/response"
	"github.com/matryer/is"
)

var Expects = testutils.Expects
var Returns = testutils.Returns

var method = expects.RequestMethod
var bodyContaining = expects.RequestBodyContaining

func TestSingleNotificationOnCreate(t *testing.T) {
	is := is.New(t)
	const entityID string = "urn:ngsi-ld:Lifebuoy:mybuoy"

	s := testutils.NewMockServiceThat(
		Expects(
			is,
			method(http.MethodPost),
			bodyContaining("urn:ngsi-ld:Lifebuoy:mybuoy"),
		),
		Returns(
			response.Code(http.StatusOK),
		),
	)
	defer s.Close()

	ctx := context.Background()
	n, _ := NewNotifier(ctx, s.URL())

	n.Start()

	e, err := entities.New(entityID, "Lifebuoy", Status("off"))
	is.NoErr(err)

	n.EntityCreated(ctx, e)

	n.Stop()

	is.Equal(s.RequestCount(), 1)
}
