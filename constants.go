package main

import "time"

type ContextKey string

const (
	OPERATOR_NAME           string        = "archaware"
	VERSION                 string        = "v0.1.0"
	RECONCILIATION_INTERVAL time.Duration = time.Minute * time.Duration(1)
	ARCH_TAINT_KEY_NAME     string        = "supported-arch"
	K8S_KUBECONFIG_PATH_KEY ContextKey    = "kubeconfig"
	K8S_CONFIG_KEY          ContextKey    = "k8sconfig"
	K8S_INTERFACE_KEY       ContextKey    = "k8sclientset"
	MAX_RETRY_ATTEMPTS      int           = 5
)
