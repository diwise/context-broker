package ngsild

import (
	"log/slog"
	"net/http"
)

// TODO: Load from file in file system instead of hardcoding a constant
const DefaultContext string = `{
    "@context": [
        "https://raw.githubusercontent.com/diwise/context-broker/main/assets/jsonldcontexts/default-context.jsonld"
    ]
}`

func NewServeContextHandler(logger *slog.Logger) http.HandlerFunc {
	responseBytes := []byte(DefaultContext)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextID := r.PathValue("contextId")

		if contextID != "default-context.jsonld" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		logger.Info("default context requested from client")

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(responseBytes)
	})
}
