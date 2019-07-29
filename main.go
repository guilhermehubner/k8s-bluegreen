package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	apicorev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/kubernetes/typed/extensions/v1beta1"
	"k8s.io/client-go/tools/clientcmd"

	cli "gopkg.in/urfave/cli.v2"
)

const (
	blueSufix  = "--blue"
	greenSufix = "--green"

	greenVersion = "green"
	blueVersion  = "blue"
)

var (
	deploymentsClient v1beta1.DeploymentInterface
	servicesClient    corev1.ServiceInterface
)

func main() {
	app := cli.App{}
	app.Name = "K8s Blue&Green deploy"
	app.Usage = "A blue/green deploy implemetation with pure kubernetes"
	app.Version = "1.0.0"

	var (
		configFile    string
		serviceName   string
		newImage      string
		containerName string
		namespace     string
	)

	app.Commands = []*cli.Command{
		{
			Name: "deploy",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "config-file",
					Aliases:     []string{"f"},
					Usage:       "the .kube/config file path",
					Destination: &configFile,
				},
				&cli.StringFlag{
					Name:        "service",
					Aliases:     []string{"s"},
					Usage:       "the service name",
					Destination: &serviceName,
				},
				&cli.StringFlag{
					Name:        "image",
					Aliases:     []string{"i"},
					Usage:       "the new image for deployment",
					Destination: &newImage,
				},
				&cli.StringFlag{
					Name:        "container",
					Aliases:     []string{"c"},
					Usage:       "the name of container in deployment",
					Destination: &containerName,
				},
				&cli.StringFlag{
					Name:        "namespace",
					Aliases:     []string{"n"},
					Usage:       "the kubernetes namespace",
					Value:       "default",
					Destination: &namespace,
				},
			},
			Action: func(c *cli.Context) error {
				clientset, err := getClientset(configFile)
				if err != nil {
					return fmt.Errorf("fail to get kubernetes clientset: %v", err)
				}

				deploymentsClient = clientset.ExtensionsV1beta1().Deployments(namespace)
				servicesClient = clientset.CoreV1().Services(namespace)

				return deploy(serviceName, newImage, containerName)
			},
		},
		{
			Name: "rollback",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "config-file",
					Aliases:     []string{"f"},
					Usage:       "the .kube/config file path",
					Destination: &configFile,
				},
				&cli.StringFlag{
					Name:        "service",
					Aliases:     []string{"s"},
					Usage:       "the service name",
					Destination: &serviceName,
				},
				&cli.StringFlag{
					Name:        "namespace",
					Aliases:     []string{"n"},
					Usage:       "the kubernetes namespace",
					Value:       "default",
					Destination: &namespace,
				},
			},
			Action: func(c *cli.Context) error {
				clientset, err := getClientset(configFile)
				if err != nil {
					return fmt.Errorf("fail to get kubernetes clientset: %v", err)
				}

				deploymentsClient = clientset.ExtensionsV1beta1().Deployments(namespace)
				servicesClient = clientset.CoreV1().Services(namespace)

				return rollback(serviceName, newImage, containerName)
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func deploy(serviceName, newImage, containerName string) error {
	fmt.Printf("Getting the service: %s\n", serviceName)
	service, err := servicesClient.Get(serviceName, v1.GetOptions{})
	if err != nil {
		return err
	}

	selector := ""
	for k, v := range service.Spec.Selector {
		selector += fmt.Sprintf("%s=%s,", k, v)
	}
	selector = strings.TrimSuffix(selector, ",")

	fmt.Println("Getting the deployments")
	deployments, err := deploymentsClient.List(v1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return err
	}

	if len(deployments.Items) == 0 {
		return fmt.Errorf("Deployment not found")
	}

	actualDeploy := deployments.Items[0]

	isBlue := strings.HasSuffix(actualDeploy.Name, blueSufix)

	fmt.Println("Creating new deployment")
	err = createNewDeployments(actualDeploy.Name, containerName, newImage, isBlue)
	if err != nil {
		return err
	}

	fmt.Println("Point service to new deployment")
	servicePointsToNewDeployment(service, actualDeploy.Labels["version"], actualDeploy.Name)
	if err != nil {
		return err
	}

	fmt.Println("Scaling down backup deployment")
	scale, err := deploymentsClient.GetScale(actualDeploy.Name, v1.GetOptions{})
	if err != nil {
		return err
	}

	scale.Spec.Replicas = 0
	_, err = deploymentsClient.UpdateScale(actualDeploy.Name, scale)
	return err
}

func rollback(serviceName, newImage, containerName string) error {
	fmt.Printf("Getting the service: %s\n", serviceName)
	service, err := servicesClient.Get(serviceName, v1.GetOptions{})
	if err != nil {
		return err
	}

	selector := ""
	for k, v := range service.Spec.Selector {
		selector += fmt.Sprintf("%s=%s,", k, v)
	}
	selector = strings.TrimSuffix(selector, ",")

	fmt.Println("Getting the actual deployment")
	deployments, err := deploymentsClient.List(v1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return err
	}

	if len(deployments.Items) == 0 {
		return fmt.Errorf("Deployment not found")
	}

	actualDeploy := deployments.Items[0]

	fmt.Println("Getting the old deployment")
	oldDeployment, ok := service.Labels["olddeployment"]
	if !ok {
		return fmt.Errorf("fail to rollback. cannot find old deployment")
	}

	deployment, err := deploymentsClient.Get(oldDeployment, v1.GetOptions{})
	if err != nil {
		return err
	}

	fmt.Println("Scaling the old deployment")
	scale, err := deploymentsClient.GetScale(deployment.Name, v1.GetOptions{})
	if err != nil {
		return err
	}

	scale.Spec.Replicas = *actualDeploy.Spec.Replicas
	_, err = deploymentsClient.UpdateScale(deployment.Name, scale)
	if err != nil {
		return err
	}

	if err := checkDeployment(deployment.Name); err != nil {
		return err
	}

	fmt.Println("Pointing service to the old deployment")
	service.Spec.Selector = deployment.Spec.Template.Labels
	delete(service.Labels, "olddeployment")

	_, err = servicesClient.Update(service)
	if err != nil {
		return err
	}

	fmt.Println("Scaling down deployment")
	scale, err = deploymentsClient.GetScale(actualDeploy.Name, v1.GetOptions{})
	if err != nil {
		return err
	}

	scale.Spec.Replicas = 0
	_, err = deploymentsClient.UpdateScale(actualDeploy.Name, scale)
	return err
}

func getClientset(configFile string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", configFile)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

func createNewDeployments(deploymentName, containerName, newImage string, isBlue bool) error {
	copyOfActualDeploy, err := deploymentsClient.Get(deploymentName, v1.GetOptions{})
	if err != nil {
		return err
	}

	if isBlue {
		copyOfActualDeploy.Name = strings.Split(deploymentName, blueSufix)[0] + greenSufix

		copyOfActualDeploy.Labels["version"] = greenVersion
		copyOfActualDeploy.Spec.Template.Labels["version"] = greenVersion
		v1.AddLabelToSelector(copyOfActualDeploy.Spec.Selector, "version", greenVersion)
	} else {
		copyOfActualDeploy.Name = strings.Split(deploymentName, greenSufix)[0] + blueSufix

		copyOfActualDeploy.Labels["version"] = blueVersion
		copyOfActualDeploy.Spec.Template.Labels["version"] = blueVersion
		v1.AddLabelToSelector(copyOfActualDeploy.Spec.Selector, "version", blueVersion)
	}

	copyOfActualDeploy.ResourceVersion = ""

	for i, container := range copyOfActualDeploy.Spec.Template.Spec.Containers {
		if container.Name == containerName {
			copyOfActualDeploy.Spec.Template.Spec.Containers[i].Image = newImage
		}
	}

	err = deploymentsClient.Delete(copyOfActualDeploy.Name, nil)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return err
	}

	deleted := false
	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)

		_, err := deploymentsClient.Get(copyOfActualDeploy.Name, v1.GetOptions{})
		if strings.Contains(err.Error(), "not found") {
			deleted = true
			break
		}
	}
	if !deleted {
		return fmt.Errorf("Couldn't create new deployment")
	}

	_, err = deploymentsClient.Create(copyOfActualDeploy)
	if err != nil {
		return err
	}

	return checkDeployment(copyOfActualDeploy.Name)
}

func servicePointsToNewDeployment(service *apicorev1.Service, oldVersion,
	oldDeployment string) error {
	if oldVersion == greenVersion {
		service.Spec.Selector["version"] = blueVersion
	} else if oldVersion == blueVersion {
		service.Spec.Selector["version"] = greenVersion
	} else {
		service.Spec.Selector["version"] = blueVersion
	}

	service.Labels["olddeployment"] = oldDeployment

	_, err := servicesClient.Update(service)
	return err
}

func checkDeployment(name string) error {
	deployment, err := deploymentsClient.Get(name, v1.GetOptions{})
	if err != nil {
		return err
	}

	if deployment.Status.Replicas == deployment.Status.AvailableReplicas {
		return nil
	}

	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)

		deployment, err := deploymentsClient.Get(name, v1.GetOptions{})
		if err != nil {
			continue
		}

		if deployment.Status.Replicas == deployment.Status.AvailableReplicas {
			return nil
		}
	}

	return fmt.Errorf("deployment %s didn't get up", name)
}
