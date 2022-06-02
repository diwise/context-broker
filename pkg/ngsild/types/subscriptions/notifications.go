package subscriptions

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	"github.com/google/uuid"
)

type Notification struct {
	Id             string         `json:"id"`
	Type           string         `json:"type"`
	SubscriptionId string         `json:"subscriptionId"`
	NotifiedAt     string         `json:"notifiedAt"`
	Data           []types.Entity `json:"data"`
}

func (n *Notification) UnmarshalJSON(data []byte) error {
	base := struct {
		Id             string          `json:"id"`
		Type           string          `json:"type"`
		SubscriptionId string          `json:"subscriptionId"`
		NotifiedAt     string          `json:"notifiedAt"`
		Data           json.RawMessage `json:"data"`
	}{}

	err := json.Unmarshal(data, &base)
	if err != nil {
		return err
	}

	n.Id = base.Id
	n.Type = base.Type
	n.SubscriptionId = base.SubscriptionId
	n.NotifiedAt = base.NotifiedAt
	n.Data, err = entities.NewFromSlice(base.Data)

	return err
}

func NewNotification(e types.Entity) *Notification {
	n := &Notification{
		Id:             fmt.Sprintf("urn:ngsi-ld:Notification:%s", uuid.New().String()),
		Type:           "Notification",
		SubscriptionId: "notimplemented",
		NotifiedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		Data:           []types.Entity{e},
	}

	return n
}
