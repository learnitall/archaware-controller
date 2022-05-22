package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/containerd/containerd"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/client-go/util/retry"
)

type ContextKey string

const (
	OPERATOR_NAME           string        = "archaware"
	VERSION                 string        = "v0.1.0"
	RECONCILIATION_INTERVAL time.Duration = time.Second
	ARCH_TAINT_KEY_NAME     string        = "supported-arch"
	K8S_KUBECONFIG_PATH_KEY ContextKey    = "kubeconfig"
	K8S_CONFIG_KEY          ContextKey    = "k8sconfig"
	K8S_CLIENTSET_KEY       ContextKey    = "k8sclientset"
	NODE_INT_CHAN_KEY       ContextKey    = "nodeintervalchan"
	POD_INT_CHAN_KEY        ContextKey    = "podntervalchan"
	INTERVAL_CHANS_KEY      ContextKey    = "intervalchans"
	CONTAINERD_CLIENT_KEY   ContextKey    = "containerdclient"
)

func ensureNodeTaints(ctx *context.Context) {
	clientset := (*ctx).Value(K8S_CLIENTSET_KEY).(*kubernetes.Clientset)
	nodeClient := clientset.CoreV1().Nodes()
	nodeWatch, err := nodeClient.Watch(
		*ctx,
		metav1.ListOptions{},
	)
	if err != nil {
		log.Fatal().
			AnErr("err", err).
			Msg("Unable to watch nodes")
	}
	for {
		select {
		case <-(*ctx).Done():
			nodeWatch.Stop()
			return
		case nodeEvent := <-nodeWatch.ResultChan():
			if nodeEvent.Type == watch.Error {
				log.Error().
					Interface("api-err", nodeEvent.Object).
					Msg("Received error while watching nodes")
				return
			}
			node := nodeEvent.Object.(*v1.Node)
			name := node.ObjectMeta.Name
			arch := node.Status.NodeInfo.Architecture

			getLog := func(level zerolog.Level) *zerolog.Event {
				return log.WithLevel(level).
					Str("name", name).
					Str("arch", arch)
			}

			if nodeEvent.Type == watch.Deleted {
				getLog(zerolog.InfoLevel).
					Msg("Got deleted node")
				return
			}

			getLog(zerolog.InfoLevel).
				Interface("event-type", nodeEvent.Type).
				Msg("Checking state of node")

			attemptCounter := 0
			retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				attemptCounter += 1
				result, getErr := nodeClient.Get(
					*ctx,
					name,
					metav1.GetOptions{},
				)
				if getErr != nil {
					getLog(zerolog.WarnLevel).
						Msg("Unable to get latest information on node")
					return getErr
				}

				getLog(zerolog.DebugLevel).
					Interface("node-taints", node.Spec.Taints).
					Msg("node's current taints before update")

				for i, taint := range result.Spec.Taints {
					if taint.Key == ARCH_TAINT_KEY_NAME {
						if taint.Value == arch {
							getLog(zerolog.DebugLevel).
								Msg("Taint with proper architecture was found, doing nothing")
							return nil
						} else {
							getLog(zerolog.DebugLevel).
								Msg("Taint with bad architecture was found, updating")
							numTaints := len(result.Spec.Taints)
							// Delete this taint as it needs to be updated
							result.Spec.Taints[i] = result.Spec.Taints[numTaints-1]
							result.Spec.Taints = result.Spec.Taints[:numTaints-1]
						}
					}
				}

				result.Spec.Taints = append(
					result.Spec.Taints,
					v1.Taint{
						Key:    ARCH_TAINT_KEY_NAME,
						Value:  arch,
						Effect: v1.TaintEffectNoSchedule,
					},
				)

				getLog(zerolog.DebugLevel).
					Interface("node-taints", result.Spec.Taints).
					Msg("Applying the following taints")
				_, updateErr := nodeClient.Update(
					*ctx,
					result,
					metav1.UpdateOptions{},
				)
				if updateErr != nil {
					getLog(zerolog.WarnLevel).
						Err(updateErr).
						Msg("Unable to update taint on node")
					return updateErr
				}

				getLog(zerolog.InfoLevel).
					Int("attempts", attemptCounter).
					Msg("Added architecture taint on node")
				return nil
			})

			if retryErr != nil {
				getLog(zerolog.WarnLevel).
					Int("attempts", attemptCounter).
					Msg("Unable to update architecture taint on node")
			}
		}
	}
}

func ensurePodTolerations(ctx *context.Context) {
	clientset := (*ctx).Value(K8S_CLIENTSET_KEY).(*kubernetes.Clientset)
	podClient := clientset.CoreV1().Pods("")
	podWatch, err := podClient.Watch(
		*ctx,
		metav1.ListOptions{},
	)
	if err != nil {
		log.Fatal().
			AnErr("err", err).
			Msg("Unable to watch pods")
	}
	// containerdClient := (*ctx).Value(CONTAINERD_CLIENT_KEY).(*containerd.Client)
	for {
		select {
		case <-(*ctx).Done():
			podWatch.Stop()
			return
		case podEvent := <-podWatch.ResultChan():
			if podEvent.Type == watch.Error {
				log.Error().
					Interface("api-err", podEvent.Object).
					Msg("Received error while watching pods")
				return
			}
			pod := podEvent.Object.(*v1.Pod)
			name := pod.Name
			log.Info().
				Str("pod-name", name).
				Msg("Got pod")
			for _, container := range pod.Spec.Containers {
				log.Info().
					Str("container-name", container.Name).
					Str("container-image", container.Image).
					Msg("Got container")
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
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
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

func setupContainerd(ctx *context.Context) error {
	cmd := exec.Command("containerd")

	// pipe output of containerd to stdout live
	// https://stackoverflow.com/questions/48253268/print-the-stdout-from-exec-command-in-real-time-in-go
	var stdBuffer bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &stdBuffer)
	cmd.Stdout = mw
	cmd.Stderr = mw

	go func() {
		log.Info().
			Msg("Starting containerd")
		cmd.Start()
		<-(*ctx).Done()
		if err := cmd.Process.Kill(); err != nil {
			log.Warn().
				AnErr("err", err).
				Msg("Unable to kill containerd process")
		}
		if err := cmd.Process.Kill(); err != nil {
			log.Warn().
				AnErr("err", err).
				Msg("Unable to wait for containerd process to complete")
		}
	}()

	client, err := containerd.New("/run/containerd/containerd.sock")
	if err != nil {
		log.Fatal().
			AnErr("err", err).
			Msg("Unable to create containerd client")
		return err
	}

	*ctx = context.WithValue(*ctx, CONTAINERD_CLIENT_KEY, client)
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

	err := setupContainerd(&ctx)
	if err != nil {
		panic(err)
	}

	err = setupK8sClient(&ctx, *kubeconfig)
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
	<-ctx.Done()
	stop()
}
