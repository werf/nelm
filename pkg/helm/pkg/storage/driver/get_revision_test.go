package driver

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	rspb "github.com/werf/nelm/v2/pkg/helm/pkg/release/v1"
)

func TestSecretsGetRevision_KeepsSystemLabels(t *testing.T) {
	rel := releaseStub("ok", 2, "default", common.StatusDeployed)
	secrets := newTestFixtureSecrets(t, rel)

	got, err := secrets.GetRevision(testKey("ok", 2))
	require.NoError(t, err)

	labels := got.(*rspb.Release).Labels
	assert.Equal(t, "helm", labels["owner"], "system labels must survive, unlike Get")
	assert.Equal(t, "2", labels["version"])
	assert.Equal(t, "val1", labels["key1"], "custom labels are kept too")
}

func TestSecretsGetRevision_UndecodableBodyIsDistinctFromNotFound(t *testing.T) {
	key := testKey("broken", 1)
	secrets := newTestFixtureSecrets(t, releaseStub("broken", 1, "default", common.StatusDeployed))
	secrets.impl.(*MockSecretsInterface).objects[key].Data["release"] = []byte("not-base64-at-all!!!")

	_, err := secrets.GetRevision(key)
	require.ErrorIs(t, err, ErrReleaseUndecodable)
	assert.NotErrorIs(t, err, ErrReleaseNotFound)

	_, err = secrets.GetRevision(testKey("broken", 9))
	assert.ErrorIs(t, err, ErrReleaseNotFound)
}

func TestConfigMapsGetRevision_UndecodableBodyIsDistinctFromNotFound(t *testing.T) {
	key := testKey("broken", 1)
	cfgmaps := newTestFixtureCfgMaps(t, releaseStub("broken", 1, "default", common.StatusDeployed))
	cfgmaps.impl.(*MockConfigMapsInterface).objects[key].Data["release"] = "not-base64-at-all!!!"

	_, err := cfgmaps.GetRevision(key)
	require.ErrorIs(t, err, ErrReleaseUndecodable)
	assert.NotErrorIs(t, err, ErrReleaseNotFound)
}

func TestSQLGetRevision_UndecodableBodyIsDistinctFromNotFound(t *testing.T) {
	key := testKey("broken", 1)
	sqlDriver, mock := newTestFixtureSQL(t)

	selectQuery := regexp.QuoteMeta(fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s = $1 AND %s = $2",
		sqlReleaseTableBodyColumn, sqlReleaseTableName, sqlReleaseTableKeyColumn, sqlReleaseTableNamespaceColumn,
	))
	mock.ExpectQuery(selectQuery).WithArgs(key, sqlDriver.namespace).
		WillReturnRows(mock.NewRows([]string{sqlReleaseTableBodyColumn}).AddRow("corrupt")).RowsWillBeClosed()

	_, err := sqlDriver.GetRevision(key)
	require.ErrorIs(t, err, ErrReleaseUndecodable)
	assert.NotErrorIs(t, err, ErrReleaseNotFound)

	mock.ExpectQuery(selectQuery).WithArgs(testKey("broken", 9), sqlDriver.namespace).
		WillReturnRows(mock.NewRows([]string{sqlReleaseTableBodyColumn})).RowsWillBeClosed()

	_, err = sqlDriver.GetRevision(testKey("broken", 9))
	assert.ErrorIs(t, err, ErrReleaseNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMemoryGetRevision_ReturnsRecord(t *testing.T) {
	mem := tsFixtureMemory(t)
	mem.SetNamespace("default")

	got, err := mem.GetRevision(testKey("rls-a", 4))
	require.NoError(t, err)
	assert.Equal(t, 4, got.(*rspb.Release).Version)

	_, err = mem.GetRevision(testKey("rls-a", 99))
	assert.ErrorIs(t, err, ErrReleaseNotFound)
}
