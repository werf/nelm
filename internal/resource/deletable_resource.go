package resource

import (
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/resource/meta"
)

type DeletableResourceOptions struct{}

func NewDeletableResource(meta *meta.ResourceMeta, releaseNamespace string, stage common.Stage, opts DeletableResourceOptions) *DeletableResource {
	var keep bool
	if err := ValidateResourcePolicy(meta); err != nil {
		keep = true
	} else {
		keep = KeepOnDelete(meta, releaseNamespace)
	}

	var owner common.Ownership
	if err := validateOwnership(meta); err != nil {
		owner = common.OwnershipRelease
	} else {
		owner = ownership(meta, releaseNamespace)
	}

	return &DeletableResource{
		ResourceMeta: meta,
		Ownership:    owner,
		KeepOnDelete: keep,
	}
}

type DeletableResource struct {
	*meta.ResourceMeta

	Ownership    common.Ownership
	KeepOnDelete bool
	Stage        common.Stage
}
