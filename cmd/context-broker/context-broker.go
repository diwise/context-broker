package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/diwise/context-broker/internal/pkg/application/config"
	contextbroker "github.com/diwise/context-broker/internal/pkg/application/context-broker"
	ngsild "github.com/diwise/context-broker/internal/pkg/presentation/api/ngsi-ld"
	"github.com/diwise/service-chassis/pkg/infrastructure/buildinfo"
	"github.com/diwise/service-chassis/pkg/infrastructure/env"
	k8shandlers "github.com/diwise/service-chassis/pkg/infrastructure/net/http/handlers"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"
	"github.com/diwise/service-chassis/pkg/infrastructure/servicerunner"
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

	appConfig, err := newConfig(ctx, flags)
	exitIf(err, logger, "failed to create application config")

	defer appConfig.brokerConfig.Close()
	defer appConfig.opaConfig.Close()

	ctx, appConfig.cancelContext = context.WithCancel(ctx)

	runner, err := initialize(ctx, flags, appConfig)
	exitIf(err, logger, "failed to initialize service")

	err = runner.Run(ctx)
	exitIf(err, logger, "service runner failed")

	logger.Info("shutting down")
}

func newConfig(ctx context.Context, flags FlagMap) (*AppConfig, error) {
	logger := logging.GetFromContext(ctx)

	configFile, err := os.Open(flags[configPath])
	exitIf(err, logger, "failed to open the configuration file", "path", flags[configPath])

	policyFile, err := os.Open(flags[opaPath])
	exitIf(err, logger, "unable to open opa policy file", "path", flags[opaPath])

	cfg := &AppConfig{
		brokerConfig: configFile,
		opaConfig:    policyFile,
	}

	return cfg, nil
}

func initialize(ctx context.Context, flags FlagMap, appConfig *AppConfig) (servicerunner.Runner[AppConfig], error) {
	probes := map[string]k8shandlers.ServiceProber{
		"mongo":    func(context.Context) (string, error) { return "ok", nil },
		"temporal": func(context.Context) (string, error) { return "ok", nil },
	}

	_, runner := servicerunner.New(ctx, *appConfig,
		ifnot(flags[controlPort] == "",
			webserver("control", listen(flags[listenAddress]), port(flags[controlPort]),
				pprof(), liveness(func() error { return nil }), readiness(probes),
			)),
		webserver("public", listen(flags[listenAddress]), port(flags[servicePort]),
			muxinit(func(ctx context.Context, identifier string, port string, svcCfg *AppConfig, handler *http.ServeMux) (err error) {

				svcCfg.url = "http://127.0.0.1:" + port

				brokerConfig, err := config.Load(appConfig.brokerConfig)
				if err != nil {
					return fmt.Errorf("failed to load configuration: %w", err)
				}

				svcCfg.app, err = contextbroker.New(ctx, *brokerConfig)
				if err != nil {
					return fmt.Errorf("failed to configure context broker: %w", err)
				}

				mux := http.NewServeMux()

				ngsild.RegisterHandlers(ctx, nil, appConfig.opaConfig, svcCfg.app)

				handler.Handle("GET /", mux)
				handler.Handle("POST /", mux)
				handler.Handle("DELETE /", mux)

				return nil
			}),
		),
		onstarting(func(ctx context.Context, svcCfg *AppConfig) (err error) {
			return nil
		}),
		onrunning(func(ctx context.Context, svcCfg *AppConfig) error {
			logging.GetFromContext(ctx).Info("service is running and waiting for connections")
			return nil
		}),
		onshutdown(func(ctx context.Context, svcCfg *AppConfig) error {
			if svcCfg.cancelContext != nil {
				svcCfg.cancelContext()
				svcCfg.cancelContext = nil
			}

			return nil
		}),
	)

	return runner, nil
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
