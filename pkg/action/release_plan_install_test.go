package action //nolint:testpackage

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/werf/nelm/pkg/release"
)

func TestReleaseMustInstallMessage(t *testing.T) {
	tests := []struct {
		name             string
		releaseName      string
		releaseNamespace string
		reason           release.ReleaseOutdatedReason
		want             string
	}{
		{
			name:             "no-reason",
			releaseName:      "myrelease",
			releaseNamespace: "mynamespace",
			reason:           release.ReleaseOutdatedReasonNone,
			want:             `No resource changes planned, but still must install release "myrelease" (namespace: "mynamespace")`,
		},
		{
			name:             "values-changed",
			releaseName:      "myrelease",
			releaseNamespace: "mynamespace",
			reason:           release.ReleaseOutdatedReasonValuesChanged,
			want:             `No resource changes planned, but still must install release "myrelease" (namespace: "mynamespace") because ` + string(release.ReleaseOutdatedReasonValuesChanged),
		},
		{
			name:             "notes-changed",
			releaseName:      "myrelease",
			releaseNamespace: "mynamespace",
			reason:           release.ReleaseOutdatedReasonNotesChanged,
			want:             `No resource changes planned, but still must install release "myrelease" (namespace: "mynamespace") because ` + string(release.ReleaseOutdatedReasonNotesChanged),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := releaseMustInstallMessage(tt.releaseName, tt.releaseNamespace, tt.reason)
			assert.Equal(t, tt.want, got)
		})
	}
}
