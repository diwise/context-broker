package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"strings"

	contextbroker "github.com/diwise/context-broker/internal/pkg/application/context-broker"
	"github.com/diwise/context-broker/internal/pkg/infrastructure/router"
	"github.com/diwise/context-broker/internal/pkg/infrastructure/tracing"
	ngsild "github.com/diwise/context-broker/internal/pkg/presentation/api/ngsi-ld"
	"github.com/rs/zerolog/log"
)

var configFilePath string

func main() {

	serviceName := "context-broker"
	serviceVersion := "0.0.1"

	logger := log.With().Str("service", strings.ToLower(serviceName)).Logger()
	logger.Info().Msg("starting up ...")

	flag.StringVar(&configFilePath, "config", "/opt/diwise/config/default.yaml", "A configuration file containing federation information")
	flag.Parse()

	ctx := context.Background()

	cleanup, err := tracing.Init(ctx, logger, serviceName, serviceVersion)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to init tracing")
	}
	defer cleanup()

	configfile, err := os.Open(configFilePath)
	if err != nil {
		logger.Fatal().Err(err).Msgf("failed to open the configuration file %s", configFilePath)
	}

	cfg, err := contextbroker.LoadConfiguration(configfile)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load configuration")
	}

	app, err := contextbroker.New(logger, *cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to configure the context broker")
	}

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
