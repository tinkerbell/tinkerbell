#!/usr/bin/env bash

set -eux

failed=0

GOIMPORTS="${GOIMPORTS:-goimports}"

if [[ -n $("$GOIMPORTS" -d -e -l .) ]]; then
	"$GOIMPORTS" -w .
	failed=1
fi

if ! go mod tidy; then
	failed=true
fi

if ! make generate generate-proto ui-generate manifests; then
	failed=1
fi

if ! git diff | (! grep .); then
	failed=1
fi

# This checks for any new files that were created. We should not have any new files.
if ! git status --porcelain | (! grep .); then
	failed=1
fi

exit "$failed"
