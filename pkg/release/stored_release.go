package release

import (
	"encoding/json"
	"fmt"

	v2release "github.com/werf/nelm/pkg/helm/intern/release/v2"
	helmrel "github.com/werf/nelm/pkg/helm/pkg/release"
	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release/v1"
)

type StoredRelease struct {
	Releaser helmrel.Releaser
}

func (s *StoredRelease) MarshalJSON() ([]byte, error) {
	if s == nil || s.Releaser == nil {
		return []byte("null"), nil
	}

	payload, err := json.Marshal(s.Releaser)
	if err != nil {
		return nil, fmt.Errorf("marshal release payload: %w", err)
	}

	data, err := json.Marshal(storedReleaseEnvelope{
		Release: payload,
		Version: ReleaserVersion(s.Releaser),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal stored release envelope: %w", err)
	}

	return data, nil
}

func (s *StoredRelease) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		s.Releaser = nil

		return nil
	}

	var env storedReleaseEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("unmarshal stored release envelope: %w", err)
	}

	switch env.Version {
	case ReleaseVersionV1:
		rel := &helmrelease.Release{}
		if err := json.Unmarshal(env.Release, rel); err != nil {
			return fmt.Errorf("unmarshal v1 release: %w", err)
		}

		s.Releaser = rel
	case ReleaseVersionV2:
		rel := &v2release.Release{}
		if err := json.Unmarshal(env.Release, rel); err != nil {
			return fmt.Errorf("unmarshal v2 release: %w", err)
		}

		s.Releaser = rel
	default:
		return fmt.Errorf("unknown release version %q", env.Version)
	}

	return nil
}

type storedReleaseEnvelope struct {
	Version string          `json:"version"`
	Release json.RawMessage `json:"release"`
}
