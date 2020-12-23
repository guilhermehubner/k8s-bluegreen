<img width="300" src="https://raw.githubusercontent.com/guilhermehubner/k8s-bluegreen/master/logo.png">

# Kubernetes Blue Green Deploy
[![Github Actions](https://github.com/guilhermehubner/k8s-bluegreen/workflows/tests/badge.svg)](https://github.com/guilhermehubner/k8s-bluegreen/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/guilhermehubner/k8s-bluegreen)](https://goreportcard.com/report/github.com/guilhermehubner/k8s-bluegreen)
[![GoDoc](https://godoc.org/github.com/guilhermehubner/k8s-bluegreen?status.svg)](https://godoc.org/github.com/guilhermehubner/k8s-bluegreen)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](https://opensource.org/licenses/MIT)

A blue/green deploy implementation with pure Kubernetes.
This will atomically update a deployment image using a service and its labels.

The deploy steps will consist in:
1. Gets the service received as a parameter;
2. Gets the deployment the service is selecting;
4. Creates a new deployment, exactly as the old one, but with the new image and an opposite
blue/green suffix (if the current deployment has no blue/green suffix, the next one will be
blue);
5. Points the service to the new deployment;
6. Scales old deployment down to 0 replicas.

## Installation

```
go get -u github.com/guilhermehubner/k8s-bluegreen
```

## Credentials

You can connect to the cluster with a config file (`-f .kube/config`) or using environment variables. For example:
```
export KUBERNETES_SERVER=https://1.2.3.4:1234
export KUBERNETES_CERT=$(cat ca.cert)
export KUBERNETES_TOKEN=eyJhcGciDiJS...
```
 
## Usage

### Deploy
```
k8s-bluegreen deploy [OPTIONS]

OPTIONS:
   --config-file value, -f value  the kubernetes config file path (default: "~/.kube/config")
   --service value, -s value      the service name
   --image value, -i value        the new image for deployment
   --container value, -c value    the name of container in deployment
   --namespace value, -n value    the kubernetes namespace (default: "default")
   --help, -h                     show help (default: false)
```

### Rollback
```
k8s-bluegreen rollback [OPTIONS]

OPTIONS:
   --config-file value, -f value  the kubernetes config file path (default: "~/.kube/config")
   --service value, -s value      the service name
   --namespace value, -n value    the kubernetes namespace (default: "default")
   --help, -h                     show help (default: false)
```
