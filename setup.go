package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// setupLogging kicks-off the rs/zerolog logger
func setupLogging() {
	log.Logger = log.Output(
		zerolog.ConsoleWriter{
			TimeFormat: time.RFC3339,
			NoColor:    false,
			Out:        os.Stdout,
		},
	)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log.Info().
		Str("operator_name", OPERATOR_NAME).
		Str("version", VERSION).
		Msg("Hello World!")

	// containerd uses logrus, so allow for those logs
	// to be visible as well
	logrus.SetLevel(logrus.DebugLevel)
}

// setupK8sClient creates and authenticates a Kubernetes client
// using either a kubeconfig or the current pod's service account
// Preferred order of configuration: kubeconfig given on CLI, KUBECONFIG env
// variable, kubeconfig on home directory, in-cluster config
func setupK8sClient(ctx *context.Context) error {
	var config *rest.Config
	var err error

	kubeconfigEnv := os.Getenv("KUBECONFIG")
	home := homedir.HomeDir()
	kubeconfigArg := flag.Lookup("kubeconfig").Value.String()

	var kubeconfig string
	if kubeconfigArg == "" {
		if kubeconfigEnv == "" {
			if home == "" {
				log.Debug().
					Msg("Could not extract user provided config")
				kubeconfig = ""
			} else {
				log.Debug().
					Msg("Using default kubeconfig in home directory")
				kubeconfig = filepath.Join(home, ".kube", "config")
			}
		} else {
			log.Debug().
				Msg("Using kubeconfig from $KUBECONFIG")
			kubeconfig = kubeconfigEnv
		}
	} else {
		log.Debug().
			Msg("Using kubeconfig provided as input")
		kubeconfig = kubeconfigArg
	}

	if _, err = os.Stat(kubeconfig); err == nil {
		log.Info().
			Str("kubeconfig", kubeconfig).
			Msg("Got the following kubeconfig")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		log.Warn().
			Str("kubeconfig", kubeconfig).
			Msg("Assuming in-cluster kubeconfig, as given kubeconfig does not exist")
		config, err = rest.InClusterConfig()
	}
	*ctx = context.WithValue(*ctx, K8S_KUBECONFIG_PATH_KEY, kubeconfig)

	if err != nil {
		log.Error().
			AnErr("err", err).
			Msg("Unable to create Kubernetes client configuration")
		return err
	}
	*ctx = context.WithValue(*ctx, K8S_CONFIG_KEY, config)

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Error().
			AnErr("err", err).
			Msg("Unable to create Kubernetes client")
		return err
	}
	*ctx = context.WithValue(*ctx, K8S_INTERFACE_KEY, clientset)

	return nil
}

// GetK8sInterface pulls the set interface from the given context.
func GetK8sInterface(ctx *context.Context) kubernetes.Interface {
	result := (*ctx).Value(K8S_INTERFACE_KEY)
	if result == nil {
		return nil
	}
	return result.(kubernetes.Interface)
}

// Setup performs setup functions required before execution,
// returning a context object populated with variables needed
// by other functions consuming the context
func Setup() (context.Context, context.CancelFunc) {
	setupLogging()
	ctx := context.Background()

	err := setupK8sClient(&ctx)
	if err != nil {
		panic(err)
	}
	ctx, stop := signal.NotifyContext(
		ctx,
		syscall.SIGINT, syscall.SIGTERM,
	)
	return ctx, stop
}
