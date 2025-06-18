package main

import (
	"context"
	"io"

	"github.com/diwise/service-chassis/pkg/infrastructure/servicerunner"
)

type FlagType int
type FlagMap map[FlagType]string

const (
	listenAddress FlagType = iota
	servicePort
	controlPort

	configPath
	opaPath

	logFormat
)

type AppConfig struct {
	brokerConfig io.ReadCloser
	opaConfig    io.ReadCloser

	cancelContext context.CancelFunc
}

var ifnot = servicerunner.IfNot[AppConfig]
var onstarting = servicerunner.OnStarting[AppConfig]
var onrunning = servicerunner.OnRunning[AppConfig]
var onshutdown = servicerunner.OnShutdown[AppConfig]
var webserver = servicerunner.WithHTTPServeMux[AppConfig]
var muxinit = servicerunner.OnMuxInit[AppConfig]
var listen = servicerunner.WithListenAddr[AppConfig]
var port = servicerunner.WithPort[AppConfig]
var pprof = servicerunner.WithPPROF[AppConfig]
var liveness = servicerunner.WithK8SLivenessProbe[AppConfig]
var readiness = servicerunner.WithK8SReadinessProbes[AppConfig]
