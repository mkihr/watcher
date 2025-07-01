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
)

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

	for {
		if debug {
			fmt.Println("[INFO] Checking pods in namespace:", ns)
		}
		pods, err := clientset.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Println("[ERROR] listing pods:", err)
			continue
		}

		restart := false
		for _, pod := range pods.Items {
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.State.Terminated != nil && cs.State.Terminated.Reason == "OOMKilled" {
					if debug {
						fmt.Println("[INFO] OOMKilled in pod:", pod.Name)
					}
					restart = true
					break
				}
				if cs.RestartCount > 0 && cs.LastTerminationState.Terminated != nil &&
					cs.LastTerminationState.Terminated.ExitCode == 137 {
					if debug {
						fmt.Println("[INFO] ExitCode 137 in pod:", pod.Name)
					}
					restart = true
					break
				}
			}
		}

		if restart {
			ts := fmt.Sprintf("%d", time.Now().Unix())
			for _, name := range targets {
				sts, err := clientset.AppsV1().StatefulSets(ns).Get(context.TODO(), name, metav1.GetOptions{})
				if err != nil {
					fmt.Println("[ERROR] get sts", name, ":", err)
					continue
				}

				if sts.Spec.Template.Annotations == nil {
					sts.Spec.Template.Annotations = map[string]string{}
				}
				sts.Spec.Template.Annotations["restartTimestamp"] = ts

				_, err = clientset.AppsV1().StatefulSets(ns).Update(context.TODO(), sts, metav1.UpdateOptions{})
				if err != nil {
					fmt.Println("[ERROR] update sts", name, ":", err)
				} else if debug {
					fmt.Println("[INFO] Restarted", name)
				}
			}
		} else if debug {
			fmt.Println("[INFO] No restart needed.")
		}

		if debug {
			fmt.Printf("[INFO] Sleeping for %ds...", sleepSeconds)
		}
		time.Sleep(time.Duration(sleepSeconds) * time.Second)
	}
}

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
