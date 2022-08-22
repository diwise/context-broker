package geojson

import (
	"fmt"

	"github.com/diwise/context-broker/pkg/ngsild/types"
)

type GeoJSONFeatureCollection struct {
	Type     string           `json:"type"`
	Features []GeoJSONFeature `json:"features"`
	Context  *[]string        `json:"@context,omitempty"`
}

func NewFeatureCollection() *GeoJSONFeatureCollection {
	fc := &GeoJSONFeatureCollection{Type: "FeatureCollection"}
	fc.Context = &[]string{
		"https://schema.lab.fiware.org/ld/context",
		"https://uri.etsi.org/ngsi-ld/v1/ngsi-ld-core-context.jsonld",
	}

	return fc
}

type GeoJSONFeature struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Geometry   any            `json:"geometry"`
	Properties map[string]any `json:"properties"`
}

func ConvertEntity(e types.Entity) (*GeoJSONFeature, error) {
	feature := &GeoJSONFeature{
		ID:   e.ID(),
		Type: "Feature",
		Properties: map[string]any{
			"type": e.Type(),
		},
	}

	e.ForEachAttribute(func(attributeType, attributeName string, contents any) {
		feature.Properties[attributeName] = contents

		if attributeType == "GeoProperty" && attributeName == "location" {
			geoprop, ok := contents.(GeoJSONGeometry)
			if ok {
				feature.Geometry = geoprop.GeoPropertyValue()
			}
		}
	})

	return feature, nil
}

type GeoJSONGeometry interface {
	GeoPropertyType() string
	GeoPropertyValue() GeoJSONGeometry
	GetAsPoint() GeoJSONPropertyPoint
}

type PropertyImpl struct {
	Type string `json:"type"`
}

// GeoJSONProperty is used to encapsulate different GeoJSONGeometry types
type GeoJSONProperty struct {
	PropertyImpl
	Val GeoJSONGeometry `json:"value"`
}

func (gjp *GeoJSONProperty) GeoPropertyType() string {
	return gjp.Val.GeoPropertyType()
}

func (gjp *GeoJSONProperty) GeoPropertyValue() GeoJSONGeometry {
	return gjp.Val
}

func (gjp *GeoJSONProperty) GetAsPoint() GeoJSONPropertyPoint {
	return gjp.Val.GetAsPoint()
}

func (gjp *GeoJSONProperty) Type() string {
	return gjp.PropertyImpl.Type
}

func (gjp *GeoJSONProperty) Value() any {
	return gjp.GeoPropertyValue()
}

// GeoJSONPropertyPoint is used as the value object for a GeoJSONPropertyPoint
type GeoJSONPropertyPoint struct {
	Type        string     `json:"type"`
	Coordinates [2]float64 `json:"coordinates"`
}

func (gjpp *GeoJSONPropertyPoint) GeoPropertyType() string {
	return gjpp.Type
}

func (gjpp *GeoJSONPropertyPoint) GeoPropertyValue() GeoJSONGeometry {
	return gjpp
}

func (gjpp *GeoJSONPropertyPoint) GetAsPoint() GeoJSONPropertyPoint {
	// Return a copy of this point to prevent mutation
	return GeoJSONPropertyPoint{
		Type:        gjpp.Type,
		Coordinates: [2]float64{gjpp.Coordinates[0], gjpp.Coordinates[1]},
	}
}

func (gjpp GeoJSONPropertyPoint) Latitude() float64 {
	return gjpp.Coordinates[1]
}

func (gjpp GeoJSONPropertyPoint) Longitude() float64 {
	return gjpp.Coordinates[0]
}

// GeoJSONPropertyLineString is used as the value object for a GeoJSONPropertyLineString
type GeoJSONPropertyLineString struct {
	Type        string      `json:"type"`
	Coordinates [][]float64 `json:"coordinates"`
}

func (gjpls *GeoJSONPropertyLineString) GeoPropertyType() string {
	return gjpls.Type
}

func (gjpls *GeoJSONPropertyLineString) GeoPropertyValue() GeoJSONGeometry {
	return gjpls
}

func (gjpls *GeoJSONPropertyLineString) GetAsPoint() GeoJSONPropertyPoint {
	return GeoJSONPropertyPoint{
		Type:        "Point",
		Coordinates: [2]float64{gjpls.Coordinates[0][0], gjpls.Coordinates[0][1]},
	}
}

// GeoJSONPropertyMultiPolygon is used as the value object for a GeoJSONPropertyMultiPolygon
type GeoJSONPropertyMultiPolygon struct {
	Type        string          `json:"type"`
	Coordinates [][][][]float64 `json:"coordinates"`
}

func (gjpmp *GeoJSONPropertyMultiPolygon) GeoPropertyType() string {
	return gjpmp.Type
}

func (gjpmp *GeoJSONPropertyMultiPolygon) GeoPropertyValue() GeoJSONGeometry {
	return gjpmp
}

func (gjpmp *GeoJSONPropertyMultiPolygon) GetAsPoint() GeoJSONPropertyPoint {
	return GeoJSONPropertyPoint{
		Type:        "Point",
		Coordinates: [2]float64{gjpmp.Coordinates[0][0][0][0], gjpmp.Coordinates[0][0][0][1]},
	}
}

// CreateGeoJSONPropertyFromWGS84 creates a GeoJSONProperty from a WGS84 coordinate
func CreateGeoJSONPropertyFromWGS84(longitude, latitude float64) *GeoJSONProperty {
	p := &GeoJSONProperty{
		PropertyImpl: PropertyImpl{Type: "GeoProperty"},
		Val: &GeoJSONPropertyPoint{
			Type:        "Point",
			Coordinates: [2]float64{longitude, latitude},
		},
	}

	return p
}

// CreateGeoJSONPropertyFromLineString creates a GeoJSONProperty from an array of line coordinate arrays
func CreateGeoJSONPropertyFromLineString(coordinates [][]float64) *GeoJSONProperty {
	p := &GeoJSONProperty{
		PropertyImpl: PropertyImpl{Type: "GeoProperty"},
		Val: &GeoJSONPropertyLineString{
			Type:        "LineString",
			Coordinates: coordinates,
		},
	}

	return p
}

// CreateGeoJSONPropertyFromMultiPolygon creates a GeoJSONProperty from an array of polygon coordinate arrays
func CreateGeoJSONPropertyFromMultiPolygon(coordinates [][][][]float64) *GeoJSONProperty {
	p := &GeoJSONProperty{
		PropertyImpl: PropertyImpl{Type: "GeoProperty"},
		Val: &GeoJSONPropertyMultiPolygon{
			Type:        "MultiPolygon",
			Coordinates: coordinates,
		},
	}

	return p
}

func UnmarshalG(body map[string]any) (types.Property, error) {
	value, ok := body["value"]
	if !ok {
		return nil, fmt.Errorf("geoproperties without a value attribute are not supported")
	}

	switch typedValue := value.(type) {
	case map[string]any:
		geoType, ok := typedValue["type"]
		if !ok {
			return nil, fmt.Errorf("geoproperties without a geotype is not supported")
		}

		geoTypeStr, ok := geoType.(string)
		if !ok {
			return nil, fmt.Errorf("geoproperty type value is of an unconvertible type")
		}

		untypedCoordinates, ok := typedValue["coordinates"]
		if !ok {
			return nil, fmt.Errorf("unable to unmarshal geoproperty point with no coordinates")
		}

		switch geoTypeStr {
		case "Point":
			coordinates := untypedCoordinates.([]any)
			if len(coordinates) < 2 {
				return nil, fmt.Errorf("geoproperty point coordinates array has insufficient length (%d < 2)", len(coordinates))
			}

			lon, ok_lon := coordinates[0].(float64)
			lat, ok_lat := coordinates[1].(float64)

			if !ok_lon || !ok_lat {
				return nil, fmt.Errorf("geoproperty point coordinates not convertible to float64")
			}

			return CreateGeoJSONPropertyFromWGS84(lon, lat), nil
		case "LineString":
			return unmarshalLineString(typedValue)
		case "MultiPolygon":
			return unmarshalMultiPolygon(typedValue)
		default:
			return nil, fmt.Errorf("unknown geotype %s not supported in geoproperty", geoTypeStr)
		}

	default:
		return nil, fmt.Errorf("unable to parse geoproperty of unknown value type %T", typedValue)
	}
}

func unmarshalLineString(value map[string]any) (types.Property, error) {
	untypedCoordinates, ok := value["coordinates"]
	if !ok {
		return nil, fmt.Errorf("unable to unmarshal geoproperty point with no coordinates")
	}

	coordinates := untypedCoordinates.([]any)
	coords := make([][]float64, 0, len(coordinates))

	for _, a := range coordinates {
		a, ok := a.([]any)
		if !ok {
			return nil, fmt.Errorf("malformed linestring coordinates")
		}

		c1 := make([]float64, 0, len(a))

		for _, p := range a {
			v, ok := p.(float64)
			if !ok {
				return nil, fmt.Errorf("failed to convert line string coordinate to float64")
			}

			c1 = append(c1, v)
		}

		coords = append(coords, c1)
	}

	return CreateGeoJSONPropertyFromLineString(coords), nil
}

func unmarshalMultiPolygon(value map[string]any) (types.Property, error) {
	untypedCoordinates, ok := value["coordinates"]
	if !ok {
		return nil, fmt.Errorf("unable to unmarshal geoproperty point with no coordinates")
	}

	coordinates := untypedCoordinates.([]any)
	coords := make([][][][]float64, 0, len(coordinates))

	for _, a := range coordinates {
		a, ok := a.([]any)
		if !ok {
			return nil, fmt.Errorf("malformed multipolygon coordinates")
		}

		c1 := make([][][]float64, 0, len(a))

		for _, b := range a {
			b, ok := b.([]any)
			if !ok {
				return nil, fmt.Errorf("malformed multipolygon coordinates")
			}

			c2 := make([][]float64, 0, len(b))

			for _, c := range b {
				c, ok := c.([]any)
				if !ok {
					return nil, fmt.Errorf("malformed multipolygon coordinates")
				}

				c3 := make([]float64, 0, len(c))

				for _, p := range c {
					v, ok := p.(float64)
					if !ok {
						return nil, fmt.Errorf("failed to convert multi polygon coordinate to float64")
					}

					c3 = append(c3, v)
				}

				c2 = append(c2, c3)
			}

			c1 = append(c1, c2)
		}
		coords = append(coords, c1)
	}

	return CreateGeoJSONPropertyFromMultiPolygon(coords), nil
}
