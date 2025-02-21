package action

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/docker/cli/cli/config"
	"github.com/docker/docker/pkg/homedir"
	"k8s.io/klog"
	klog_v2 "k8s.io/klog/v2"

	"github.com/werf/kubedog/pkg/display"
	"github.com/werf/logboek"
)

type LogColorMode string

const (
	LogColorModeUnspecified LogColorMode = ""
	LogColorModeAuto        LogColorMode = "auto"
	LogColorModeOff         LogColorMode = "off"
	LogColorModeOn          LogColorMode = "on"
)

type ReleaseStorageDriver string

const (
	ReleaseStorageDriverDefault    ReleaseStorageDriver = ""
	ReleaseStorageDriverSecrets    ReleaseStorageDriver = "secrets"
	ReleaseStorageDriverSecret     ReleaseStorageDriver = "secret"
	ReleaseStorageDriverConfigMaps ReleaseStorageDriver = "configmaps"
	ReleaseStorageDriverConfigMap  ReleaseStorageDriver = "configmap"
	ReleaseStorageDriverMemory     ReleaseStorageDriver = "memory"
	ReleaseStorageDriverSQL        ReleaseStorageDriver = "sql"
)

const (
	DefaultQPSLimit              = 30
	DefaultBurstLimit            = 100
	DefaultNetworkParallelism    = 30
	DefaultLocalKubeVersion      = "1.20.0"
	DefaultProgressPrintInterval = 5 * time.Second
	DefaultReleaseHistoryLimit   = 10
	DefaultLogColorMode          = LogColorModeAuto
)

var DefaultRegistryCredentialsPath = filepath.Join(homedir.Get(), ".docker", config.ConfigFileName)

func initKubedog(ctx context.Context) error {
	flag.CommandLine.Parse([]string{})

	display.SetOut(logboek.Context(ctx).OutStream())
	display.SetErr(logboek.Context(ctx).ErrStream())

	if err := silenceKlog(ctx); err != nil {
		return fmt.Errorf("silence klog: %w", err)
	}

	if err := silenceKlogV2(ctx); err != nil {
		return fmt.Errorf("silence klog v2: %w", err)
	}

	return nil
}

func silenceKlog(ctx context.Context) error {
	fs := flag.NewFlagSet("klog", flag.PanicOnError)
	klog.InitFlags(fs)

	if err := fs.Set("logtostderr", "false"); err != nil {
		return fmt.Errorf("set logtostderr: %w", err)
	}

	if err := fs.Set("alsologtostderr", "false"); err != nil {
		return fmt.Errorf("set alsologtostderr: %w", err)
	}

	if err := fs.Set("stderrthreshold", "5"); err != nil {
		return fmt.Errorf("set stderrthreshold: %w", err)
	}

	// Suppress info and warnings from client-go reflector
	klog.SetOutputBySeverity("INFO", ioutil.Discard)
	klog.SetOutputBySeverity("WARNING", ioutil.Discard)
	klog.SetOutputBySeverity("ERROR", ioutil.Discard)
	klog.SetOutputBySeverity("FATAL", logboek.Context(ctx).ErrStream())

	return nil
}

func silenceKlogV2(ctx context.Context) error {
	fs := flag.NewFlagSet("klog", flag.PanicOnError)
	klog_v2.InitFlags(fs)

	if err := fs.Set("logtostderr", "false"); err != nil {
		return fmt.Errorf("set logtostderr: %w", err)
	}

	if err := fs.Set("alsologtostderr", "false"); err != nil {
		return fmt.Errorf("set alsologtostderr: %w", err)
	}

	if err := fs.Set("stderrthreshold", "5"); err != nil {
		return fmt.Errorf("set stderrthreshold: %w", err)
	}

	// Suppress info and warnings from client-go reflector
	klog_v2.SetOutputBySeverity("INFO", ioutil.Discard)
	klog_v2.SetOutputBySeverity("WARNING", ioutil.Discard)
	klog_v2.SetOutputBySeverity("ERROR", ioutil.Discard)
	klog_v2.SetOutputBySeverity("FATAL", logboek.Context(ctx).ErrStream())

	return nil
}
