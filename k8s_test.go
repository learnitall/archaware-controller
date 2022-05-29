package main

import (
	"context"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func CreateK8sClientsetForTesting() kubernetes.Interface {
	return fake.NewSimpleClientset()
}

// TestGetK8sClientsetReturnsNilWhenNoClientsetIsSet ensures that this function
// can be called before WithK8sClientset
func TestGetK8sClientsetReturnsNilWhenNoClientsetIsSet(t *testing.T) {
	ctx := context.Background()
	defer func() {
		if err := recover(); err != nil {
			t.Errorf("Got an unexpected error: %s", err)
		}
	}()
	result := GetK8sClientset(ctx)
	if result != nil {
		t.Fail()
	}
}

// TestCanSToreClientsetInContext ensures that a kubernetes clientset can be
// stored within a context
func TestCanStoreClientsetInContext(t *testing.T) {
	ctx := context.Background()
	cs := CreateK8sClientsetForTesting()
	ctx = WithK8sClientset(ctx, cs)
	csReturned := GetK8sClientset(ctx)
	if csReturned == nil {
		t.Fail()
	}
}
