package main

import (
	"context"
	"flag"
	"io"
	"net/http"
	"os"

	"github.com/diwise/context-broker/internal/pkg/application/cim"
	"github.com/diwise/context-broker/internal/pkg/application/config"
	contextbroker "github.com/diwise/context-broker/internal/pkg/application/context-broker"
	"github.com/diwise/context-broker/internal/pkg/infrastructure/router"
	ngsild "github.com/diwise/context-broker/internal/pkg/presentation/api/ngsi-ld"
	"github.com/diwise/service-chassis/pkg/infrastructure/buildinfo"
	"github.com/diwise/service-chassis/pkg/infrastructure/env"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

const serviceName string = "context-broker"

var configFilePath string
var opaFilePath string

func main() {

	serviceVersion := buildinfo.SourceVersion()
	ctx, logger, cleanup := o11y.Init(context.Background(), serviceName, serviceVersion)
	defer cleanup()

	flag.StringVar(&configFilePath, "config", "/opt/diwise/config/default.yaml", "A configuration file containing federation information")
	flag.StringVar(&opaFilePath, "policies", "/opt/diwise/config/authz.rego", "An authorization policy file")
	flag.Parse()

	configFile, err := os.Open(configFilePath)
	if err != nil {
		logger.Fatal().Err(err).Msgf("failed to open the configuration file %s", configFilePath)
	}
	defer configFile.Close()

	policyFile, err := os.Open(opaFilePath)
	if err != nil {
		logger.Fatal().Err(err).Msg("unable to open opa policy file")
	}
	defer policyFile.Close()

	app, r := initialize(ctx, logger, configFile, policyFile)
	app.Start()
	defer app.Stop()

	port := env.GetVariableOrDefault(logger, "SERVICE_PORT", "8080")

	logger.Info().Str("port", port).Msg("starting to listen for connections")

	err = http.ListenAndServe(":"+port, r)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to listen for connections")
	}
}

func initialize(ctx context.Context, logger zerolog.Logger, brokerConfig io.Reader, authPolices io.Reader) (cim.ContextInformationManager, *chi.Mux) {
	cfg, err := config.Load(brokerConfig)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load configuration")
	}

	app, err := contextbroker.New(ctx, *cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to configure the context broker")
	}

	r := router.New(serviceName)
	ngsild.RegisterHandlers(r, authPolices, app, logger)

	return app, r
}
