package main

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MockKubeClient implements the KubeClient interface for testing
type MockKubeClient struct {
	Pods                []corev1.Pod
	StatefulSets        map[string]*appsv1.StatefulSet
	UpdateCalledFor     []string
	GetStatefulSetError error
	UpdateError         error
}

func (m *MockKubeClient) ListPods(ctx context.Context, ns string) ([]corev1.Pod, error) {
	return m.Pods, nil
}
func (m *MockKubeClient) GetStatefulSet(ctx context.Context, ns, name string) (*appsv1.StatefulSet, error) {
	if m.GetStatefulSetError != nil {
		return nil, m.GetStatefulSetError
	}
	sts, ok := m.StatefulSets[name]
	if !ok {
		return nil, nil
	}
	return sts, nil
}
func (m *MockKubeClient) UpdateStatefulSet(ctx context.Context, ns string, sts *appsv1.StatefulSet) error {
	m.UpdateCalledFor = append(m.UpdateCalledFor, sts.Name)
	if m.UpdateError != nil {
		return m.UpdateError
	}
	m.StatefulSets[sts.Name] = sts
	return nil
}

// --- Unit Tests ---

func TestNeedsRestart(t *testing.T) {
	// OOMKilled pod
	pods := []corev1.Pod{
		{
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								Reason: "OOMKilled",
							},
						},
					},
				},
			},
		},
	}
	if !needsRestart(pods, true) {
		t.Error("Expected needsRestart to return true for OOMKilled pod")
	}

	// ExitCode 137 pod
	pods = []corev1.Pod{
		{
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						RestartCount: 1,
						LastTerminationState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: 137,
							},
						},
					},
				},
			},
		},
	}
	if !needsRestart(pods) {
		t.Error("Expected needsRestart to return true for ExitCode 137 pod")
	}

	// Healthy pod
	pods = []corev1.Pod{
		{
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						State:        corev1.ContainerState{},
						RestartCount: 0,
					},
				},
			},
		},
	}
	if needsRestart(pods) {
		t.Error("Expected needsRestart to return false for healthy pod")
	}
}

func TestRestartStatefulSets(t *testing.T) {
	mock := &MockKubeClient{
		StatefulSets: map[string]*appsv1.StatefulSet{
			"foo": {
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{},
						},
					},
				},
			},
		},
	}
	targets := []string{"foo"}
	ctx := context.Background()
	restartStatefulSets(ctx, mock, "default", targets, false)
	if len(mock.UpdateCalledFor) != 1 || mock.UpdateCalledFor[0] != "foo" {
		t.Errorf("Expected UpdateStatefulSet to be called for 'foo', got %v", mock.UpdateCalledFor)
	}
	// Check annotation
	sts := mock.StatefulSets["foo"]
	if sts.Spec.Template.Annotations["restartTimestamp"] == "" {
		t.Errorf("Expected restartTimestamp annotation to be set")
	}
}
