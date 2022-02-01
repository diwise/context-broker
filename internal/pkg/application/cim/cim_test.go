package cim

import (
	"testing"

	"github.com/matryer/is"
)

func TestMe(t *testing.T) {
	is := is.New(t)

	var jsonData string = `{
		"id": "urn:ngsi-ld:Consumer:Consumer01",
		"type": "WaterConsumptionObserved",
		"waterConsumption": {
			"type": "Property",
			"value": 191051,
			"observedAt": "2021-05-23T23:14:16.000Z",
			"unitCode": "LTR"
		},
		"refDevice": {
			"type": "Relationship",
			"object": "urn:ngsi-ld:Device:01"
		}
	}`

	e := &EntityImpl{contents: []byte(jsonData)}
	attributes := []string{}
	e.ForEachAttribute(func(at, an string, data interface{}) {
		attributes = append(attributes, an)
	})

	is.Equal(e.ID(), "urn:ngsi-ld:Consumer:Consumer01")
	is.Equal(e.Type(), "WaterConsumptionObserved")
	is.Equal(len(attributes), 2) // should find two attributes
}
