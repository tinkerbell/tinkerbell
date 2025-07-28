# Upgrading Tinkerbell Helm Chart

**From:** `ghcr.io/tinkerbell/charts/stack` version 0.6.2  
**To:** `ghcr.io/tinkerbell/charts/tinkerbell` version v0.19.x

## Prerequisites

Before starting the upgrade process, ensure the following conditions are met:

- ✅ No Workflows are actively running in the cluster
- ✅ All CAPT (Cluster API Provider Tinkerbell) controllers are paused or uninstalled
- ✅ You have `kubectl` and `jq` installed and configured

## Important Changes

### CRD Updates

- **No breaking changes** in the Custom Resource Definitions (CRDs)
- Additional status fields have been added to the Workflow CRD
- CRDs will be automatically updated when deploying the v0.19.x Helm chart

> [!Note]
> To disable automatic CRD migrations, use the flag `--set "deployment.envs.globals.enableCRDMigrations=false"` during deployment. If disabled, you must manually update CRDs (not covered in this guide).

### Workflow State Field Changes

A significant change in v0.19.x affects the `Workflow.Status.State` field:

- **Before:** `STATE_PENDING`, `STATE_RUNNING`, `STATE_SUCCESS`, etc.
- **After:** `PENDING`, `RUNNING`, `SUCCESS`, etc. (removed `STATE_` prefix)

> [!IMPORTANT]
> Existing Workflows with `STATE_PENDING` status will not be reconciled by the new Tink Controller.

## Upgrade Process

### Step 1: Uninstall the Old Chart

Before deploying the new chart, you must first uninstall the existing chart:

```bash
helm uninstall <release-name> -n <namespace>
```

### Step 2: Handle Existing Workflows

Choose one of the following options to handle the Workflow state changes:

#### Option 1: Update Workflow States (Recommended)

Run this command to update existing Workflows with the old state format:

```bash
kubectl get workflows -A -o json | jq -r '.items[] | select(.status.state == "STATE_PENDING") | .metadata.namespace + " " + .metadata.name' | while read -r NS WF; do
  kubectl patch workflow -n $NS $WF --subresource=status --type=merge -p '{"status":{"state":"","tasks":[]}}'
  echo "Updated workflow $WF in namespace $NS"
done
```

#### Option 2: Clean Slate Approach

If you prefer to start fresh, delete all existing Workflows:

```bash
kubectl delete workflows --all-namespaces --all
```

> [!WARNING]
> This will permanently delete all Workflows.

### Step 3: Install the New Chart

Deploy the new Tinkerbell Helm chart:

Follow the installation instructions from the [Tinkerbell Helm Chart README](../../helm/tinkerbell/README.md).

## Support

If you encounter issues during the upgrade process, please:

1. Check the [Tinkerbell documentation](https://docs.tinkerbell.org)
2. Review the [troubleshooting guide](https://docs.tinkerbell.org/troubleshooting)
3. Open an issue on the [Tinkerbell GitHub repository](https://github.com/tinkerbell/tinkerbell)
