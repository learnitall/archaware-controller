package main

import "os"

func main() {
	ctx, stop := Setup()

	defer stop()

	if len(os.Args) > 1 && os.Args[1] == "clean" {
		go Clean(&ctx, stop)
	} else {
		go EnsureNodeTaints(&ctx)
		go EnsurePodTolerations(&ctx)
	}

	<-ctx.Done()
}
