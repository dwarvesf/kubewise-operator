# kubewise-operator
This project is designed to provide a Kubernetes operator framework that automates the deployment, management, and scaling of cloud cost optimization resources in a Kubernetes cluster.

## Description
Kubewise-operator helps users save costs by dynamically managing cloud resources related to Kubernetes workloads. This operator implements custom resource definitions (CRDs) to extend Kubernetes capabilities, making it easy to optimize workloads on various cloud providers.

## Features
- Automatic resource optimization based on historical usage metrics
- Configurable analysis intervals and cost-saving thresholds
- Support for ignoring specific resources (Deployments, StatefulSets, DaemonSets) from optimization
- Integration with Prometheus for metric collection
- Discord notifications for optimization recommendations

## Roadmap

### Implemented Features
- [x] Basic CloudCostOptimizer CRD definition
- [x] Resource analysis based on historical metrics
- [x] Configurable analysis intervals
- [x] Ignore specific resources (Deployments, StatefulSets, DaemonSets)
- [x] Prometheus integration for metric collection
- [x] Discord notifications for optimization recommendations

### Planned Features
- [] Cost-saving threshold configuration
- [ ] Machine learning-based predictive optimization
- [ ] Integration with cloud-specific cost management APIs
- [ ] User interface for visualization and manual control
- [ ] Multi-cluster support
- [ ] Custom metric support beyond Prometheus
- [ ] Automated rollback of optimizations if performance degrades
- [ ] Integration with other notification platforms (Slack, Email, etc.)
- [ ] Cost allocation and chargeback features
- [ ] Support for optimizing storage resources

## Project Structure
The project follows a standard Kubernetes operator structure:

- `api/v1alpha1/`: Contains the API definitions for the CloudCostOptimizer CRD
- `controllers/`: Contains the main logic for the operator
- `config/`: Contains Kubernetes manifests for deploying the operator
- `Dockerfile`: Defines the container image for the operator
- `main.go`: The entry point of the operator

Key files:
- `api/v1alpha1/cloudcostoptimizer_types.go`: Defines the CloudCostOptimizer CRD
- `controllers/cloudcostoptimizer_controller.go`: Main reconciliation loop
- `controllers/cloudcostoptimizer_analyze.go`: Resource analysis and optimization logic
- `config/samples/optimization_v1alpha1_cloudcostoptimizer.yaml`: Sample CloudCostOptimizer resource

## Getting Started

### Prerequisites
- Go version v1.21.0+ (https://golang.org/doc/install)
- Docker version 17.03+ (https://docs.docker.com/get-docker/)
- kubectl version v1.11.3+ (https://kubernetes.io/docs/tasks/tools/)
- Access to a Kubernetes v1.11.3+ cluster
- Kubernetes cluster with Prometheus installed (for metric collection)
- kustomize v3.8.7+ (https://kustomize.io/)
- Operator SDK v1.28.0+ (https://sdk.operatorframework.io/docs/installation/)
- A container registry account (e.g., Docker Hub) to store the operator image

Optional:
- Discord webhook URL (for notifications)

### Development Environment Setup
1. Install Go, Docker, kubectl, and kustomize using the links provided above.
2. Install the Operator SDK:
   ```sh
   export ARCH=$(case $(uname -m) in x86_64) echo -n amd64 ;; aarch64) echo -n arm64 ;; *) echo -n $(uname -m) ;; esac)
   export OS=$(uname | awk '{print tolower($0)}')
   export OPERATOR_SDK_DL_URL=https://github.com/operator-framework/operator-sdk/releases/download/v1.28.0
   curl -LO ${OPERATOR_SDK_DL_URL}/operator-sdk_${OS}_${ARCH}
   chmod +x operator-sdk_${OS}_${ARCH} && sudo mv operator-sdk_${OS}_${ARCH} /usr/local/bin/operator-sdk
   ```
3. Clone the kubewise-operator repository:
   ```sh
   git clone https://github.com/your-org/kubewise-operator.git
   cd kubewise-operator
   ```

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/kubewise-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don't work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/kubewise-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### Configuration

The CloudCostOptimizer custom resource allows you to configure various aspects of the optimization process. Here's an example configuration:

```yaml
apiVersion: optimization.dwarvesf.com/v1alpha1
kind: CloudCostOptimizer
metadata:
  name: cloudcostoptimizer-sample
spec:
  analysisInterval: "1h"
  targets:
    - resources: ["pods"]
      namespaces: ["default", "kube-system"]
      automateOptimization: false
      ignoreResources:
        deployment: ["important-deployment", "critical-app"]
        statefulSet: ["database"]
        daemonSet: ["monitoring-agent"]
  costSavingThreshold: 10
  prometheusConfig:
    serverAddress: "http://prometheus-server.monitoring"
    historicalMetricDuration: 6h
  discordConfig:
    webhookURL: "https://discord.com/api/webhooks/your-webhook-url"
```

This configuration sets up the CloudCostOptimizer to:
- Analyze resources every hour
- Target pods in the "default" and "kube-system" namespaces
- Ignore specific deployments, statefulsets, and daemonsets
- Set a cost-saving threshold of 10%
- Use Prometheus for historical metrics
- Send notifications to Discord

#### ArgoCD Integration

When using the `automateOptimization` feature with ArgoCD, you need to configure ArgoCD to ignore the changes made by the CloudCostOptimizer. This is because the operator will automatically adjust resource configurations, which may conflict with ArgoCD's sync process.

To configure ArgoCD to ignore these changes, add the following `ignoreDifferences` section to your Application or ApplicationSet:

```yaml
spec:
  ignoreDifferences:
    - group: apps
      kind: Deployment
      jsonPointers:
        - /spec/template/spec/containers/0/resources
```

This configuration tells ArgoCD to ignore differences in the container resources specification, which is where the CloudCostOptimizer makes its optimizations. You can adjust the `jsonPointers` as needed to match the specific resources you want ArgoCD to ignore.

By applying this configuration, you can ensure that ArgoCD and the CloudCostOptimizer work together harmoniously, allowing the operator to make resource optimizations without triggering unnecessary syncs or conflicts in ArgoCD.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/kubewise-operator:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.: 

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/kubewise-operator/<tag or branch>/dist/install.yaml
```

## Contributing
To contribute to this project, please follow these guidelines:
1. Fork the repository.
2. Create a feature branch.
3. Make your changes and commit them.
4. Push your branch to GitHub.
5. Create a pull request.

For detailed information on contribution processes and coding standards, please refer to the CONTRIBUTING.md file (to be created).

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
