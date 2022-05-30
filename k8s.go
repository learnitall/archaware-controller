package main

import (
	"context"
	"os"

	"github.com/rs/zerolog/log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type K8S_CLIENTSET_CTX_KEY_TYPE struct{}

var K8S_CLIENTSET_CTX_KEY K8S_CLIENTSET_CTX_KEY_TYPE = K8S_CLIENTSET_CTX_KEY_TYPE{}

// CreateK8sClientset creates and returns a new kubernetes clientset.
// The only argument, kubeconfig takes in a path to the kubeconfig to be used.
// If left blank, then a path to the kubeconfig will attempt to be pulled from the KUBECONFIG environment variable.
// If it's not available, then it will be assumed an in-cluster configuration is available.
func CreateK8sClientset(kubeconfig string) (kubernetes.Interface, error) {
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

	if err != nil {
		log.Error().
			AnErr("err", err).
			Msg("Unable to create Kubernetes client configuration")
		return nil, err
	}

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Error().
			AnErr("err", err).
			Msg("Unable to create Kubernetes client")
		return nil, err
	}

	return clientset, nil
}

// WithK8sClientset adds the given clientset to the given context, allowing for it to be passed on to other objects at runtime.
func WithK8sClientset(ctx context.Context, clientset kubernetes.Interface) context.Context {
	return context.WithValue(ctx, K8S_CLIENTSET_CTX_KEY, clientset)
}

// GetK8sClientset pulls the set clientset from the given context.
func GetK8sClientset(ctx context.Context) kubernetes.Interface {
	result := ctx.Value(K8S_CLIENTSET_CTX_KEY)
	if result == nil {
		return nil
	}
	return result.(kubernetes.Interface)
}
