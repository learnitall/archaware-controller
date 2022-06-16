package main

func main() {
	ctx, stop := Setup()

	defer stop()
	go EnsureNodeTaints(&ctx)
	go EnsurePodTolerations(&ctx)
	<-ctx.Done()
	stop()
}
