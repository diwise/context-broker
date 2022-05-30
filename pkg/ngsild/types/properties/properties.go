package properties

import (
	"fmt"
	"strconv"

	"github.com/diwise/context-broker/pkg/ngsild/types"
)

//NumberProperty holds a float64 Value
type NumberProperty struct {
	PropertyImpl
	Value      float64 `json:"value"`
	ObservedAt *string `json:"observedAt,omitempty"`
	UnitCode   *string `json:"unitCode,omitempty"`
}

func (np *NumberProperty) Type() string {
	return np.PropertyImpl.Type
}

//NewNumberProperty is a convenience function for creating NumberProperty instances
func NewNumberProperty(value float64) *NumberProperty {
	return &NumberProperty{
		PropertyImpl: PropertyImpl{Type: "Property"},
		Value:        value,
	}
}

//Property contains the mandatory Type property
type PropertyImpl struct {
	Type string `json:"type"`
}

//DateTimeProperty stores date and time values (surprise, surprise ...)
type DateTimeProperty struct {
	PropertyImpl
	Value struct {
		Type  string `json:"@type"`
		Value string `json:"@value"`
	} `json:"value"`
}

type NumberPropertyDecoratorFunc func(np *NumberProperty)

func ObservedAt(timestamp string) NumberPropertyDecoratorFunc {
	return func(np *NumberProperty) {
		np.ObservedAt = &timestamp
	}
}

func UnitCode(code string) NumberPropertyDecoratorFunc {
	return func(np *NumberProperty) {
		np.UnitCode = &code
	}
}

//NewDateTimeProperty creates a property from a UTC time stamp
func NewDateTimeProperty(value string) *DateTimeProperty {
	dtp := &DateTimeProperty{
		PropertyImpl: PropertyImpl{Type: "Property"},
	}

	dtp.Value.Type = "DateTime"
	dtp.Value.Value = value

	return dtp
}

func (dtp *DateTimeProperty) Type() string {
	return dtp.PropertyImpl.Type
}

//TextProperty stores values of type text
type TextProperty struct {
	PropertyImpl
	Value string `json:"value"`
}

func (tp *TextProperty) Type() string {
	return tp.PropertyImpl.Type
}

//TextListProperty stores values of type text list
type TextListProperty struct {
	PropertyImpl
	Value []string `json:"value"`
}

func (tlp *TextListProperty) Type() string {
	return tlp.PropertyImpl.Type
}

//NewTextListProperty accepts a value as a string array and returns a new TextListProperty
func NewTextListProperty(value []string) *TextListProperty {
	return &TextListProperty{
		PropertyImpl: PropertyImpl{Type: "Property"},
		Value:        value,
	}
}

//NewNumberPropertyFromString accepts a value as a string and returns a new NumberProperty
func NewNumberPropertyFromString(value string) *NumberProperty {
	number, _ := strconv.ParseFloat(value, 64)
	return NewNumberProperty(number)
}

//NewTextProperty accepts a value as a string and returns a new TextProperty
func NewTextProperty(value string) *TextProperty {
	return &TextProperty{
		PropertyImpl: PropertyImpl{Type: "Property"},
		Value:        value,
	}
}

func UnmarshalP(body map[string]any) (types.Property, error) {
	value, ok := body["value"]
	if !ok {
		return nil, fmt.Errorf("properties without a value attribute are not supported")
	}

	switch typedValue := value.(type) {
	case float64:
		return NewNumberProperty(typedValue), nil
	case string:
		return NewTextProperty(typedValue), nil
	case map[string]any:
		return unmarshalPropertyObject(typedValue)
	case []any:
		values := []string{}
		for _, v := range typedValue {
			str, ok := v.(string)
			if ok {
				values = append(values, str)
			}
		}
		return NewTextListProperty(values), nil
	default:
		return NewTextProperty(fmt.Sprintf("support for type %T not implemented", typedValue)), nil
	}
}

func unmarshalPropertyObject(object map[string]any) (types.Property, error) {
	objectType, ok := object["@type"]
	if !ok {
		return nil, fmt.Errorf("property objects without a @type attribute are not supported")
	}

	objectValue, ok := object["@value"]
	if !ok {
		return nil, fmt.Errorf("property objects without a @value attribute are not supported")
	}

	objectTypeStr, ok := objectType.(string)
	if !ok {
		return nil, fmt.Errorf("property object @type not convertible to string")
	}

	switch objectTypeStr {
	case "DateTime":
		dateTimeStr, ok := objectValue.(string)
		if !ok {
			return nil, fmt.Errorf("datetime property @value not convertible to string")
		}
		return NewDateTimeProperty(dateTimeStr), nil
	default:
		return nil, fmt.Errorf("property object of type %s not supported", objectTypeStr)
	}
}
