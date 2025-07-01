package main

import (
	"os"
	"testing"

	"k8s.io/client-go/rest"
)

// Test getenv returns env var if set, otherwise fallback
func TestGetenv(t *testing.T) {
	const key = "TEST_ENV_KEY"
	os.Setenv(key, "value1")
	defer os.Unsetenv(key)

	if v := getenv(key, "fallback"); v != "value1" {
		t.Errorf("expected 'value1', got '%s'", v)
	}

	os.Unsetenv(key)
	if v := getenv(key, "fallback"); v != "fallback" {
		t.Errorf("expected 'fallback', got '%s'", v)
	}
}

// Test getKubeConfig returns in-cluster config if KUBECONFIG is not set
func TestGetKubeConfig_InCluster(t *testing.T) {
	os.Unsetenv("KUBECONFIG")
	orig := rest.InClusterConfig
	defer func() { rest.InClusterConfig = orig }()

	called := false
	rest.InClusterConfig = func() (*rest.Config, error) {
		called = true
		return &rest.Config{}, nil
	}

	_, err := getKubeConfig()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected InClusterConfig to be called")
	}
}

// Test getKubeConfig returns config from KUBECONFIG if set
func TestGetKubeConfig_KubeconfigEnv(t *testing.T) {
	os.Setenv("KUBECONFIG", "/fake/path")
	defer os.Unsetenv("KUBECONFIG")

	orig := rest.InClusterConfig
	origBuild := buildConfigFromFlags
	defer func() {
		rest.InClusterConfig = orig
		buildConfigFromFlags = origBuild
	}()

	called := false
	buildConfigFromFlags = func(_, kubeconfigPath string) (*rest.Config, error) {
		called = true
		if kubeconfigPath != "/fake/path" {
			t.Errorf("expected kubeconfig path '/fake/path', got '%s'", kubeconfigPath)
		}
		return &rest.Config{}, nil
	}

	_, err := getKubeConfig()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected BuildConfigFromFlags to be called")
	}
}

// Patch for clientcmd.BuildConfigFromFlags for test
var buildConfigFromFlags = func(masterUrl, kubeconfigPath string) (*rest.Config, error) {
	return clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
}

// Patch getKubeConfig to use testable buildConfigFromFlags
func getKubeConfig() (*rest.Config, error) {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return buildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}
