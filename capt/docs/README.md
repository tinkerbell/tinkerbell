## Running development version of CAPT using existing Tinkerbell instance

If you have Tinkerbell running in your environment, you can use it for CAPT.

### Requirements

Here is the list of required components to use CAPT:

- Existing Tinkerbell installation running at least versions mentioned in v0.6.0 of [sandbox](https://github.com/tinkerbell/sandbox/tree/v0.6.0), this guide assumes deployment using the sandbox with an IP address of 192.168.1.1, so modifications will be needed if Tinkerbell was deployed with a different method or if the IP address is different.
  - This also assumes that the hegel port is exposed in your environment, if running a version of the sandbox prior to https://github.com/tinkerbell/sandbox/tree/v0.6.0, this will need to be done manually.
- A Kubernetes cluster which pods has access to your Tinkerbell instance.
- At least one Hardware available with DHCP IP address configured on first interface and with proper metadata configured
- `git` binary
- `tilt` binary
- `kubectl` binary
- `clusterctl` binary
- `go` binary

Required metadata for hardware:
- metadata.facility.facility_code is set (default is "onprem")
- metadata.instance.id is set (should be a MAC address)
- metadata.instance.hostname is set
- metadata.spec.disks is set and contains at least one device matching an available disk on the system

### Mirror the necessary Tinkerbell actions to the registry

```sh
IMAGES=(
  oci2disk:v1.0.0
  writefile:v1.0.0
  kexec:v1.0.0
)
REGISTRY_IP="192.168.1.1"
for IMAGE in "${IMAGES[@]}"; do
  docker run -it --rm quay.io/containers/skopeo:latest copy --all --dest-tls-verify=false --dest-creds=admin:Admin1234 docker://quay.io/tinkerbell-actions/"${IMAGE}" docker://${REGISTRY_IP}/"${IMAGE}"
done
```

### Deploying CAPT

To run CAPT, we're going to use `tilt`.

First, make sure your `kubeconfig` points to the right Kubernetes cluster.

Take a note of name of your `kubeconfig` context, which will be used in Tilt configuration.

You can get current context name using the following command:
```sh
kubectl config get-contexts  | awk '/^*/ {print $2}'
```

Then, run the following commands to clone code we're going to run:
```sh
git clone https://github.com/kubernetes-sigs/cluster-api
git clone git@github.com:tinkerbell/cluster-api-provider-tinkerbell.git
cd ../cluster-api
```

Now, create a configuration file for Tilt. You can run the command below to create a sample config file,
then replace placeholders with actual values:
```sh
cat <<EOF > tilt-settings.json
{
  "default_registry": "quay.io/<your username>",
  "provider_repos": ["../cluster-api-provider-tinkerbell"],
  "enable_providers": ["tinkerbell", "kubeadm-bootstrap", "kubeadm-control-plane"],
  "allowed_contexts": ["<your kubeconfig context to use"]
}
EOF
```

Finally, run Tilt to deploy CAPI and CAPT to your cluster using the command below:
```sh
tilt up
```

You can now open a webpage printed by Tilt to see the progress on the deployment.

### Adding Hardware objects to your cluster

Create YAML files, which we can apply on the cluster:
```yaml
apiVersion: "tinkerbell.org/v1alpha1"
kind: Hardware
metadata:
  name: node-1
  namespace: default
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

Now, apply created YAML files on your cluster.

At least one Hardware is required to create a controlplane machine. 

**NOTE: CAPT expects Hardware to have DHCP IP address configured on first interface of the Hardware. This IP will
be then used for Node Internal IP.**

To confirm that your Hardware entries are correct, run the following command:
```sh
kubectl describe hardware
```

In the output, you should be able to find MAC address and IP addresses of the hardware.

### Creating workload clusters

With all the steps above, we can now create a workload cluster.

So, let's start with generating the configuration for your cluster using the command below:
```sh
CONTROL_PLANE_VIP=192.168.1.110 POD_CIDR=172.25.0.0/16 clusterctl config cluster capi-quickstart --from templates/cluster-template.yaml --kubernetes-version=v1.20.11 --control-plane-machine-count=1 --worker-machine-count=1 > test-cluster.yaml
```

Note, the POD_CIDR is overridden above to avoid conflicting with the default assumed IP address of the Tinkerbell host (192.168.1.1).

Inspect the new configuration generated in `test-cluster.yaml` and modify it as needed.

Finally, run the following command to create a cluster:
```sh
kubectl apply -f test-cluster.yaml
```

### Observing cluster provisioning

Few seconds after creating a workload cluster, you should see some log messages in Tilt tab with CAPT that IP address has been selected for controlplane machine etc.

If you list your Hardware now with labels, you should see which Hardware has been selected by CAPT controllers:
```sh
kubectl get hardware --show-labels
```

You should also be able to list and describe the created workflows for machine provisioning using the commands below:
```sh
kubectl get workflows
kubectl describe workflows
```

Once workflows are created, make sure your machines boot from the network to pick up new Workflow.

In the output of commands above, you can see status of provisioning workflows. If everything goes well, reboot step should be the last step you can see.

You can also check general cluster provisioning status using the commands below:
```sh
kubectl get kubeadmcontrolplanes
kubectl get machines
```

### Getting access to workload cluster

To finish cluster provisioning, we must get access to it and install a CNI plugin. In this guide we will use Cilium. Cilium was chosen to avoid conflicts with the default assumed IP address for Tinkerbell (192.168.1.1)

Run the following command to fetch `kubeconfig` for your workload cluster:
```sh
clusterctl get kubeconfig capi-quickstart > kubeconfig-workload
```

Now you can apply Cilium using the command below:
```sh
KUBECONFIG=kubeconfig-workload kubectl create -f https://raw.githubusercontent.com/cilium/cilium/v1.9/install/kubernetes/quick-install.yaml
```

At this point your workload cluster should be ready for other deployments.

### Cleaning up

To remove created cluster resources, run the following command:
```sh
kubectl delete -f test-cluster.yaml
```

Right now CAPT does not de-provision the hardware when cluster is removed but makes Hardware available again for other clusters. To make sure machines can be provisioned again, securely wipe their disk and reboot them.
