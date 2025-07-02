package main

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Example function to test OOMKilled/restart detection logic
func TestNeedsRestart(t *testing.T) {
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
		t.Error("Expected needsRestart to return true for OOMKilled pod")
	}

	// Test with healthy pod
	pods = []corev1.Pod{
		{
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						State: corev1.ContainerState{},
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

// Example implementation you would need in watcher.go
func needsRestart(pods []corev1.Pod) bool {
	for _, pod := range pods {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Terminated != nil && cs.State.Terminated.Reason == "OOMKilled" {
				return true
			}
			if cs.RestartCount > 0 && cs.LastTerminationState.Terminated != nil &&
				cs.LastTerminationState.Terminated.ExitCode == 137 {
				return true
			}
		}
	}
	return false
}
