package main

import (
	"context"
	"fmt"
	"os"
	"testing"
)

func contains(slice []string, target string) bool {
	for _, value := range slice {
		if value == target {
			return true
		}
	}
	return false
}

func ensureGetArchWorksForIndex(t *testing.T, registry string, ns_var string) {
	ns := os.Getenv(ns_var)
	if ns == "" {
		t.Errorf("Need %s to be set", ns_var)
		t.FailNow()
	}

	index := fmt.Sprintf("%s/%s/archaware-testimg:index", registry, ns)

	ctx := context.Background()
	archs, err := getArchitectures(&ctx, index)

	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if len(archs) != 2 || !contains(archs, "amd64") || !contains(archs, "arm") {
		t.FailNow()
	}
}

func ensureArchWorksForManifest(t *testing.T, registry string, ns_var string) {
	ns := os.Getenv(ns_var)
	if ns == "" {
		t.Errorf("Need %s to be set", ns_var)
		t.FailNow()
	}

	makeManifest := func(tag string) string {
		return fmt.Sprintf("%s/%s/archaware-testimg:%s", registry, ns, tag)
	}
	manifestTests := []string{"amd64", "arm"}

	ctx := context.Background()
	var manifest string
	var archs []string
	var err error
	for _, arch := range manifestTests {
		manifest = makeManifest(arch)
		archs, err = getArchitectures(&ctx, manifest)
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		if len(archs) != 1 || !contains(archs, arch) {
			t.FailNow()
		}
	}
}

func TestEnsureGetArchWorksForDockerIndex(t *testing.T) {
	ensureGetArchWorksForIndex(t, "docker.io", "DOCKER_NS")
}

func TestEnsureGetArchWorksForOCIIndex(t *testing.T) {
	ensureGetArchWorksForIndex(t, "quay.io", "QUAY_NS")
}

func TestEnsureGetArchWorksForDockerManifest(t *testing.T) {
	ensureArchWorksForManifest(t, "docker.io", "DOCKER_NS")
}

func TestEnsureGetArchWorksForOCIManifest(t *testing.T) {
	ensureArchWorksForManifest(t, "quay.io", "QUAY_NS")
}
