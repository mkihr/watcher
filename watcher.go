package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// KubeClient defines an interface for interacting with Kubernetes resources.
// It provides methods to list pods, retrieve a StatefulSet, and update a StatefulSet
// within a specified namespace.
type KubeClient interface {
	ListPods(ctx context.Context, ns string) ([]corev1.Pod, error)
	GetStatefulSet(ctx context.Context, ns, name string) (*appsv1.StatefulSet, error)
	UpdateStatefulSet(ctx context.Context, ns string, sts *appsv1.StatefulSet) error
}

// RealKubeClient is a concrete implementation that wraps a Kubernetes client interface,
// providing methods to interact with a Kubernetes cluster.
type RealKubeClient struct {
	Client kubernetes.Interface
}

// ListPods retrieves the list of Pods in the specified namespace using the Kubernetes client.
// It returns a slice of corev1.Pod objects and an error if the operation fails.
//
// Parameters:
//
//	ctx - The context for controlling cancellation and deadlines.
//	ns  - The namespace from which to list the Pods.
//
// Returns:
//
//	[]corev1.Pod - A slice containing the Pods found in the specified namespace.
//	error        - An error if the list operation fails, otherwise nil.
func (r *RealKubeClient) ListPods(ctx context.Context, ns string) ([]corev1.Pod, error) {
	list, err := r.Client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// GetStatefulSet retrieves a StatefulSet resource by its namespace and name from the Kubernetes cluster.
// It returns the StatefulSet object if found, or an error if the retrieval fails.
//
// Parameters:
//
//	ctx  - The context for controlling cancellation and timeouts.
//	ns   - The namespace where the StatefulSet resides.
//	name - The name of the StatefulSet to retrieve.
//
// Returns:
//
//	*appsv1.StatefulSet - The retrieved StatefulSet object.
//	error               - An error if the StatefulSet could not be retrieved.
func (r *RealKubeClient) GetStatefulSet(ctx context.Context, ns, name string) (*appsv1.StatefulSet, error) {
	return r.Client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
}

// UpdateStatefulSet updates the specified StatefulSet resource in the given namespace using the Kubernetes client.
// It takes a context for request scoping, the namespace as a string, and a pointer to the StatefulSet object to update.
// Returns an error if the update operation fails.
func (r *RealKubeClient) UpdateStatefulSet(ctx context.Context, ns string, sts *appsv1.StatefulSet) error {
	_, err := r.Client.AppsV1().StatefulSets(ns).Update(ctx, sts, metav1.UpdateOptions{})
	return err
}

// -- Refactored logic functions:

// needsRestart checks a slice of Kubernetes pods to determine if any of their containers
// have been terminated due to an out-of-memory (OOMKilled) event or have previously exited
// with code 137 (commonly indicating an OOM kill). It returns true if any such condition
// is detected, indicating that a restart may be necessary.
func needsRestart(pods []corev1.Pod, debug bool) bool {
	for _, pod := range pods {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Terminated != nil {
				if debug {
					fmt.Printf("[DEBUG] Pod %s, Container %s: ExitCode=%d, Reason=%s\n",
						pod.Name, cs.Name, cs.State.Terminated.ExitCode, cs.State.Terminated.Reason)
				}
				if cs.State.Terminated.Reason == "OOMKilled" {
					return true
				}
			}
			if cs.RestartCount > 0 && cs.LastTerminationState.Terminated != nil {
				if debug {
					fmt.Printf("[DEBUG] Pod %s, Container %s: ExitCode=%d, Reason=%s\n",
						pod.Name, cs.Name, cs.State.Terminated.ExitCode, cs.State.Terminated.Reason)
				}
				return true
			}
		}
	}
	return false
}

// restartStatefulSets restarts the specified StatefulSets in the given namespace by updating their pod template
// annotations with a new "restartTimestamp". This triggers Kubernetes to perform a rolling restart of the pods.
// It uses the provided KubeClient interface to fetch and update StatefulSets. If debug is true, informational
// messages are printed to the console. Errors encountered during get or update operations are logged.
func restartStatefulSets(ctx context.Context, kc KubeClient, ns string, targets []string, delaySeconds int, debug bool) {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	for i, name := range targets {
		sts, err := kc.GetStatefulSet(ctx, ns, name)
		if err != nil {
			fmt.Println("[ERROR] get sts", name, ":", err)
			continue
		}

		if sts.Spec.Template.Annotations == nil {
			sts.Spec.Template.Annotations = map[string]string{}
		}
		sts.Spec.Template.Annotations["restartTimestamp"] = ts

		err = kc.UpdateStatefulSet(ctx, ns, sts)
		if err != nil {
			fmt.Println("[ERROR] update sts", name, ":", err)
		} else if debug {
			fmt.Println("[INFO] Restarted", name)
		}

		// Delay for 3 minutes after the first target before restarting others
		if i == 0 && len(targets) > 1 {
			if debug {
				fmt.Println("[INFO] Waiting ", delaySeconds, " seconds before restarting next StatefulSet...")
			}
			time.Sleep(time.Duration(delaySeconds))
		}
	}
}

// -- Main loop, now using the above functions

func runWatcher(ctx context.Context, kc KubeClient, ns string, targets []string, sleepSeconds int, delaySeconds int, debug bool) {
	for {
		if debug {
			fmt.Println("[INFO] Checking pods in namespace:", ns)
		}
		pods, err := kc.ListPods(ctx, ns)
		if err != nil {
			fmt.Println("[ERROR] listing pods:", err)
			continue
		}

		if needsRestart(pods, debug) {
			restartStatefulSets(ctx, kc, ns, targets, delaySeconds, debug)
		} else if debug {
			fmt.Println("[INFO] No restart needed.")
		}

		if debug {
			fmt.Printf("[INFO] Sleeping for %ds...\n", sleepSeconds)
		}
		time.Sleep(time.Duration(sleepSeconds) * time.Second)
	}
}

// -- Main entry point

func main() {
	ns := getenv("WATCH_NAMESPACE", "default")
	debug := getenv("DEBUG", "false") == "true"
	targets := strings.Split(os.Getenv("TARGET_STS"), ",")
	sleepSeconds, _ := strconv.Atoi(getenv("SLEEP_SECONDS", "30"))
	delaySeconds, _ := strconv.Atoi(getenv("RESTART_DELAY_SECONDS", "30"))

	config, err := getKubeConfig()
	if err != nil {
		panic(err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	kc := &RealKubeClient{Client: clientset}
	ctx := context.TODO()
	runWatcher(ctx, kc, ns, targets, sleepSeconds, delaySeconds, debug)
}

// -- Other helpers unchanged (getenv, getKubeConfig)
func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getKubeConfig() (*rest.Config, error) {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}
