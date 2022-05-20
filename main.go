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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type ContextKey string

const (
	OPERATOR_NAME           string        = "archaware"
	VERSION                 string        = "v0.1.0"
	RECONCILIATION_INTERVAL time.Duration = time.Second
	K8S_KUBECONFIG_PATH_KEY ContextKey    = "kubeconfig"
	K8S_CONFIG_KEY          ContextKey    = "k8sconfig"
	K8S_CLIENTSET_KEY       ContextKey    = "k8sclientset"
	NODE_INT_CHAN_KEY       ContextKey    = "nodeintervalchan"
	POD_INT_CHAN_KEY        ContextKey    = "podntervalchan"
	INTERVAL_CHANS_KEY      ContextKey    = "intervalchans"
)

func consumeChannel(target chan int, stop chan int) {
	for {
		select {
		case <-target:
		case <-stop:
			return
		}
	}
}

func ensureNodeTaints(ctx *context.Context) {
	target := func() {
		log.Info().Msg("node")
	}
	runOnInterval(
		ctx,
		NODE_INT_CHAN_KEY,
		target,
	)
}

func ensurePodTolerations(ctx *context.Context) {
	target := func() {
		log.Info().Msg("pod")
	}
	runOnInterval(
		ctx,
		POD_INT_CHAN_KEY,
		target,
	)
}

func runOnInterval(
	ctx *context.Context,
	interval_chan_key ContextKey,
	target func(),
) {
	var interval_chan chan int = (*ctx).Value(interval_chan_key).(chan int)
	for {
		select {
		case <-(*ctx).Done():
			return
		case <-interval_chan:
			stopConsuming := make(chan int)
			go consumeChannel(interval_chan, stopConsuming)
			target()
			stopConsuming <- 1
		}
	}
}

func startIntervals(ctx *context.Context) {
	ticker := time.NewTicker(RECONCILIATION_INTERVAL)
	interval_channels := (*ctx).Value(INTERVAL_CHANS_KEY).([]chan int)
	for {
		select {
		case <-(*ctx).Done():
			ticker.Stop()
			return
		case <-ticker.C:
			for _, channel := range interval_channels {
				channel <- 1
			}
		}
	}
}

// setupLogging kicks-off the rs/zerolog logger
func setupLogging(ctx *context.Context) {
	log.Logger = log.Output(
		zerolog.ConsoleWriter{
			TimeFormat: time.RFC3339,
			NoColor:    false,
			Out:        os.Stdout,
		},
	)
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Info().
		Str("operator_name", OPERATOR_NAME).
		Str("version", VERSION).
		Msg("Hello World!")
}

// setupK8sClient creates and authenticates a Kubernetes client
// using either the given kubeconfig or the current pod's service account
// Preferred order of configuration: kubeconfig given on CLI, KUBECONFIG env
// variable, in-cluster config
func setupK8sClient(ctx *context.Context, kubeconfig string) error {
	var config *rest.Config
	var err error

	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBECONFIG")
	}
	if kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	*ctx = context.WithValue(*ctx, K8S_KUBECONFIG_PATH_KEY, kubeconfig)

	if err != nil {
		log.Fatal().
			AnErr("err", err).
			Msg("Unable to create Kubernetes client configuration")
		return err
	}
	*ctx = context.WithValue(*ctx, K8S_CONFIG_KEY, config)

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal().
			AnErr("err", err).
			Msg("Unable to create Kubernetes client")
		return err
	}
	*ctx = context.WithValue(*ctx, K8S_CLIENTSET_KEY, clientset)

	return nil
}

// setup performs setup functions required before execution,
// returning a context object populated with variables needed
// by other functions consuming the context
func setup() (context.Context, context.CancelFunc) {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	ctx := context.Background()
	setupLogging(&ctx)

	err := setupK8sClient(&ctx, *kubeconfig)
	if err != nil {
		panic(err)
	}
	ctx, stop := signal.NotifyContext(
		ctx,
		syscall.SIGINT, syscall.SIGTERM,
	)
	interval_channels := [](chan int){make(chan int), make(chan int)}
	ctx = context.WithValue(ctx, NODE_INT_CHAN_KEY, interval_channels[0])
	ctx = context.WithValue(ctx, POD_INT_CHAN_KEY, interval_channels[1])
	ctx = context.WithValue(ctx, INTERVAL_CHANS_KEY, interval_channels)
	return ctx, stop
}

func main() {
	ctx, stop := setup()

	defer stop()
	go ensureNodeTaints(&ctx)
	go ensurePodTolerations(&ctx)
	startIntervals(&ctx)
	<-ctx.Done()
}
