package main

import (
	"context"
	"flag"
	"fmt"
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
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"
	"github.com/go-chi/chi/v5"
)

const serviceName string = "context-broker"

var configFilePath string
var opaFilePath string

func main() {

	serviceVersion := buildinfo.SourceVersion()
	ctx, logger, cleanup := o11y.Init(context.Background(), serviceName, serviceVersion, "json")
	defer cleanup()

	flag.StringVar(&configFilePath, "config", "/opt/diwise/config/default.yaml", "A configuration file containing federation information")
	flag.StringVar(&opaFilePath, "policies", "/opt/diwise/config/authz.rego", "An authorization policy file")
	flag.Parse()

	configFile, err := os.Open(configFilePath)
	if err != nil {
		fatal(ctx, fmt.Sprintf("failed to open the configuration file %s", configFilePath), err)
	}
	defer configFile.Close()

	policyFile, err := os.Open(opaFilePath)
	if err != nil {
		fatal(ctx, "unable to open opa policy file", err)
	}
	defer policyFile.Close()

	app, r := initialize(ctx, configFile, policyFile)
	app.Start()
	defer app.Stop()

	port := env.GetVariableOrDefault(ctx, "SERVICE_PORT", "8080")

	logger.Info("starting to listen for connections", "port", port)

	err = http.ListenAndServe(":"+port, r)
	if err != nil {
		fatal(ctx, "failed to listen for connections", err)
	}
}

func initialize(ctx context.Context, brokerConfig io.Reader, authPolices io.Reader) (cim.ContextInformationManager, *chi.Mux) {
	cfg, err := config.Load(brokerConfig)
	if err != nil {
		fatal(ctx, "failed to load configuration", err)
	}

	app, err := contextbroker.New(ctx, *cfg)
	if err != nil {
		fatal(ctx, "failed to configure the context broker", err)
	}

	r := router.New(serviceName)
	ngsild.RegisterHandlers(ctx, r, authPolices, app)

	return app, r
}

func fatal(ctx context.Context, msg string, err error) {
	logger := logging.GetFromContext(ctx)
	logger.Error(msg, "err", err.Error())
	os.Exit(1)
}
