package cim

import (
	"testing"

	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	"github.com/diwise/context-broker/pkg/ngsild/types/properties"
	"github.com/matryer/is"
)

func TestMe(t *testing.T) {
	is := is.New(t)

	e, err := entities.New(
		"urn:ngsi-ld:Consumer:Consumer01", "WaterConsumptionObserved",
		entities.RefDevice("urn:ngsi-ld:Device:01"),
		entities.Number("waterConsumption", 191051, properties.UnitCode("LTR"), properties.ObservedAt("2021-05-23T23:14:16.000Z")),
	)

	is.NoErr(err)

	attributes := []string{}
	e.ForEachAttribute(func(at, an string, data interface{}) {
		attributes = append(attributes, an)
	})

	is.Equal(e.ID(), "urn:ngsi-ld:Consumer:Consumer01")
	is.Equal(e.Type(), "WaterConsumptionObserved")
	is.Equal(len(attributes), 2) // should find two attributes
}
