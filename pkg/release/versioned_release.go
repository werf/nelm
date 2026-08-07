package release

import (
	"encoding/json"
	"fmt"

	v2release "github.com/werf/nelm/pkg/helm/intern/release/v2"
	helmrel "github.com/werf/nelm/pkg/helm/pkg/release"
	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release/v1"
)

type VersionedRelease struct {
	Accessor helmrel.Accessor
}

func (r *VersionedRelease) MarshalJSON() ([]byte, error) {
	if r == nil || r.Accessor == nil {
		return []byte("null"), nil
	}

	releaser := r.Accessor.Releaser()

	payload, err := json.Marshal(releaser)
	if err != nil {
		return nil, fmt.Errorf("marshal release payload: %w", err)
	}

	data, err := json.Marshal(versionedReleaseEnvelope{
		Release: payload,
		Version: ReleaserVersion(releaser),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal versioned release envelope: %w", err)
	}

	return data, nil
}

func (r *VersionedRelease) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		r.Accessor = nil

		return nil
	}

	var env versionedReleaseEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("unmarshal versioned release envelope: %w", err)
	}

	var releaser helmrel.Releaser
	switch env.Version {
	case ReleaseVersionV1:
		rel := &helmrelease.Release{}
		if err := json.Unmarshal(env.Release, rel); err != nil {
			return fmt.Errorf("unmarshal v1 release: %w", err)
		}

		releaser = rel
	case ReleaseVersionV2:
		rel := &v2release.Release{}
		if err := json.Unmarshal(env.Release, rel); err != nil {
			return fmt.Errorf("unmarshal v2 release: %w", err)
		}

		releaser = rel
	default:
		return fmt.Errorf("unknown release version %q", env.Version)
	}

	acc, err := helmrel.NewAccessor(releaser)
	if err != nil {
		return fmt.Errorf("wrap release: %w", err)
	}

	r.Accessor = acc

	return nil
}

type versionedReleaseEnvelope struct {
	Version string          `json:"version"`
	Release json.RawMessage `json:"release"`
}
