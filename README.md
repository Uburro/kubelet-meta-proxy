# kubelet-meta-proxy
kubelet-meta-proxy is a lightweight service designed to fetch and augment metrics from a Kubernetes node’s kubelet. Specifically, it reads kubelet metrics and can inject additional metadata—such as custom labels—before re-exposing those metrics on a separate local endpoint. This setup makes it easier to integrate enriched kubelet data into your existing observability stack without requiring direct access to the kubelet or modifying your core monitoring pipeline.

Metrics can be collected either directly from the kubelet or via the kube-apiserver.

The main purpose of this project is to demonstrate an alternative approach to label enrichment: rather than relying on recording rules or metric joins, you can use a proxy that injects additional labels into kubelet metrics directly. This approach can greatly simplify multi-tenant alerting in Kubernetes clusters, allowing you to generate alerts based on organizational or team-specific labels without complex rule configurations.

Below is an extended explanation and example demonstrating how you might label multiple namespaces for the same team. This approach is useful when your organization has multiple services in separate namespaces but owned by the same team. If ownership changes, you can simply update the label in the namespace(s) to route metrics and alerts to the new team.

---

## Labeling Multiple Namespaces for the Same Team

You can assign the same `team` label to multiple namespaces in your cluster. For instance, if the **frontend** team owns two services—**frontend-service1** and **frontend-service2**—you could label both namespaces like so:

```yaml
---
apiVersion: v1
kind: Namespace
metadata:
  name: frontend-service1
  labels:
    team: "frontend"
---
apiVersion: v1
kind: Namespace
metadata:
  name: frontend-service2
  labels:
    team: "frontend"
```

Both namespaces now share the label `team: frontend`. Consequently:

1. **kubelet-meta-proxy** will see these labels and enrich the metrics from all pods in these namespaces with the label `team="frontend"`.  
2. Your Prometheus queries, rules, or Alertmanager routes that filter by `team="frontend"` will match **all** services within these namespaces.

### Changing Ownership by Updating Labels
If you need to reassign a namespace from the **frontend** team to the **backend** team, you only have to change the label on that namespace:

```bash
kubectl label namespace frontend-service2 team=backend --overwrite
```

This immediately redirects any future metrics to be labeled `team="backend"`, causing them to match the backend team’s alerts and routes instead of frontend’s.

---

## Example Alertmanager Configuration

Below is an example of how you might configure **Alertmanager** to route alerts based on a custom label—in this case, `team: frontend`. When **kubelet-meta-proxy** enriches all kubelet metrics with `team: frontend`, any alerts fired with that label will be routed according to the rules defined under the `route:` section.

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

1. **Metrics Enrichment**: When your **Namespace** has the label `team=frontend`, **kubelet-meta-proxy** automatically enriches kubelete metrics with the label `team="frontend"`.
2. **Alert Rules**: Your Prometheus alerts (in the rule files) do **not** need extra joins or complicated label manipulations; they simply propagate the `team` label from the metrics into the firing alerts.
3. **Alertmanager Routing**: Alertmanager sees the `team="frontend"` label in the alert and matches it against the route with `match: {team: "frontend"}`, sending notifications to `frontend-team` (or whichever receiver you’ve configured).

---

## Collecting Metrics via the Kube-apiserver

If you specify the `-kube-apiserver` flag, **kubelet-meta-proxy** will fetch metrics through the Kubernetes API server instead of directly from the kubelet:

```bash
go run cmd/main.go \
  -node-port=443 \
  -node-name-or-ip=nodeIPOrName \
  -kube-apiserver=kubeApiServerIPorDNSName
```

In this setup, you don’t need direct network connectivity to each node’s kubelet port. Instead, **kubelet-meta-proxy** uses the API server as a proxy for metrics retrieval, which can simplify network security considerations.

you might deploy kubelet-meta-proxy either as a DaemonSet (one pod per node) or as a Deployment (one or more replicas, typically behind a Service). Your choice depends on how you plan to collect metrics and your operational requirements:

Below are examples of how you might deploy **kubelet-meta-proxy** either as a **DaemonSet** (one pod per node) or as a **Deployment** (one or more replicas, typically behind a Service). Your choice depends on how you plan to collect metrics and your operational requirements:

1. **DaemonSet**:  
   - Ensures there is exactly one **kubelet-meta-proxy** pod running on each node.  
   - Ideal if you want to enrich metrics on each node separately or need local node access (e.g., via `hostNetwork` for direct kubelet connectivity).  
   - Useful in air-gapped or edge setups where each node runs an isolated copy of the proxy.

2. **Deployment**:  
   - Lets you run one or more **kubelet-meta-proxy** instances as normal pods, usually behind a **Service**.  
   - Good for centralized processing where direct per-node connectivity is less critical, or when fetching metrics via the **kube-apiserver**.

---

## Example DaemonSet

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: kubelet-meta-proxy
  labels:
    app: kubelet-meta-proxy
spec:
  selector:
    matchLabels:
      app: kubelet-meta-proxy
  updateStrategy:
    type: RollingUpdate
  template:
    metadata:
      labels:
        app: kubelet-meta-proxy
    spec:
      # Optional: run on every node in the cluster
      # using hostNetwork if you need direct connectivity.
      # hostNetwork: true
      # dnsPolicy: ClusterFirstWithHostNet
      containers:
        - name: kubelet-meta-proxy
          image: kubelet-meta-proxy:latest
          # Use args to pass your flags
          args:
            - "-node-port=443"
            - "-enable-http2=false"
            - "-kube-apiserver=$(KUBE_APISERVER)"
            - "-node-name-or-ip=$(NODE_NAME)"
            - "-metrics-port=8080"
          env:
            - name: NODE_NAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
            - name: KUBE_APISERVER
              value: "kubeApiServerIPorDNSName"  # set to the actual address if needed
          ports:
            - containerPort: 8080
              name: metrics
            # If using secure metrics (e.g., :8443)
            # - containerPort: 8443
            #   name: metrics-secure
          # securityContext and other fields go here if needed
      # (Optional) Node selector, tolerations, etc., if you want to limit where it runs
      # nodeSelector:
      #   kubernetes.io/os: linux
      # tolerations:
      #   - key: "node-role.kubernetes.io/master"
      #     effect: "NoSchedule"
```

### Notes on DaemonSet Deployment
1. **hostNetwork**: If you need direct connections to the host’s kubelet ports (e.g., 10250) without going through the API server, you can enable `hostNetwork: true` and typically set `dnsPolicy: ClusterFirstWithHostNet` in the pod spec.  
2. **NODE_NAME**: By referencing `spec.nodeName` through the Downward API, you can automatically discover each node's name, which **kubelet-meta-proxy** can use to connect to the local kubelet.  
3. **Security and RBAC**: Ensure the service account and RBAC rules allow the proxy to discover namespace labels (if you enrich from the apiserver) or read metrics from the kubelet.  

---

## Example Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubelet-meta-proxy
  labels:
    app: kubelet-meta-proxy
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kubelet-meta-proxy
  template:
    metadata:
      labels:
        app: kubelet-meta-proxy
    spec:
      containers:
        - name: kubelet-meta-proxy
          image: kubelet-meta-proxy:latest
          args:
            - "-node-port=443"
            - "-enable-http2=false"
            - "-kube-apiserver=kubeApiServerIPorDNSName"
            - "-node-name-or-ip=nodeIPOrName"
            - "-metrics-port=8080"
          ports:
            - containerPort: 8080
              name: metrics
      # Add your own node selectors, tolerations, affinity, etc. as needed
```

### Notes on Deployment
1. **Single or Multiple Replicas**: You can set `replicas: 1` for simplicity or increase the count for high availability.  
2. **Service**: Typically, you’d create a Service to expose this Deployment if you want a stable endpoint for Prometheus to scrape, especially in multi-replica scenarios.  
3. **API Server Connectivity**: If you’re using the `-kube-apiserver` flag, each replica only needs network access to the API server, **not** to every node’s kubelet port. This can simplify your cluster’s networking requirements and firewall rules.

---

## When to Use DaemonSet vs. Deployment

- **DaemonSet**:  
  - Collecting metrics directly from the kubelet on each node.  
  - Requires local connectivity (e.g., `hostNetwork`) to node-specific ports.  
  - Ensures uniform distribution—one instance per node.

- **Deployment**:  
  - Collecting metrics via the **kube-apiserver** (no direct node connectivity required).  
  - Use fewer pods for metrics aggregation.  
  - Easier to scale horizontally if you need more capacity.

---

By choosing the appropriate workload type (DaemonSet or Deployment) and combining it with the `-kube-apiserver` flag (if desired), you can tailor **kubelet-meta-proxy** to your infrastructure and security needs, whether that means running a local proxy on every node or a centralized pool of proxies behind a Service.

---

## Caveats

- **kubelet-meta-proxy** is **not a production-grade** solution. Use it in production at your own discretion and risk.
- The primary goal of this project is to showcase an alternative method for label enrichment at the metrics level, instead of relying on more complex Prometheus recording rules or joins.

This approach can greatly simplify **multi-tenant alerting** in Kubernetes clusters, allowing you to generate alerts based on organizational or team-specific labels without overly complicated rule configurations.

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

