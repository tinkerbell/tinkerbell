# Migration Guide for Helm Values

This document provides a guide for migrating Helm chart values from ghcr.io/tinkerbell/charts/stack version 0.6.2 to ghcr.io/tinkerbell/charts/tinkerbell version 0.19.1.

## Overview

The Helm chart from `ghcr.io/tinkerbell/charts/stack` has been deprecated and replaced with `ghcr.io/tinkerbell/charts/tinkerbell`. The new chart deploys the single binary `tinkerbell` stack which includes all the components previously deployed by the `stack` chart. This chart doesn't include a `nginx` server that runs in front of all the services. Overall, the new chart is much simpler and more straightforward to use. The values.yaml file has changed significantly, and this guide will help you migrate your existing values to the new format.

## Prerequisites

Before you begin the migration process, ensure you have the following:

- A Tinkerbell stack Helm chart version 0.6.2 deployed in your Kubernetes cluster.
- Helm 3.x installed and configured to interact with your Kubernetes cluster.

## Migration Steps

1. Get the current Helm values from a v0.6.2 Tinkerbell Helm chart release:

   ```bash
   helm get values <release-name> -n <namespace> -a -o yaml > v0.6.2_values.yaml
   ```

1. Run the migration command to convert the v0.6.2 values to the v0.19.x format:

   ```bash
   # Get the pod CIDRs to set as trusted proxies
   TRUSTED_PROXIES=$(kubectl get nodes -o jsonpath='{.items[*].spec.podCIDR}' | tr ' ' ',')
   
   # Set the LoadBalancer IP for Tinkerbell services
   LB_IP=192.168.2.116
   
   # Set the artifacts file server URL for HookOS
   ARTIFACTS_FILE_SERVER=http://192.168.2.117:7173

   helm template migration oci://tinkerbell/charts/tinkerbell --version v0.19.2 -f v0.6.2_values.yaml --show-only="templates/migration/from-0.6.2.yaml" --set "optional.migration.enabled=true" --set "trustedProxies={${TRUSTED_PROXIES}}" --set "publicIP=$LB_IP" --set "artifactsFileServer=$ARTIFACTS_FILE_SERVER"
   ```

When you install the v0.19.x chart, use the output from the migration command. For example:

```bash
helm upgrade --install tinkerbell oci://ghcr.io/tinkerbell/charts/tinkerbell \
  --version v0.19.2 \
  --namespace tinkerbell --create-namespace  --wait  -f <output-from-migration-command>  --set "trustedProxies={${TRUSTED_PROXIES}}"  --set "publicIP=$LB_IP" --set "artifactsFileServer=$ARTIFACTS_FILE_SERVER"
```
