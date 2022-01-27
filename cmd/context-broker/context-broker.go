package main

import (
	"context"
	"net/http"
	"os"
	"strings"

	contextbroker "github.com/diwise/ngsi-ld-context-broker/internal/pkg/application/context-broker"
	"github.com/diwise/ngsi-ld-context-broker/internal/pkg/infrastructure/router"
	"github.com/diwise/ngsi-ld-context-broker/internal/pkg/infrastructure/tracing"
	ngsild "github.com/diwise/ngsi-ld-context-broker/internal/pkg/presentation/api/ngsi-ld"
	"github.com/rs/zerolog/log"
)

func main() {

	serviceName := "context-broker"
	serviceVersion := "0.0.1"

	logger := log.With().Str("service", strings.ToLower(serviceName)).Logger()
	logger.Info().Msg("starting up ...")

	ctx := context.Background()

	cleanup, err := tracing.Init(ctx, logger, serviceName, serviceVersion)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to init tracing")
	}
	defer cleanup()

	app := contextbroker.New(logger)
	r := router.New(serviceName)

	ngsild.RegisterHandlers(r, app, logger)

	port := os.Getenv("SERVICE_PORT")
	if port == "" {
		port = "8080"
	}

	logger.Info().Str("port", port).Msg("starting to listen for connections")

	err = http.ListenAndServe(":"+port, r)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to listen for connections")
	}
}
