# Kubernetes RDS Operator

[![Build status](https://badge.buildkite.com/e7c13388bb3657b571037c100c3c607604002ff4ab6e69df2c.svg?branch=master)](https://buildkite.com/myob/ops-kube-db-operator)

Operator to control RDS DBs in AWS, uses Config Maps for dafault configuration and Secrets for DB parameters.

## Installation

```bash
glide install
```

## Running from source

* Set required AWS config in the configmap

```bash
 kubectl apply -f yaml/config-map.yaml
```

* authenticate to kubes
* authenticate to AWS
* run it locally

```bash
go run *.go -kubeconfig ~/.kube/config
```

## Auto Generating Client with Kubernetes code-generator

* Make sure to `go get -d k8s.io/code-generator`
* If that is failing, try to get it from github.com/sttts/code-generator

```bash
❯ cd $GOPATH/src/k8s.io/code-generator
❯ ./generate-groups.sh all github.com/MYOB-Technology/ops-kube-db-operator/pkg/client github.com/MYOB-Technology/ops-kube-db-operator/pkg/apis "db:v1alpha1" --go-header-file ./hack/boilerplate.go.txt
Generating deepcopy funcs
Generating clientset for db:v1alpha1 at github.com/MYOB-Technology/ops-kube-db-operator/pkg/client/clientset
Generating listers for db:v1alpha1 at github.com/MYOB-Technology/ops-kube-db-operator/pkg/client/listers
Generating informers for db:v1alpha1 at github.com/MYOB-Technology/ops-kube-db-operator/pkg/client/informers
```

> If it is failing with the following, make sure to clone the latest version of github.com/kubernetes/gengo into the `vendor/k8s.io/gengo` folder
>
> ```bash
> cmd/client-gen/main.go:34:29: "k8s.io/code-generator/vendor/k8s.io/gengo/args".Default().WithoutDefaultFlagParsing undefined (type *"k8s.io/code-generator/vendor/k8s.io/gengo/args".GeneratorArgs has no field or method WithoutDefaultFlagParsing)
> ```
