package decorators

import (
	"github.com/diwise/context-broker/pkg/ngsild/geojson"
	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	"github.com/diwise/context-broker/pkg/ngsild/types/properties"
	"github.com/diwise/context-broker/pkg/ngsild/types/relationships"
)

func RefDevice(device string) entities.EntityDecoratorFunc {
	return entities.R("refDevice", relationships.NewSingleObjectRelationship(device))
}

func Location(latitude, longitude float64) entities.EntityDecoratorFunc {
	location := geojson.CreateGeoJSONPropertyFromWGS84(longitude, latitude)
	return entities.P("location", location)
}

func DateTime(name string, value string) entities.EntityDecoratorFunc {
	return entities.P("dateObserved", properties.NewDateTimeProperty(value))
}

func Number(name string, value float64, decorators ...properties.NumberPropertyDecoratorFunc) entities.EntityDecoratorFunc {
	np := properties.NewNumberProperty(value)
	for _, decorator := range decorators {
		decorator(np)
	}
	return entities.P(name, np)
}

func Text(name string, value string) entities.EntityDecoratorFunc {
	return entities.P(name, properties.NewTextProperty(value))
}

func DateLastValueReported(timestamp string) entities.EntityDecoratorFunc {
	return DateTime("dateLastValueReported", timestamp)
}

func DateObserved(timestamp string) entities.EntityDecoratorFunc {
	return DateTime("dateObserved", timestamp)
}

func Status(value string) entities.EntityDecoratorFunc {
	return Text("status", value)
}

func Temperature(t float64) entities.EntityDecoratorFunc {
	return Number("temperature", t)
}
