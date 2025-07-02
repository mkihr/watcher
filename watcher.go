package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/api/core/v1"
	"k8s.io/api/apps/v1"
)

type KubeClient interface {
	ListPods(ctx context.Context, ns string) ([]corev1.Pod, error)
	GetStatefulSet(ctx context.Context, ns, name string) (*appsv1.StatefulSet, error)
	UpdateStatefulSet(ctx context.Context, ns string, sts *appsv1.StatefulSet) error
}

type RealKubeClient struct {
	Client kubernetes.Interface
}

func (r *RealKubeClient) ListPods(ctx context.Context, ns string) ([]corev1.Pod, error) {
	list, err := r.Client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (r *RealKubeClient) GetStatefulSet(ctx context.Context, ns, name string) (*appsv1.StatefulSet, error) {
	return r.Client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
}

func (r *RealKubeClient) UpdateStatefulSet(ctx context.Context, ns string, sts *appsv1.StatefulSet) error {
	_, err := r.Client.AppsV1().StatefulSets(ns).Update(ctx, sts, metav1.UpdateOptions{})
	return err
}

// -- Refactored logic functions:

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

func restartStatefulSets(ctx context.Context, kc KubeClient, ns string, targets []string, debug bool) {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	for _, name := range targets {
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
	}
}

// -- Main loop, now using the above functions

func runWatcher(ctx context.Context, kc KubeClient, ns string, targets []string, sleepSeconds int, debug bool) {
	for {
		if debug {
			fmt.Println("[INFO] Checking pods in namespace:", ns)
		}
		pods, err := kc.ListPods(ctx, ns)
		if err != nil {
			fmt.Println("[ERROR] listing pods:", err)
			continue
		}

		if needsRestart(pods) {
			restartStatefulSets(ctx, kc, ns, targets, debug)
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
	runWatcher(ctx, kc, ns, targets, sleepSeconds, debug)
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
