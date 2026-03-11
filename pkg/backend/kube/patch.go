package kube

import (
	"fmt"

	"github.com/tinkerbell/tinkerbell/pkg/data"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// validPatchTypes is the set of Kubernetes patch types accepted by patchFromOpts.
var validPatchTypes = map[types.PatchType]bool{
	types.JSONPatchType:           true,
	types.MergePatchType:          true,
	types.StrategicMergePatchType: true,
	types.ApplyPatchType:          true,
}

// patchFromOpts builds a client.Patch from the given UpdateOptions.
// It returns (nil, nil) when no patch options are set, signaling the caller to fall through to a full Update.
func patchFromOpts(opts data.UpdateOptions) (client.Patch, error) {
	switch {
	case opts.PatchFrom != nil:
		obj, ok := opts.PatchFrom.(client.Object)
		if !ok {
			return nil, fmt.Errorf("PatchFrom must be a client.Object, got %T", opts.PatchFrom)
		}
		return client.MergeFrom(obj), nil
	case opts.RawPatch != nil:
		if opts.RawPatchType == "" {
			return nil, fmt.Errorf("RawPatchType must be set when RawPatch is provided")
		}
		pt := types.PatchType(opts.RawPatchType)
		if !validPatchTypes[pt] {
			return nil, fmt.Errorf("unsupported RawPatchType %q", opts.RawPatchType)
		}
		return client.RawPatch(pt, opts.RawPatch), nil
	default:
		return nil, nil
	}
}
