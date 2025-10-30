# Migration Guide for Helm Values

This document provides a guide for migrating Helm chart values from ghcr.io/tinkerbell/charts/stack version `0.6.2` to `ghcr.io/tinkerbell/charts/tinkerbell` version `0.19.x`.

## Overview

The Helm chart from `ghcr.io/tinkerbell/charts/stack` has been deprecated and replaced with `ghcr.io/tinkerbell/charts/tinkerbell`. The new chart deploys the single binary `tinkerbell` stack which includes all the components previously deployed by the `stack` chart. The `values.yaml` file has changed significantly, and this guide will help you migrate your existing values to the new format.

## Prerequisites

Before you begin the migration process, ensure you have the following:

- A Tinkerbell stack Helm chart, version 0.6.2, deployed in your Kubernetes cluster.
- Helm 3.x installed and configured to interact with your Kubernetes cluster.

## Migration Steps

1. Get the Helm values from the currently deployed Tinkerbell Helm chart. This will generate a `v0.6.2_values.yaml` file that contains the current configuration of your Tinkerbell stack. This will be used as the input for the migration.

   ```bash
   # helm get values <release-name> -n <namespace> -a -o yaml > v0.6.2_values.yaml
   helm get values tink-stack -n tink -a -o yaml > v0.6.2_values.yaml 
   ```

1. Run the migration command to convert the `0.6.2` values to the `v0.19.x` values format. This will generate a `migrated_values.yaml` file that you can use with the installation of the `v0.19.x` chart.

   ```bash
   # Get the pod CIDRs to set as trusted proxies
   TRUSTED_PROXIES=$(kubectl get nodes -o jsonpath='{.items[*].spec.podCIDR}' | tr ' ' ',')
   
   # Set the LoadBalancer IP for Tinkerbell services
   LB_IP=192.168.2.116
   
   # Set the artifacts file server URL for HookOS
   ARTIFACTS_FILE_SERVER=http://192.168.2.117:7173

   # Tinkerbell Helm chart version
   TINKERBELL_VERSION=v0.19.2

   helm template migration oci://ghcr.io/tinkerbell/charts/tinkerbell \
     --version ${TINKERBELL_VERSION} \
     --values v0.6.2_values.yaml \
     --show-only="templates/migration/from-0.6.2.yaml" \
     --set "optional.migration.enabled=true" \
     --set "trustedProxies={${TRUSTED_PROXIES}}" \
     --set "publicIP=$LB_IP" \
     --set "artifactsFileServer=$ARTIFACTS_FILE_SERVER" > migrated_values.yaml
   ```

1. Follow the [Helm chart installation guide](/helm/tinkerbell/README.md) using the generated `migrated_values.yaml` file to install the Tinkerbell chart `v0.19.x`. This file contains the updated values that are compatible with the new chart version.

   > [!IMPORTANT]  
   > For production deployments, be sure to thoroughly review the `migrated_values.yaml` file and adjust any settings as necessary before applying it.
