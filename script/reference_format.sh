#!/bin/bash

#set -eou pipefail

# inputs: resource type, resource name, namespace
# output:
#   name: <name>
#   namespace: <namespace>
#   group: <group>
#   version: <version>
#   resource: <resource>

# example usage:
# ./reference_format.sh <type> <name> <namespace>
# ./reference_format.sh workflow virtual tink-system

type="$1"
name="$2"
namespace=${3:-"default"}

if [[ -z "${type}" ]] || [[ -z "${name}" ]]; then
  echo "Usage: $0 <resource-type> <resource-name> [<namespace>]"
  exit 1
fi

error=$(kubectl get "${type}" -n "${namespace}" "${name}" 2>&1)
sc=$?
if [[ "${sc}" -ne 0 ]]; then
  echo "${error}; namespace: ${namespace}"
  exit "${sc}"
fi

read -r retrieved_name retrieved_namespace unparsed_api_version retrieved_kind <<< "$(kubectl get "${type}" -n "${namespace}" "${name}" -o jsonpath='{.metadata.name} {.metadata.namespace} {.apiVersion} {.kind}')"

retrieved_api_version=$(echo "${unparsed_api_version}" | cut -d'/' -s -f1)
if [[ -z "${retrieved_api_version}" ]]; then
  retrieved_version=$(echo "${unparsed_api_version}" | cut -d'/' -f1)
  retrieved_group=""
else
  retrieved_group=$(echo "${unparsed_api_version}" | cut -d'/' -f1)
  retrieved_version=$(echo "${unparsed_api_version}" | cut -d'/' -f2)
fi

echo "name: ${retrieved_name}"
echo "namespace: ${retrieved_namespace}"
echo "group: ${retrieved_group}"
echo "version: ${retrieved_version}"
echo "resource: $(kubectl api-resources | grep -w "${retrieved_kind}" | awk '{print $1}')"
