# Migration Guide for Helm Values

This document provides a guide for migrating Helm chart values from ghcr.io/tinkerbell/charts/stack version 0.6.2 to ghcr.io/tinkerbell/charts/tinkerbell version 0.19.1.

## Overview

The Helm chart from `ghcr.io/tinkerbell/charts/stack` has been deprecated and replaced with `ghcr.io/tinkerbell/charts/tinkerbell`. The new chart deploys the single binary `tinkerbell` stack which includes all the components previously deployed by the `stack` chart. This chart doesn't include a `nginx` server that runs in front of all the services. Overall, the new chart is much simpler and more straightforward to use. The values.yaml file has changed significantly, and this guide will help you migrate your existing values to the new format.

## Prerequisites

Before you begin the migration process, ensure you have the following:

- A Tinkerbell stack Helm chart version 0.6.2 deployed in your Kubernetes cluster.
- Helm 3.x installed and configured to interact with your Kubernetes cluster.
- Docker installed.

## Migration Steps

Option 1: Populate a values.yaml from the existing stack chart.

1. Use the `helm get values` command to retrieve the current values from the `stack` chart:

   ```bash
   helm get values <release-name> -n <namespace> -a -o yaml > values.yaml
   ```

1. Run the migration command to convert the values to the new format:

   ```bash
   
   ```

1. Pipe an existing values file into the migration command running in a container:

   ```bash
   cat helm/tinkerbell/value-migrations/v0.6.2.yaml | docker run -i --rm -v ${PWD}:/code -w /code tinkerbell/tinkerbell migrate helm -m /code/helm/tinkerbell/value-migrations/to-v0.19.yaml 
   ```

1. Pipe the existing values file from an existing release into the migration command running in a container:

   ```bash
   helm get values stack-release -n tink -a -o yaml | docker run -i --rm -v ${PWD}:/code -w /code tinkerbell/tinkerbell migrate helm -m /code/helm/tinkerbell/value-migrations/to-v0.19.yaml
   ```

1. Specify an existing values file as a flag to the migration command running in a container:

   ```bash
   docker run -it --rm -v ${PWD}:/code -w /code tinkerbell/tinkerbell migrate helm -m /code/helm/tinkerbell/value-migrations/to-v0.19.yaml -p /code/helm/tinkerbell/value-migrations/v0.6.2.yaml
   ```

1. Pipe the existing values file from an existing release into the migration command:

   ```bash
   helm get values stack-release -n tink -a -o yaml | tinkerbell migrate helm -m helm/tinkerbell/value-migrations/to-v0.19.yaml
   ```

1. Pipe an existing values file into the migration command:

   ```bash
   cat helm/tinkerbell/value-migrations/v0.6.2.yaml | tinkerbell migrate helm -m helm/tinkerbell/value-migrations/to-v0.19.yaml
   ```

1. Specify an existing values file as a flag to the migration command:

   ```bash
   tinkerbell migrate helm --set "trustedProxies={${TRUSTED_PROXIES}}" --set "publicIP=$LB_IP" --set "artifactsFileServer=$ARTIFACTS_FILE_SERVER" --set "deployment.envs.smee.dhcpEnabled=false" -m helm/tinkerbell/value-migrations/to-v0.19.yaml -p helm/tinkerbell/value-migrations/v0.6.2.yaml
   ```
