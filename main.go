package main

import (
	"flag"
)

func main() {
	clean := flag.Bool(
		"clean",
		false,
		"If given, will remove supported-arch taints from nodes and delete all pods so their tolerations can be reset",
	)
	flag.String(
		"kubeconfig",
		"",
		"absolute path to the kubeconfig file. Precedence: given kubeconfig > $KUBECONFIG > ~/.config/kube",
	)
	flag.Parse()

	ctx, stop := Setup()

	defer stop()

	if *clean {
		go Clean(&ctx, stop)
	} else {
		go EnsureNodeTaints(&ctx)
		go EnsurePodTolerations(&ctx)
	}

	<-ctx.Done()
}
