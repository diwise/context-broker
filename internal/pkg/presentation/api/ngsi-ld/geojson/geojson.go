package geojson

import "github.com/diwise/context-broker/pkg/ngsild/types"

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
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Geometry   interface{}            `json:"geometry"`
	Properties map[string]interface{} `json:"properties"`
}

func ConvertEntity(e types.Entity) (*GeoJSONFeature, error) {
	feature := &GeoJSONFeature{
		ID:   e.ID(),
		Type: "Feature",
		Properties: map[string]interface{}{
			"type": e.Type(),
		},
	}

	e.ForEachAttribute(func(attributeType, attributeName string, contents interface{}) {
		feature.Properties[attributeName] = contents

		if attributeType == "GeoProperty" && attributeName == "location" {
			geoprop, ok := contents.(map[string]interface{})
			if ok {
				geopropval, ok := geoprop["value"]
				if ok {
					feature.Geometry = geopropval
				}
			}
		}
	})

	return feature, nil
}
