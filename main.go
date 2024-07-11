package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	// "github.com/aws/aws-sdk-go/aws/client"
	// "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	var client *kubernetes.Clientset
	var deploymentLabels map[string]string
	var expectedPods int32
	var err error
	if client, err = getClient(); err != nil {
		fmt.Printf("Error %s", err)
		os.Exit(1)
	}

	if deploymentLabels, expectedPods, err = deploy(context.Background(), *client); err != nil {
		fmt.Printf("Error %s", err)
		os.Exit(1)
	}

	if err = awaitPods(context.Background(), *client, deploymentLabels, expectedPods); err != nil {
		fmt.Printf("Error %s", err)
		os.Exit(1)
	}

	fmt.Printf("Finished deploying %s \n", deploymentLabels)
}

func getClient() (*kubernetes.Clientset, error) {

	var kubeconfig string
	// if home := homedir.HomeDir(); home != "" {
	// kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	// }
	// else {
	// 	kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	// }
	// flag.Parse()
	home := homedir.HomeDir()
	kubeconfig = filepath.Join(home, ".kube", "config")

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

func deploy(ctx context.Context, client kubernetes.Clientset) (map[string]string, int32, error) {
	var deployment *v1.Deployment
	appFile, err := os.ReadFile("deployment.yaml")
	if err != nil {
		return nil, 0, fmt.Errorf("readfile error: %v", err)
	}

	decode := scheme.Codecs.UniversalDeserializer().Decode
	obj, groupVersionKind, err := decode(appFile, nil, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("decode error: %v", err)
	}

	switch obj.(type) {
	case *v1.Deployment:
		deployment = obj.(*v1.Deployment)
	default:
		return nil, 0, fmt.Errorf("unrecognised type: %v", groupVersionKind)
	}

	_, err = client.AppsV1().Deployments("default").Get(ctx, deployment.Name, metav1.GetOptions{})
	if err != nil && errors.IsNotFound(err) {
		deploymentRespose, err := client.AppsV1().Deployments("default").Create(ctx, deployment, metav1.CreateOptions{})
		if err != nil {
			return nil, 0, fmt.Errorf("deployment error: %v", err)
		}
		var labels = deploymentRespose.Spec.Template.Labels
		return labels, 0, err
	} else if err != nil && !errors.IsNotFound(err) {
		return nil, 0, fmt.Errorf("deployment error: %v", err)
	}

	deploymentRespose, err := client.AppsV1().Deployments("default").Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("deployment error: %v", err)
	}
	var labels = deploymentRespose.Spec.Template.Labels

	return labels, *deployment.Spec.Replicas, err
}

func awaitPods(ctx context.Context, client kubernetes.Clientset, deploymentLabelslabels map[string]string, expectedPods int32) error {
	for {
		validatedLabels, err := labels.ValidatedSelectorFromSet(deploymentLabelslabels)
		if err != nil {
			return fmt.Errorf("validatedSelectorFromSet error: %v", err)
		}

		podCount, err := client.CoreV1().Pods("default").List(ctx, metav1.ListOptions{LabelSelector: validatedLabels.String()})
		if err != nil {
			return fmt.Errorf("pod count error: %v", err)
		}

		runningPods := 0

		for _, pod := range podCount.Items {
			if pod.Status.Phase == "Running" {
				runningPods++
			}
		}

		fmt.Printf("waiting for pods to become ready (running %v / %v) \n", runningPods, len(podCount.Items))
		if runningPods > 0 && runningPods == len(podCount.Items) && runningPods == int(expectedPods) {
			break
		}

		time.Sleep(5 * time.Second)

	}
	return nil
}
