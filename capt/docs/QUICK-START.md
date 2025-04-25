# Quick Start

In this tutorial we’ll cover the basics of how to use Cluster API to create one or more Kubernetes clusters.

## Installation

### Prerequisites

- Install and setup [kubectl] in your local environment
- Install [clusterctl]
- A running [Tinkerbell stack].

### Initialize the management cluster

Now that we've got clusterctl installed and all the prerequisites in place, let's transform the Kubernetes cluster
into a management cluster by using `clusterctl init`. This only needs to be run once. For this quick start, run this against the same
Kubernetes cluster where the Tinkerbell stack is installed.

The command accepts as input a list of providers to install; when executed for the first time, `clusterctl init`
automatically adds to the list the `cluster-api` core provider, and if unspecified, it also adds the `kubeadm` bootstrap
and `kubeadm` control-plane providers.

`clusterctl` doesn't include the Tinkerbell CAPI provider (CAPT) so we need to configure this:

```sh
cat >> ~/.cluster-api/clusterctl.yaml <<EOF
providers:
  - name: "tinkerbell"
    url: "https://github.com/tinkerbell/tinkerbell/capt/releases/v0.4.0/infrastructure-components.yaml"
    type: "InfrastructureProvider"
EOF

# Finally, initialize the management cluster
export TINKERBELL_IP=<hegel ip>
clusterctl init --infrastructure tinkerbell
```

The output of `clusterctl init` is similar to the following:

```shell
Fetching providers
Installing cert-manager Version="v1.10.1"
Waiting for cert-manager to be available...
Installing Provider="cluster-api" Version="v1.3.2" TargetNamespace="capi-system"
Installing Provider="bootstrap-kubeadm" Version="v1.3.2" TargetNamespace="capi-kubeadm-bootstrap-system"
Installing Provider="control-plane-kubeadm" Version="v1.3.2" TargetNamespace="capi-kubeadm-control-plane-system"
Installing Provider="infrastructure-tinkerbell" Version="v0.4.0" TargetNamespace="capt-system"

Your management cluster has been initialized successfully!

You can now create your first workload cluster by running the following:

  clusterctl generate cluster [name] --kubernetes-version [version] | kubectl apply -f -
```

### Create Hardware resources to make Tinkerbell Hardware available

Cluster API Provider Tinkerbell does not assume all hardware configured in Tinkerbell is available for provisioning.
To make Tinkerbell Hardware available create a `Hardware` resource in the management cluster.
For this quick start, the `hardware.tinkerbell.org` custom resource objects need to live in the same namespace as the Tink stack(specifically where `tink-controller` lives).

An example of a valid Hardware resource definition:

- Required metadata for hardware:
    - metadata.facility.facility_code is set (default is "onprem")
    - metadata.instance.id is set (should be a MAC address)
    - metadata.instance.hostname is set
    - metadata.spec.disks is set and contains at least one device matching an available disk on the system
  - An example of a valid hardware definition for use with the Tinkerbell provider:

    ```yaml
    apiVersion: "tinkerbell.org/v1alpha1"
    kind: Hardware
    metadata:
      name: node-1
      namespace: default
      labels: # Labels are optional, and can be used for machine selection later
        manufacturer: dell
        idrac-version: 8
        rack: 1
        room: 2
    spec:
      disks:
        - device: /dev/sda
      metadata:
        facility:
          facility_code: onprem
        instance:
          userdata: ""
          hostname: "node-1"
          id: "xx:xx:xx:xx:xx:xx"
          operating_system:
            distro: "ubuntu"
            os_slug: "ubuntu_20_04"
            version: "20.04"
      interfaces:
        - dhcp:
            arch: x86_64
            hostname: node-1
            ip:
              address: 0.0.0.0
              gateway: 0.0.0.1
              netmask: 255.255.255.0
            lease_time: 86400
            mac: xx:xx:xx:xx:xx
            name_servers:
              - 8.8.8.8
            uefi: true
          netboot:
            allowPXE: true
            allowWorkflow: true
      ```

**NOTE:** The name and id in each hardware YAML file will need to be unique.

### Create your first workload cluster

Once the management cluster is ready, you can create your first workload cluster.

#### Preparing the workload cluster configuration

The `clusterctl generate cluster` command returns a YAML template for creating a [workload cluster].

#### Required configuration for the Tinkerbell provider

Depending on the infrastructure provider you are planning to use, some additional prerequisites should be satisfied
before configuring a cluster with Cluster API. Instructions are provided for the Tinkerbell provider below.

Otherwise, you can look at the `clusterctl generate cluster` [command][clusterctl generate cluster] documentation for details about how to
discover the list of variables required by a cluster templates.

To see all required Tinkerbell environment variables execute:

```bash
clusterctl generate cluster --infrastructure tinkerbell --list-variables capi-quickstart
```

```bash
# Set CONTROL_PLANE_VIP to an available IP address for the network
# the machines will be provisioned on
export CONTROL_PLANE_VIP=192.168.1.110

# POD_CIDR is overridden here to avoid conflicting with the assumed
# Machine network of 192.168.1.0/24, this can be omitted if the
# Machine network does not conflict with the default of
# 192.168.0/0/16 or can be set to a different value if needed
export POD_CIDR=172.25.0.0/16

# SERVICE_CIDR can be overridden if the default of 172.26.0.0/16
# would interfere with the Machine network
#export SERVICE_CIDR=10.10.0.0/16
```

#### Generating the cluster configuration

For the purpose of this tutorial, we'll name our cluster capi-quickstart. The `--target-namespace` needs to be the namespace where the Tink stack is deployed. Otherwise you will see an error.

<details>
<summary>error message</summary>

```bash
Error from server (InternalError): error when creating "capi-quickstart.yaml": Internal error occurred: failed calling webhook "validation.tinkerbellcluster.infrastructure.cluster.x-k8s.io": failed to call webhook: Post "https://capt-webhook-service.capt-system.svc:443/validate-infrastructure-cluster-x-k8s-io-v1beta1-tinkerbellcluster?timeout=10s": EOF
```

</details>

```bash
clusterctl generate cluster capi-quickstart \
  --kubernetes-version v1.22.8 \
  --control-plane-machine-count=3 \
  --worker-machine-count=3 \
  --target-namespace=tink-system \
  > capi-quickstart.yaml
```

This creates a YAML file named `capi-quickstart.yaml` with a predefined list of Cluster API objects; Cluster, Machines,
Machine Deployments, etc.

The file can be eventually modified using your editor of choice.

See [clusterctl generate cluster] for more details.

#### Select your hardware

In the `capi-quickstart.yaml`, you'll see a `TinkerbellMachineTemplate` type where you can edit the `hardwareAffinity`
to enforce which CAPI machines will get provisioned on particular hardware.

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: TinkerbellMachineTemplate
metadata:
  name: capi-quickstart-md-0
  namespace: capt-system
spec:
  template:
    spec:
      hardwareAffinity:
        required:  # 'required' entries are OR'd
        - labelSelector:
            matchLabels:
              manufacturer: dell
            matchExpressions:
            - key: idracVersion
              operator: In
              values: ["7", "8"]
        preferred: # 'preferred' entries are sorted by weight and then sequentially evaluated
        - weight: 50
          hardwareAffinityTerm:
            labelSelector:
              matchLabels:
                rack: 1
                room: 2
```

#### Apply the workload cluster

When ready, run the following command to apply the cluster manifest.

```bash
kubectl apply -f capi-quickstart.yaml
```

The output is similar to this:

```bash
cluster.cluster.x-k8s.io/capi-quickstart created
tinkerbellcluster.infrastructure.cluster.x-k8s.io/capi-quickstart created
kubeadmcontrolplane.controlplane.cluster.x-k8s.io/capi-quickstart-control-plane created
tinkerbellmachinetemplate.infrastructure.cluster.x-k8s.io/capi-quickstart-control-plane created
machinedeployment.cluster.x-k8s.io/capi-quickstart-md-0 created
tinkerbellmachinetemplate.infrastructure.cluster.x-k8s.io/capi-quickstart-md-0 created
kubeadmconfigtemplate.bootstrap.cluster.x-k8s.io/capi-quickstart-md-0 created
```

#### Accessing the workload cluster

The cluster will now start provisioning. You can check status with:

```bash
kubectl get cluster
```

You can also get an "at glance" view of the cluster and its resources by running:

```bash
clusterctl describe cluster capi-quickstart
```

To verify the first control plane is up:

```bash
kubectl get kubeadmcontrolplane
```

You should see an output is similar to this:

```bash
NAME                            INITIALIZED   API SERVER AVAILABLE   VERSION   REPLICAS   READY   UPDATED   UNAVAILABLE
capi-quickstart-control-plane   true                                 v1.22.0   3                  3         3
```

**NOTE** The control plane won't be `Ready` until we install a CNI in the next step.

After the first control plane node is up and running, we can retrieve the [workload cluster] Kubeconfig:

```bash
clusterctl get kubeconfig capi-quickstart > capi-quickstart.kubeconfig
```

### Deploy a CNI solution

For the purposes of this quick start we will deploy Cilium as a CNI solution,
if deploying another CNI solution, you may need to make configuration changes
either in the CNI deployment or in the Cluster deployment manifests to ensure
a working cluster.

```bash
kubectl --kubeconfig=capi-quickstart.kubeconfig create -f https://raw.githubusercontent.com/cilium/cilium/v1.9/install/kubernetes/quick-install.yaml
```

### Clean Up

Delete workload cluster.

```bash
kubectl delete cluster capi-quickstart
```

**NOTE** IMPORTANT: In order to ensure a proper cleanup of your infrastructure you must always delete the cluster object. Deleting the entire cluster template with `kubectl delete -f capi-quickstart.yaml` might lead to pending resources to be cleaned up manually.

**NOTE** IMPORTANT: The OS images used in this quick start live here: https://github.com/orgs/tinkerbell/packages?repo_name=cluster-api-provider-tinkerbell and are only build for BIOS based systems.

<!-- links -->
[bootstrap cluster]: https://cluster-api.sigs.k8s.io/reference/glossary.html#bootstrap-cluster
[clusterctl generate cluster]: https://cluster-api.sigs.k8s.io/clusterctl/commands/generate-cluster.html
[clusterctl get kubeconfig]: https://cluster-api.sigs.k8s.io/clusterctl/commands/get-kubeconfig.html
[clusterctl]: https://cluster-api.sigs.k8s.io/user/quick-start.html#install-clusterctl
[Docker]: https://docs.docker.com/get-docker/
[infrastructure provider]: https://cluster-api.sigs.k8s.io/reference/glossary.html#infrastructure-provider
[kind]: https://kind.sigs.k8s.io/docs/user/quick-start
[KubeadmControlPlane]: https://cluster-api.sigs.k8s.io/developer/architecture/controllers/control-plane.html
[kubectl]: https://kubernetes.io/docs/tasks/tools/#kubectl
[management cluster]: https://cluster-api.sigs.k8s.io/reference/glossary.html#management-cluster
[provider]: https://cluster-api.sigs.k8s.io/reference/providers.html
[provider components]: https://cluster-api.sigs.k8s.io/reference/glossary.html#provider-components
[workload cluster]: https://cluster-api.sigs.k8s.io/reference/glossary.html#workload-cluster
[Tinkerbell]: https://tinkerbell.org
[Tinkerbell stack]: https://github.com/tinkerbell/charts/blob/main/tinkerbell/stack/README.md
