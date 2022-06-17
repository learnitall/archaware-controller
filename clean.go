package main

import (
	"context"

	"github.com/rs/zerolog/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

func Clean(ctx *context.Context, stop context.CancelFunc) {
	defer stop()
	clientset := GetK8sInterface(ctx)
	podClient := clientset.CoreV1().Pods("")
	nodeClient := clientset.CoreV1().Nodes()

	podList, err := podClient.List(*ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatal().
			AnErr("err", err).
			Msg("Unable to list pods")
	}

	nodeList, err := nodeClient.List(*ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatal().
			AnErr("err", err).
			Msg("Unable to list nodes")
	}

	for _, pod := range podList.Items {
		retry.RetryOnConflict(retry.DefaultRetry, func() error {
			nsPodClient := clientset.CoreV1().Pods(pod.Namespace)
			result, getErr := nsPodClient.Get(
				*ctx,
				pod.Name,
				metav1.GetOptions{},
			)
			if getErr != nil {
				log.Error().
					Str("pod", pod.Name).
					AnErr("err", getErr).
					Msg("Unable to get latest information on pod")
				return getErr
			}

			for {
				done := true
				for i, toleration := range result.Spec.Tolerations {
					if toleration.Key == ARCH_TAINT_KEY_NAME {
						done = false
						RemoveFromSlice(&result.Spec.Tolerations, i)
						break
					}
				}
				if done {
					break
				}
			}

			deleteErr := nsPodClient.Delete(
				*ctx,
				result.Name,
				metav1.DeleteOptions{},
			)
			if deleteErr != nil {
				log.Error().
					Str("pod", result.Name).
					AnErr("err", deleteErr).
					Msg("Unable to delete pod")
				return deleteErr
			}

			log.Info().
				Str("pod", result.Name).
				Msg("Mark for deletion")
			return nil
		})
	}

	for _, node := range nodeList.Items {
		retry.RetryOnConflict(retry.DefaultRetry, func() error {

			result, getErr := nodeClient.Get(
				*ctx,
				node.Name,
				metav1.GetOptions{},
			)
			if getErr != nil {
				log.Error().
					Str("node", node.Name).
					AnErr("err", getErr).
					Msg("Unable to get latest information on node")
				return getErr
			}

			for {
				done := true
				for i, taint := range result.Spec.Taints {
					if taint.Key == ARCH_TAINT_KEY_NAME {
						done = false
						RemoveFromSlice(&result.Spec.Taints, i)
						break
					}
				}
				if done {
					break
				}
			}

			_, updateErr := nodeClient.Update(
				*ctx,
				result,
				metav1.UpdateOptions{},
			)
			if updateErr != nil {
				log.Error().
					Str("node", result.Name).
					AnErr("err", updateErr).
					Msg("Unable to update node")
				return updateErr
			}

			log.Info().
				Str("node", result.Name).
				Msg("taints removed")
			return nil
		})
	}
}
