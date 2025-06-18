package main

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
