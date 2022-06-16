package main

import (
	"context"
	"math"
	"reflect"
	"runtime"
	"time"

	"github.com/rs/zerolog/log"
)

// RetryOnError attempts to execute the given function, retrying when it returns an error.
// Uses binary exponential backoff to determine how long to wait before retrying calling the function.
func RetryOnError(ctx *context.Context, callable func() error) (int, error) {

	callable_name := runtime.FuncForPC(
		reflect.ValueOf(callable).Pointer(),
	).Name()

	var attempts int = 0
	var err error
	var backoff time.Duration

	get_backoff := func() time.Duration {
		return time.Duration(
			int(math.Pow(2, float64(attempts))),
		) * time.Second
	}

	for {
		select {
		case <-(*ctx).Done():
			return attempts, (*ctx).Err()
		default:
			attempts += 1
			err = callable()
			if err == nil {
				return attempts, nil
			}
			attempt_log := log.Warn().
				Str("callable", callable_name).
				Int("attempts", attempts)

			if attempts == MAX_RETRY_ATTEMPTS {
				attempt_log.Msg("Max attempts succeeded.")
				return attempts, err
			}
			backoff = get_backoff()
			attempt_log.
				AnErr("err", err).
				Int("backoff_seconds", int(backoff.Seconds())).
				Msg("Retrying after sleeping.")
			time.Sleep(backoff)
		}
	}
}

// Intersection finds the intersection between slices.
// Based on: https://siongui.github.io/2018/03/09/go-match-common-element-in-two-array/
func Intersection[T comparable](slices ...[]T) (intersection []T) {
	num_input_slices := len(slices)
	if num_input_slices == 0 {
		return make([]T, 0)
	} else if num_input_slices == 1 {
		return slices[0]
	}

	intersection_map := make(map[T]bool)
	for _, item := range slices[0] {
		intersection_map[item] = true
	}

	for _, slice := range slices[1:] {
		for _, item := range slice {
			if _, ok := intersection_map[item]; ok {
				intersection = append(intersection, item)
			}
		}
	}
	return
}

// RemoveFromSlice 'deletes' an item from the given slice.
// Item to remove is identified by its index.
// Does not preserve order of the slice.
func RemoveFromSlice[T interface{}](slice *[]T, target int) {
	sliceLen := len(*slice)
	(*slice)[target] = (*slice)[sliceLen-1]
	*slice = (*slice)[:sliceLen-1]
}
