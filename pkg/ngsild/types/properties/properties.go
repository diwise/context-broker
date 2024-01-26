package properties

import (
	"fmt"
	"strconv"

	"github.com/diwise/context-broker/pkg/ngsild/types"
	"github.com/diwise/context-broker/pkg/ngsild/types/relationships"
)

const (
	DateCreated  string = "dateCreated"
	DateModified string = "dateModified"
	DateObserved string = "dateObserved"

	Description string = "description"
	Location    string = "location"
	Name        string = "name"
)

// NumberProperty holds a float64 Value
type NumberProperty struct {
	PropertyImpl
	Val         float64            `json:"value"`
	ObservedAt_ *string            `json:"observedAt,omitempty"`
	ObservedBy  types.Relationship `json:"observedBy,omitempty"`
	UnitCode    *string            `json:"unitCode,omitempty"`
}

func (np *NumberProperty) Type() string {
	return np.PropertyImpl.Type
}

func (np *NumberProperty) Value() any {
	return np.Val
}

func (np *NumberProperty) ObservedAt() string {
	if np.ObservedAt_ != nil {
		return *np.ObservedAt_
	}
	return ""
}

// NewNumberProperty is a convenience function for creating NumberProperty instances
func NewNumberProperty(value float64) *NumberProperty {
	return &NumberProperty{
		PropertyImpl: PropertyImpl{Type: "Property"},
		Val:          value,
	}
}

// Property contains the mandatory Type property
type PropertyImpl struct {
	Type string `json:"type"`
}

// DateTimeProperty stores date and time values (surprise, surprise ...)
type DateTimeProperty struct {
	PropertyImpl
	Val struct {
		Type  string `json:"@type"`
		Value string `json:"@value"`
	} `json:"value"`
}

type NumberPropertyDecoratorFunc func(np *NumberProperty)

func ObservedAt(timestamp string) NumberPropertyDecoratorFunc {
	return func(np *NumberProperty) {
		np.ObservedAt_ = &timestamp
	}
}

func ObservedBy(object string) NumberPropertyDecoratorFunc {
	return func(np *NumberProperty) {
		np.ObservedBy = relationships.NewSingleObjectRelationship(object)
	}
}

func UnitCode(code string) NumberPropertyDecoratorFunc {
	return func(np *NumberProperty) {
		np.UnitCode = &code
	}
}

// NewDateTimeProperty creates a property from a UTC time stamp
func NewDateTimeProperty(value string) *DateTimeProperty {
	dtp := &DateTimeProperty{
		PropertyImpl: PropertyImpl{Type: "Property"},
	}

	dtp.Val.Type = "DateTime"
	dtp.Val.Value = value

	return dtp
}

func (dtp *DateTimeProperty) Type() string {
	return dtp.PropertyImpl.Type
}

func (dtp *DateTimeProperty) Value() any {
	return dtp.Val
}

// TextProperty stores values of type text
type TextProperty struct {
	PropertyImpl
	Val         string  `json:"value"`
	ObservedAt_ *string `json:"observedAt,omitempty"`
}

func (tp *TextProperty) Type() string {
	return tp.PropertyImpl.Type
}

func (tp *TextProperty) Value() any {
	return tp.Val
}

func (tp *TextProperty) ObservedAt() string {
	if tp.ObservedAt_ != nil {
		return *tp.ObservedAt_
	}
	return ""
}

// TextListProperty stores values of type text list
type TextListProperty struct {
	PropertyImpl
	Val         []string `json:"value"`
	ObservedAt_ *string  `json:"observedAt,omitempty"`
}

func (tlp *TextListProperty) Type() string {
	return tlp.PropertyImpl.Type
}

func (tlp *TextListProperty) Value() any {
	return tlp.Val
}

func (tlp *TextListProperty) ObservedAt() string {
	if tlp.ObservedAt_ != nil {
		return *tlp.ObservedAt_
	}
	return ""
}

// NewTextListProperty accepts a value as a string array and returns a new TextListProperty
func NewTextListProperty(value []string) *TextListProperty {
	return &TextListProperty{
		PropertyImpl: PropertyImpl{Type: "Property"},
		Val:          value,
	}
}

// NewNumberPropertyFromString accepts a value as a string and returns a new NumberProperty
func NewNumberPropertyFromString(value string) *NumberProperty {
	number, _ := strconv.ParseFloat(value, 64)
	return NewNumberProperty(number)
}

// NewTextProperty accepts a value as a string and returns a new TextProperty
func NewTextProperty(value string) *TextProperty {
	return &TextProperty{
		PropertyImpl: PropertyImpl{Type: "Property"},
		Val:          value,
	}
}

func UnmarshalP(body map[string]any) (types.Property, error) {
	value, ok := body["value"]
	if !ok {
		return nil, fmt.Errorf("properties without a value attribute are not supported")
	}

	if value == nil {
		// nil values are not allowed, but can happen anyway ...
		// here we handle them by returning an empty slice of strings instead
		return NewTextListProperty([]string{}), nil
	}

	switch typedValue := value.(type) {
	case float64:
		np := NewNumberProperty(typedValue)
		// Parse property metadata
		if obsA, ok := body["observedAt"]; ok {
			if observedAt, ok := obsA.(string); ok {
				np.ObservedAt_ = &observedAt
			}
		}
		if unit, ok := body["unitCode"]; ok {
			if unitCode, ok := unit.(string); ok {
				np.UnitCode = &unitCode
			}
		}
		if obsB, ok := body["observedBy"]; ok {
			if observedBy, ok := obsB.(map[string]any); ok {
				r, err := relationships.UnmarshalR(observedBy)
				if err != nil {
					return nil, fmt.Errorf("observedBy is not a valid relationship: %w", err)
				}
				np.ObservedBy = r
			}
		}
		return np, nil
	case string:
		tp := NewTextProperty(sanitizeString(typedValue))

		if obsA, ok := body["observedAt"]; ok {
			if observedAt, ok := obsA.(string); ok {
				tp.ObservedAt_ = &observedAt
			}
		}
		return tp, nil
	case map[string]any:
		return unmarshalPropertyObject(typedValue)
	case []any:
		values := []string{}
		for _, v := range typedValue {
			str, ok := v.(string)
			if ok {
				values = append(values, sanitizeString(str))
			}
		}
		return NewTextListProperty(values), nil
	default:
		return NewTextProperty(fmt.Sprintf("support for type %T not implemented", typedValue)), nil
	}
}

func sanitizeString(input string) string {
	if len(input) >= 6 {
		for runeIdx, stopIdx := 0, len(input)-6; runeIdx <= stopIdx; runeIdx++ {
			if input[runeIdx] == '\\' {
				if input[runeIdx+1] == 'u' {
					r, err := strconv.ParseInt(input[runeIdx+2:runeIdx+6], 16, 32)
					if err != nil {
						continue
					}

					return input[:runeIdx] + string(rune(r)) + sanitizeString(input[runeIdx+6:])
				}
			}
		}
	}

	return input
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
