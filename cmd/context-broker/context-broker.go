package main

import (
	"context"
	"flag"
	"net/http"
	"os"

	contextbroker "github.com/diwise/context-broker/internal/pkg/application/context-broker"
	"github.com/diwise/context-broker/internal/pkg/infrastructure/router"
	ngsild "github.com/diwise/context-broker/internal/pkg/presentation/api/ngsi-ld"
	"github.com/diwise/service-chassis/pkg/infrastructure/buildinfo"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"
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

	configfile, err := os.Open(configFilePath)
	if err != nil {
		logger.Fatal().Err(err).Msgf("failed to open the configuration file %s", configFilePath)
	}

	cfg, err := contextbroker.LoadConfiguration(configfile)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load configuration")
	}

	app, err := contextbroker.New(ctx, *cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to configure the context broker")
	}
	app.Start()
	defer app.Stop()

	r := router.New(serviceName)

	policies, err := os.Open(opaFilePath)
	if err != nil {
		logger.Fatal().Err(err).Msg("unable to open opa policy file")
	}
	defer policies.Close()

	ngsild.RegisterHandlers(r, policies, app, logger)

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
