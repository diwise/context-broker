package geojson

import (
	"encoding/json"
	"testing"

	"github.com/matryer/is"
)

func TestGeoJSON(t *testing.T) {
	is := is.New(t)
	is.Equal(1, 1)
}

func TestPolygonRoundTrip(t *testing.T) {
	is := is.New(t)
	body := map[string]any{
		"value": map[string]any{
			"type": "Polygon",
			"coordinates": []any{
				[]any{
					[]any{17.316427, 62.404673},
					[]any{17.3172, 62.404912},
					[]any{17.318659, 62.403718},
					[]any{17.317801, 62.403598},
					[]any{17.316427, 62.404673},
				},
			},
		},
	}
	prop, err := UnmarshalG(body)
	is.NoErr(err)
	gjp, ok := prop.(*GeoJSONProperty)
	is.True(ok)
	is.Equal(gjp.GeoPropertyType(), "Polygon")
	raw, err := json.Marshal(gjp.Value())
	is.NoErr(err)
	is.True(len(raw) > 0)
	pt := gjp.GetAsPoint()
	is.Equal(pt.Type, "Point")
}
