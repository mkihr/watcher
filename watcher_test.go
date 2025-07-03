package main

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// --- Mock KubeClient for testing ---

type mockKubeClient struct {
	Pods         []corev1.Pod
	StatefulSets map[string]*appsv1.StatefulSet
	UpdateErr    error
	GetErr       error
	UpdatedSts   []*appsv1.StatefulSet
}

func (m *mockKubeClient) ListPods(ctx context.Context, ns string) ([]corev1.Pod, error) {
	return m.Pods, nil
}
func (m *mockKubeClient) GetStatefulSet(ctx context.Context, ns, name string) (*appsv1.StatefulSet, error) {
	if m.GetErr != nil {
		return nil, m.GetErr
	}
	sts, ok := m.StatefulSets[name]
	if !ok {
		return nil, errors.New("not found")
	}
	return sts, nil
}
func (m *mockKubeClient) UpdateStatefulSet(ctx context.Context, ns string, sts *appsv1.StatefulSet) error {
	if m.UpdateErr != nil {
		return m.UpdateErr
	}
	m.UpdatedSts = append(m.UpdatedSts, sts)
	return nil
}

// --- Tests for needsRestart ---

func TestNeedsRestart_OOMKilled(t *testing.T) {
	pods := []corev1.Pod{
		{
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: 137,
								Reason:   "OOMKilled",
							},
						},
					},
				},
			},
		},
	}
	if !needsRestart(pods, false) {
		t.Error("expected needsRestart to return true for OOMKilled")
	}
}

func TestNeedsRestart_ExitCode137(t *testing.T) {
	pods := []corev1.Pod{
		{
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						RestartCount: 1,
						LastTerminationState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: 137,
								Reason:   "Error",
							},
						},
					},
				},
			},
		},
	}
	if !needsRestart(pods, false) {
		t.Error("expected needsRestart to return true for ExitCode 137")
	}
}

func TestNeedsRestart_NoRestartNeeded(t *testing.T) {
	pods := []corev1.Pod{
		{
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
					},
				},
			},
		},
	}
	if needsRestart(pods, false) {
		t.Error("expected needsRestart to return false when no OOMKilled or 137 exit code")
	}
}

// --- Tests for restartStatefulSets ---

func TestRestartStatefulSets_UpdatesAnnotations(t *testing.T) {
	ctx := context.Background()
	sts1 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "sts1"},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
		},
	}
	sts2 := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "sts2"},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
			},
		},
	}
	mkc := &mockKubeClient{
		StatefulSets: map[string]*appsv1.StatefulSet{
			"sts1": sts1,
			"sts2": sts2,
		},
	}
	targets := []string{"sts1", "sts2"}
	start := time.Now().Unix()
	restartStatefulSets(ctx, mkc, "default", targets, 0, false)
	if len(mkc.UpdatedSts) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(mkc.UpdatedSts))
	}
	for _, updated := range mkc.UpdatedSts {
		val, ok := updated.Spec.Template.Annotations["restartTimestamp"]
		if !ok {
			t.Errorf("restartTimestamp annotation missing for %s", updated.Name)
		}
		ts, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			t.Errorf("restartTimestamp annotation not an int for %s", updated.Name)
		}
		if ts < start {
			t.Errorf("restartTimestamp annotation not updated for %s", updated.Name)
		}
	}
}

func TestRestartStatefulSets_GetError(t *testing.T) {
	ctx := context.Background()
	mkc := &mockKubeClient{
		StatefulSets: map[string]*appsv1.StatefulSet{},
		GetErr:       errors.New("get error"),
	}
	targets := []string{"sts1"}
	restartStatefulSets(ctx, mkc, "default", targets, 0, false)
	if len(mkc.UpdatedSts) != 0 {
		t.Error("should not update when GetStatefulSet returns error")
	}
}

func TestRestartStatefulSets_UpdateError(t *testing.T) {
	ctx := context.Background()
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "sts1"},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
		},
	}
	mkc := &mockKubeClient{
		StatefulSets: map[string]*appsv1.StatefulSet{"sts1": sts},
		UpdateErr:    errors.New("update error"),
	}
	targets := []string{"sts1"}
	restartStatefulSets(ctx, mkc, "default", targets, 0, false)
	// Should attempt update, but error is ignored in logic
}

// --- Test getenv helper ---

func TestGetenvFallback(t *testing.T) {
	key := "TEST_ENV_NOT_SET"
	val := getenv(key, "fallback")
	if val != "fallback" {
		t.Errorf("expected fallback, got %s", val)
	}
}
