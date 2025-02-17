# kubelet-meta-proxy
kubelet-meta-proxy is a lightweight service designed to fetch and augment metrics from a Kubernetes node’s kubelet. Specifically, it reads kubelet metrics and can inject additional metadata—such as custom labels—before re-exposing those metrics on a separate local endpoint. This setup makes it easier to integrate enriched kubelet data into your existing observability stack without requiring direct access to the kubelet or modifying your core monitoring pipeline.

Metrics can be collected either directly from the kubelet or via the kube-apiserver.

```
go run cmd/main.go -node-port=443 -node-name-or-ip=nodeIPOrName -kube-apiserver=kubeApiServerIPorDNSName
```

If the `-kube-apiserver` flag is specified, metrics will be collected through the kube-apiserver.

kubelet-meta-proxy is not a production-grade solution. Use it in production at your own discretion and risk. The main purpose of this project is to demonstrate an alternative approach to label enrichment: rather than relying on recording rules or metric joins, you can use a proxy that injects additional labels into kubelet metrics directly. This approach can greatly simplify multi-tenant alerting in Kubernetes clusters, allowing you to generate alerts based on organizational or team-specific labels without complex rule configurations.

Below is an example of how you might configure Alertmanager to route alerts based on a custom label—in this case, `team: frontend`. When **kubelet-meta-proxy** enriches all kubelet metrics with `team: frontend`, any alerts fired with that label will be routed according to the rules defined under the `route:` section.

```yaml
# alertmanager.yaml (partial example)
route:
  # Default receiver if no other routes match.
  receiver: default

  # Route definitions:
  routes:
    # All alerts that have the label team=frontend
    # will be routed to the "frontend-team" receiver.
    - match:
        team: "frontend"
      receiver: "frontend-team"

    # If you had another team or default behavior, you could add them here:
    # - match:
    #     team: "backend"
    #   receiver: "backend-team"

receivers:
  # The default receiver (if no match is found).
  - name: "default"
    # Slack or email or other integration config
    slack_configs:
      - channel: "#general"
        send_resolved: true
        text: "Default alerts"

  # A receiver specifically for the 'frontend' team.
  - name: "frontend-team"
    slack_configs:
      - channel: "#frontend-alerts"
        send_resolved: true
        text: "Frontend alerts have fired"
```

### How It Works
1. **Metrics Enrichment**: When your `Namespace` has the label `team: frontend`, **kubelet-meta-proxy** automatically enriches cAdvisor metrics with the label `team="frontend"`.
2. **Alert Rules**: Your Prometheus alerts (in the rule files) do **not** need extra joins or complicated label manipulations; they simply propagate the `team` label from the metrics into the firing alerts.
3. **Alertmanager Routing**: Alertmanager sees the `team="frontend"` label in the alert and matches it against the route with `match: {team: "frontend"}`, sending notifications to `frontend-team` (or whichever receiver you’ve configured).

This setup lets you cleanly separate alerts for each team, department, or environment based on a single label in the metrics.

## Description
Internally, kubelet-meta-proxy uses a Kubebuilder-based operator pattern, providing a controller manager that can be configured via several command-line flags. These flags include basic controller runtime settings (like --max-concurrency and --cache-sync-timeout), security options for serving metrics (--metrics-secure, TLS certificate paths), and parameters for customizing how metrics are fetched (--node-ip, --node-port, --node-cadvisor-path). Once deployed, the service polls the kubelet’s cAdvisor endpoint, processes and enriches the raw Prometheus metrics with any desired metadata—often linked to Kubernetes namespace labels—and then serves these transformed metrics locally (by default on port 8080). This approach simplifies adding namespace-level context to node-level metrics, enabling multi-tenant or more granular monitoring in Kubernetes clusters.

## Getting Started

### Prerequisites
- go version v1.23.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/kubelet-meta-proxy:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/kubelet-meta-proxy:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

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

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/kubelet-meta-proxy:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/kubelet-meta-proxy/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v1-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

