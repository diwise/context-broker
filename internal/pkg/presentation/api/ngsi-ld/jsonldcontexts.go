package ngsild

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

//TODO: Load from file in file system instead of hardcoding a constant
const DefaultContext string = `{
    "@context": [
        "https://raw.githubusercontent.com/diwise/context-broker/main/assets/jsonldcontexts/diwise-context.jsonld",
        "https://raw.githubusercontent.com/diwise/context-broker/main/assets/jsonldcontexts/fiware-context.jsonld",
        "https://raw.githubusercontent.com/diwise/context-broker/main/assets/jsonldcontexts/water-meter-context.jsonld",
        "https://raw.githubusercontent.com/diwise/context-broker/main/assets/jsonldcontexts/ngsi-ld-core-context-v1.5.jsonld"
    ]
}`

func NewServeContextHandler(logger zerolog.Logger) http.HandlerFunc {
	responseBytes := []byte(DefaultContext)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextID := chi.URLParam(r, "contextId")

		if contextID != "default-context.jsonld" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		logger.Info().Msg("default context requested from client")

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(responseBytes)
	})
}
