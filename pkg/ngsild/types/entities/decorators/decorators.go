package decorators

import (
	"time"

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
	return entities.P(properties.Location, location)
}

func LocationLS(linestring [][]float64) entities.EntityDecoratorFunc {
	location := geojson.CreateGeoJSONPropertyFromLineString(linestring)
	return entities.P(properties.Location, location)
}

func LocationMP(multipolygon [][][][]float64) entities.EntityDecoratorFunc {
	location := geojson.CreateGeoJSONPropertyFromMultiPolygon(multipolygon)
	return entities.P(properties.Location, location)
}

func DateTime(name string, value string) entities.EntityDecoratorFunc {
	return entities.P(name, properties.NewDateTimeProperty(value))
}

func DateTimeIfNotZero(name string, dt time.Time) entities.EntityDecoratorFunc {
	if dt.IsZero() {
		return NoOp()
	}

	return DateTime(name, dt.Format(time.RFC3339))
}

func Description(desc string) entities.EntityDecoratorFunc {
	return Text(properties.Description, desc)
}

func Name(name string) entities.EntityDecoratorFunc {
	return Text(properties.Name, name)
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
	return DateTime(properties.DateCreated, timestamp)
}

func DateLastValueReported(timestamp string) entities.EntityDecoratorFunc {
	return DateTime("dateLastValueReported", timestamp)
}

func DateModified(timestamp string) entities.EntityDecoratorFunc {
	return DateTime(properties.DateModified, timestamp)
}

func DateObserved(timestamp string) entities.EntityDecoratorFunc {
	return DateTime(properties.DateObserved, timestamp)
}

func Status(value string, decorators ...properties.TextPropertyDecoratorFunc) entities.EntityDecoratorFunc {
	nt := properties.NewTextProperty(value)
	for _, decorator := range decorators {
		decorator(nt)
	}
	return entities.P("status", nt)
}

func Temperature(t float64) entities.EntityDecoratorFunc {
	return Number("temperature", t)
}

func NoOp() entities.EntityDecoratorFunc {
	return func(e *entities.EntityImpl) {}
}
