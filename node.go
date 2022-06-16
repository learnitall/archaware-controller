package main

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	typedv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/util/retry"
)

func handleNode(ctx *context.Context, node *v1.Node, nodeClient typedv1.NodeInterface) error {
	name := node.ObjectMeta.Name
	arch := node.Status.NodeInfo.Architecture

	getLog := func(level zerolog.Level) *zerolog.Event {
		return log.WithLevel(level).
			Str("name", name).
			Str("arch", arch)
	}

	getLog(zerolog.InfoLevel).
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
			Interface("node-taints", result.Spec.Taints).
			Msg("node's current taints before update")

		for i, taint := range result.Spec.Taints {
			if taint.Key == ARCH_TAINT_KEY_NAME {
				if taint.Value == arch {
					getLog(zerolog.InfoLevel).
						Msg("Taint with proper architecture was found, doing nothing")
					return nil
				} else {
					getLog(zerolog.DebugLevel).
						Msg("Taint with bad architecture was found, updating")
					// Delete this taint as it needs to be updated
					RemoveFromSlice(&result.Spec.Taints, i)
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
				AnErr("err", updateErr).
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
			AnErr("err", retryErr).
			Msg("Unable to update architecture taint on node")
		return retryErr
	}
	return nil
}

func EnsureNodeTaints(ctx *context.Context) {
	ticker := time.NewTicker(RECONCILIATION_INTERVAL)

	clientset := GetK8sInterface(ctx)
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

	handleNodeWrapper := func(node *v1.Node, wg *sync.WaitGroup) {
		if wg != nil {
			defer wg.Done()
		}

		RetryOnError(
			ctx,
			func() error {
				return handleNode(ctx, node, nodeClient)
			},
		)
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
				continue
			}
			node, ok := nodeEvent.Object.(*v1.Node)

			if !ok {
				log.Error().
					Interface("nodeEvent", nodeEvent).
					Msg("Unable to handle event, cannot extract node")
				continue
			}

			log.Info().
				Str("event", string(nodeEvent.Type)).
				Str("node", node.Name).
				Msg("Responding to node event")
			go handleNodeWrapper(node, nil)
		case <-ticker.C:
			ticker.Stop()
			var wg sync.WaitGroup

			nodeList, err := nodeClient.List(*ctx, metav1.ListOptions{})
			if err != nil {
				log.Warn().
					AnErr("err", err).
					Msg("Error while listing nodes")
				continue
			}

			log.Info().
				Msg("Reconciling nodes...")
			for _, node := range nodeList.Items {
				wg.Add(1)
				go handleNodeWrapper(&node, &wg)
			}
			wg.Wait()

			log.Info().
				Msg("Finished reconciling nodes!")
			ticker.Reset(RECONCILIATION_INTERVAL)
		}
	}
}
