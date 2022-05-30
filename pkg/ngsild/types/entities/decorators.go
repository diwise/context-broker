package entities

import (
	"github.com/diwise/context-broker/pkg/ngsild/geojson"
	"github.com/diwise/context-broker/pkg/ngsild/types/properties"
	"github.com/diwise/context-broker/pkg/ngsild/types/relationships"
)

func RefDevice(device string) EntityDecoratorFunc {
	return R("refDevice", relationships.NewSingleObjectRelationship(device))
}

func Location(latitude, longitude float64) EntityDecoratorFunc {
	location := geojson.CreateGeoJSONPropertyFromWGS84(longitude, latitude)
	return P("location", location)
}

func DateTime(name string, value string) EntityDecoratorFunc {
	return P("dateObserved", properties.NewDateTimeProperty(value))
}

func Number(name string, value float64, decorators ...properties.NumberPropertyDecoratorFunc) EntityDecoratorFunc {
	np := properties.NewNumberProperty(value)
	for _, decorator := range decorators {
		decorator(np)
	}
	return P(name, np)
}

func Text(name string, value string) EntityDecoratorFunc {
	return P(name, properties.NewTextProperty(value))
}

func DateLastValueReported(timestamp string) EntityDecoratorFunc {
	return DateTime("dateLastValueReported", timestamp)
}

func DateObserved(timestamp string) EntityDecoratorFunc {
	return DateTime("dateObserved", timestamp)
}

func Status(value string) EntityDecoratorFunc {
	return Text("status", value)
}

func Temperature(t float64) EntityDecoratorFunc {
	return Number("temperature", t)
}
