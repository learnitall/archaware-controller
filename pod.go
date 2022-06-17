package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/remotes/docker"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

func getArchitectures(ctx *context.Context, ref string) ([]string, error) {
	// Resolve image reference and fetch content
	fetchCtx := containerd.RemoteContext{
		Resolver: docker.NewResolver(
			docker.ResolverOptions{},
		),
	}

	// desc determines the 'thing' that is fetched later on.
	// if desc describes a manifest, a manifest will be fetched,
	// if desc describes an index, an index will be fetched,
	// if desc describes a blob, a blob will be fetched
	// See https://github.com/containerd/containerd/blob/9b33526ef64d921375598e0d568e98468d1ab81b/remotes/docker/fetcher.go#L39=
	// and https://github.com/containerd/containerd/blob/9b33526ef64d921375598e0d568e98468d1ab81b/remotes/docker/resolver.go
	name, desc, err := fetchCtx.Resolver.Resolve(*ctx, ref)
	if err != nil {
		log.Error().
			AnErr("err", err).
			Str("ref", ref).
			Msg("Unable to resolve image reference")
		return nil, err
	}

	getLog := func(level zerolog.Level) *zerolog.Event {
		return log.WithLevel(level).
			Str("name", name)
	}

	getLog(zerolog.DebugLevel).
		Str("reference", ref).
		Interface("desc", desc).
		Msg("Resolved image reference")

	fetcher, err := fetchCtx.Resolver.Fetcher(*ctx, name)
	if err != nil {
		getLog(zerolog.ErrorLevel).
			AnErr("err", err).
			Msg("Unable to create fetcher for image")
		return nil, err
	}

	fetchedContentReader, err := fetcher.Fetch(*ctx, desc)
	if err != nil {
		getLog(zerolog.ErrorLevel).
			AnErr("err", err).
			Msg("Unable to fetch content defined by descriptor")
		return nil, err
	}
	fetchedContentBuffer := bytes.Buffer{}
	_, err = fetchedContentBuffer.ReadFrom(fetchedContentReader)
	if err != nil {
		getLog(zerolog.ErrorLevel).
			AnErr("err", err).
			Msg("Unable to read remote content")
		return nil, err
	}
	fetchedContentBytes := fetchedContentBuffer.Bytes()
	getLog(zerolog.DebugLevel).
		Bytes("manifest", fetchedContentBytes).
		Msg("Fetched bytes for image")

	// unmarshal unmarshals fetchedContentBytes into the given target.
	unmarshal := func(target interface{}) error {
		if err := json.Unmarshal(fetchedContentBytes, &target); err != nil {
			getLog(zerolog.ErrorLevel).
				AnErr("err", err).
				Bytes("fetched", fetchedContentBytes).
				Msg("Unable to unmarshal fetched bytes")
			return err
		}
		return nil
	}

	// handleIndex is called when the fetchedContentBytes represents an
	// oci Index
	handleIndex := func() ([]string, error) {
		getLog(zerolog.DebugLevel).
			Msg("Got index for image")
		var index ocispec.Index
		if err := unmarshal(&index); err != nil {
			return nil, err
		}
		getLog(zerolog.DebugLevel).
			Interface("index", index).
			Msg("Unmarshalled index")
		var architectures []string = make([]string, 0)
		for _, manifest := range index.Manifests {
			architectures = append(architectures, manifest.Platform.Architecture)
		}
		getLog(zerolog.DebugLevel).
			Str("architectures", strings.Join(architectures, ", ")).
			Msg("Got architectures for image")
		return architectures, nil
	}

	// handleManifest is called when the fetchedContentBytes represents
	// an oci Manifest
	handleManifest := func() ([]string, error) {
		getLog(zerolog.DebugLevel).
			Msg("Got manifest for image")
		var manifest ocispec.Manifest
		if err := unmarshal(&manifest); err != nil {
			return nil, err
		}
		getLog(zerolog.DebugLevel).
			Interface("manifest", manifest).
			Msg("Unmarshalled manifest")
		if manifest.Config.Platform == nil {
			getLog(zerolog.WarnLevel).
				Interface("manifest", manifest).
				Msg("Platform not given in manifest, assuming amd64")
			return []string{"amd64"}, nil
		}
		return []string{manifest.Config.Platform.Architecture}, nil
	}

	if images.IsIndexType(desc.MediaType) {
		return handleIndex()
	} else if images.IsManifestType(desc.MediaType) {
		return handleManifest()
	} else {
		err := fmt.Errorf("unknown media type: %s", desc.MediaType)
		getLog(zerolog.ErrorLevel).
			AnErr("err", err)
		return nil, err
	}

}

func handlePod(ctx *context.Context, pod *v1.Pod, clientset kubernetes.Interface) error {
	name := pod.Name
	podClient := clientset.CoreV1().Pods(pod.Namespace)

	getPodLog := func(level zerolog.Level) *zerolog.Event {
		return log.WithLevel(level).
			Str("pod-name", name).
			Int("num_containers", len(pod.Spec.Containers))
	}
	getPodLog(zerolog.InfoLevel).
		Msg("Got pod")

	architectureLists := make([][]string, 0)
	for _, container := range pod.Spec.Containers {
		getContainerLog := func(level zerolog.Level) *zerolog.Event {
			return log.WithLevel(level).
				Str("container-name", container.Name).
				Str("container-image", container.Image)
		}
		getContainerLog(zerolog.DebugLevel).
			Msg("Got container")

		architectures, err := getArchitectures(ctx, container.Image)
		if err != nil {
			getContainerLog(zerolog.ErrorLevel).
				AnErr("err", err).
				Msg("Unable to get architectures for container")
			return err
		}
		architectureLists = append(
			architectureLists,
			architectures,
		)
	}
	architectures := Intersection(architectureLists...)
	getPodLog(zerolog.InfoLevel).
		Str("architectures", strings.Join(architectures, ", ")).
		Msg("Got intersection of architectures for pod")

	missingArchMap := make(map[string]bool)
	for _, arch := range architectures {
		missingArchMap[arch] = true
	}

	missingArchs := len(architectures)
	for _, tol := range pod.Spec.Tolerations {
		if tol.Key == ARCH_TAINT_KEY_NAME {
			if _, ok := missingArchMap[tol.Value]; ok {
				// Mark this toleration as already present
				missingArchMap[tol.Value] = false
				missingArchs -= 1
			}
		}
	}

	if missingArchs == 0 {
		getPodLog(zerolog.InfoLevel).
			Msg("Pod tolerations up to date, doing nothing")
		return nil
	}

	attemptCounter := 0
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		attemptCounter += 1
		result, getErr := podClient.Get(
			*ctx,
			name,
			metav1.GetOptions{},
		)
		if getErr != nil {
			getPodLog(zerolog.WarnLevel).
				Msg("Unable to get latest information on pod")
			return getErr
		}

		getPodLog(zerolog.DebugLevel).
			Interface("pod-tols", result.Spec.Tolerations).
			Msg("pod's current tolerations before update")

		// Add missing tolerations
		for arch, toAdd := range missingArchMap {
			if !toAdd {
				continue
			}
			result.Spec.Tolerations = append(
				result.Spec.Tolerations,
				v1.Toleration{
					Key:    ARCH_TAINT_KEY_NAME,
					Value:  arch,
					Effect: v1.TaintEffectNoSchedule,
				},
			)
		}

		getPodLog(zerolog.DebugLevel).
			Interface("pod-tols", result.Spec.Tolerations).
			Msg("Applying the following tolerations")

		_, updateErr := podClient.Update(
			*ctx,
			result,
			metav1.UpdateOptions{},
		)
		if updateErr != nil {
			getPodLog(zerolog.WarnLevel).
				AnErr("err", updateErr).
				Msg("Unable to update tolerations on pod")
			return updateErr
		}

		getPodLog(zerolog.InfoLevel).
			Int("attempts", attemptCounter).
			Msg("Added tolerations onto pod")
		return nil
	})

	if retryErr != nil {
		getPodLog(zerolog.WarnLevel).
			Int("attempts", attemptCounter).
			AnErr("err", retryErr).
			Msg("Unable to update tolerations on pod")
	}

	return nil
}

func EnsurePodTolerations(ctx *context.Context) {
	ticker := time.NewTicker(RECONCILIATION_INTERVAL)

	clientset := GetK8sInterface(ctx)
	podClient := clientset.CoreV1().Pods("")
	podWatch, err := podClient.Watch(
		*ctx,
		metav1.ListOptions{},
	)
	if err != nil {
		log.Error().
			AnErr("err", err).
			Msg("Unable to watch pods")
		return
	}

	handlePodWrapper := func(pod *v1.Pod, wg *sync.WaitGroup) {
		if wg != nil {
			defer wg.Done()
		}

		RetryOnError(
			ctx,
			func() error {
				return handlePod(ctx, pod, clientset)
			},
		)
	}

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
				continue
			}
			pod, ok := podEvent.Object.(*v1.Pod)

			if !ok {
				log.Error().
					Interface("podEvent", podEvent).
					Msg("Unable to handle event, cannot extract pod")
				continue
			}

			log.Info().
				Str("event", string(podEvent.Type)).
				Str("pod", pod.Name).
				Msg("Responding to pod event")
			go handlePodWrapper(pod, nil)
		case <-ticker.C:
			ticker.Stop()
			var wg sync.WaitGroup

			nodeList, err := podClient.List(*ctx, metav1.ListOptions{})
			if err != nil {
				log.Warn().
					AnErr("err", err).
					Msg("Error while listing pods")
				continue
			}

			log.Info().
				Msg("Reconciling pods...")
			for _, node := range nodeList.Items {
				wg.Add(1)
				go handlePodWrapper(&node, &wg)
			}
			wg.Wait()

			log.Info().
				Msg("Finished reconciling pods!")
			ticker.Reset(RECONCILIATION_INTERVAL)
		}
	}
}
