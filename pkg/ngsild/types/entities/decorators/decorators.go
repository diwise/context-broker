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

func LocationLS(linestring [][]float64) entities.EntityDecoratorFunc {
	location := geojson.CreateGeoJSONPropertyFromLineString(linestring)
	return entities.P("location", location)
}

func LocationMP(multipolygon [][][][]float64) entities.EntityDecoratorFunc {
	location := geojson.CreateGeoJSONPropertyFromMultiPolygon(multipolygon)
	return entities.P("location", location)
}

func DateTime(name string, value string) entities.EntityDecoratorFunc {
	return entities.P(name, properties.NewDateTimeProperty(value))
}

func Description(desc string) entities.EntityDecoratorFunc {
	return Text("description", desc)
}

func Name(name string) entities.EntityDecoratorFunc {
	return Text("name", name)
}

func Number(name string, value float64, decorators ...properties.NumberPropertyDecoratorFunc) entities.EntityDecoratorFunc {
	np := properties.NewNumberProperty(value)
	for _, decorator := range decorators {
		decorator(np)
	}
	return entities.P(name, np)
}

func RefSeeAlso(refs []string) entities.EntityDecoratorFunc {
	return TextList("refSeeAlso", refs)
}

func Source(src string) entities.EntityDecoratorFunc {
	return Text("source", src)
}

func Text(name string, value string) entities.EntityDecoratorFunc {
	return entities.P(name, properties.NewTextProperty(value))
}

func TextList(name string, value []string) entities.EntityDecoratorFunc {
	return entities.P(name, properties.NewTextListProperty(value))
}

func DateCreated(timestamp string) entities.EntityDecoratorFunc {
	return DateTime("dateCreated", timestamp)
}

func DateLastValueReported(timestamp string) entities.EntityDecoratorFunc {
	return DateTime("dateLastValueReported", timestamp)
}

func DateModified(timestamp string) entities.EntityDecoratorFunc {
	return DateTime("dateModified", timestamp)
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
