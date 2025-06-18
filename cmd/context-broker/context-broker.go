package main

import (
	"context"
	"flag"
	"io"
	"log/slog"
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

func DefaultFlags() FlagMap {
	return FlagMap{
		listenAddress: "",     // listen on all ipv4 and ipv6 interfaces
		servicePort:   "8080", //
		controlPort:   "",     // control port disabled by default

		configPath: "/opt/diwise/config/default.yaml",
		opaPath:    "/opt/diwise/config/authz.rego",

		logFormat: "json",
	}
}

func main() {
	ctx, flags := parseExternalConfig(context.Background(), DefaultFlags())

	serviceVersion := buildinfo.SourceVersion()
	ctx, logger, cleanup := o11y.Init(ctx, serviceName, serviceVersion, flags[logFormat])
	defer cleanup()

	configFile, err := os.Open(flags[configPath])
	exitIf(err, logger, "failed to open the configuration file", "path", flags[configPath])
	defer configFile.Close()

	policyFile, err := os.Open(flags[opaPath])
	exitIf(err, logger, "unable to open opa policy file", "path", flags[opaPath])
	defer policyFile.Close()

	app, r := initialize(ctx, configFile, policyFile)
	app.Start()
	defer app.Stop()

	port := flags[servicePort]
	logger.Info("starting to listen for connections", "port", port)

	err = http.ListenAndServe(":"+port, r)
	exitIf(err, logger, "failed to listen for connections", "port", port)
}

func initialize(ctx context.Context, brokerConfig io.Reader, authPolices io.Reader) (cim.ContextInformationManager, *chi.Mux) {
	logger := logging.GetFromContext(ctx)

	cfg, err := config.Load(brokerConfig)
	exitIf(err, logger, "failed to load configuration")

	app, err := contextbroker.New(ctx, *cfg)
	exitIf(err, logger, "failed to configure context broker")

	r := router.New(serviceName)
	ngsild.RegisterHandlers(ctx, r, authPolices, app)

	return app, r
}

func parseExternalConfig(ctx context.Context, flags FlagMap) (context.Context, FlagMap) {

	// Allow environment variables to override certain defaults
	envOrDef := env.GetVariableOrDefault
	flags[servicePort] = envOrDef(ctx, "SERVICE_PORT", flags[servicePort])
	flags[controlPort] = envOrDef(ctx, "CONTROL_PORT", flags[controlPort])

	apply := func(f FlagType) func(string) error {
		return func(value string) error {
			flags[f] = value
			return nil
		}
	}

	// Allow command line arguments to override defaults and environment variables
	flag.Func("config", "A configuration file containing federation information", apply(configPath))
	flag.Func("policies", "An authorization policy file", apply(opaPath))
	flag.Func("logformat", "Choose json or text format for logging", apply(logFormat))
	flag.Parse()

	return ctx, flags
}

func exitIf(err error, logger *slog.Logger, msg string, args ...any) {
	if err != nil {
		logger.With(args...).Error(msg, "err", err.Error())
		os.Exit(1)
	}
}
