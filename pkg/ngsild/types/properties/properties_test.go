package properties

import (
	"testing"

	"github.com/matryer/is"
)

func TestSanitizeEmptyString(t *testing.T) {
	is := is.New(t)
	is.Equal(sanitizeString(""), "")
}

func TestSanitizeInvalidEscapeString(t *testing.T) {
	is := is.New(t)
	is.Equal(sanitizeString("\\uqwab"), "\\uqwab")
}

func TestSanitizeAmpersandString(t *testing.T) {
	is := is.New(t)
	is.Equal(sanitizeString("\\u0026"), "&")
}

func TestSanitizeDoubleAmpersandString(t *testing.T) {
	is := is.New(t)
	is.Equal(sanitizeString("\\u0026\\u0026"), "&&")
}

func TestSanitizeEmbeddedAmpersandString(t *testing.T) {
	is := is.New(t)
	is.Equal(sanitizeString("A \\u0026 B"), "A & B")
}

func TestSanitizeCroppedString(t *testing.T) {
	is := is.New(t)
	is.Equal(sanitizeString("A \\u0026 \\u00"), "A & \\u00")
}
