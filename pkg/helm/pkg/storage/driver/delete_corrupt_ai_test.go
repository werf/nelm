//go:build ai_tests

package driver

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
)

const corruptReleaseBody = "not-base64-at-all!!!"

func TestAI_SecretsDelete_RemovesReleaseWithUndecodableBody(t *testing.T) {
	key := testKey("broken", 1)
	secrets := newTestFixtureSecrets(t, releaseStub("broken", 1, "default", common.StatusDeployed))
	secrets.impl.(*MockSecretsInterface).objects[key].Data["release"] = []byte(corruptReleaseBody)

	rls, err := secrets.Delete(key)
	require.NoError(t, err, "a corrupt body must not block deletion")
	assert.Nil(t, rls)

	_, err = secrets.impl.Get(context.Background(), key, metav1.GetOptions{})
	require.Error(t, err, "the object must be gone from storage")

	_, err = secrets.Delete(key)
	assert.ErrorIs(t, err, ErrReleaseNotFound)
}

func TestAI_ConfigMapsDelete_RemovesReleaseWithUndecodableBody(t *testing.T) {
	key := testKey("broken", 1)
	cfgmaps := newTestFixtureCfgMaps(t, releaseStub("broken", 1, "default", common.StatusDeployed))
	cfgmaps.impl.(*MockConfigMapsInterface).objects[key].Data["release"] = corruptReleaseBody

	rls, err := cfgmaps.Delete(key)
	require.NoError(t, err, "a corrupt body must not block deletion")
	assert.Nil(t, rls)

	_, err = cfgmaps.impl.Get(context.Background(), key, metav1.GetOptions{})
	require.Error(t, err, "the object must be gone from storage")
}

func TestAI_SQLDelete_RemovesReleaseWithUndecodableBody(t *testing.T) {
	key := testKey("broken", 1)
	sqlDriver, mock := newTestFixtureSQL(t)
	namespace := sqlDriver.namespace

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s = $1 AND %s = $2",
		sqlReleaseTableBodyColumn, sqlReleaseTableName, sqlReleaseTableKeyColumn, sqlReleaseTableNamespaceColumn,
	))).WithArgs(key, namespace).
		WillReturnRows(mock.NewRows([]string{sqlReleaseTableBodyColumn}).AddRow(corruptReleaseBody)).
		RowsWillBeClosed()
	mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(
		"DELETE FROM %s WHERE %s = $1 AND %s = $2",
		sqlReleaseTableName, sqlReleaseTableKeyColumn, sqlReleaseTableNamespaceColumn,
	))).WithArgs(key, namespace).WillReturnResult(sqlmock.NewResult(0, 1))
	mockGetReleaseCustomLabels(mock, key, namespace, nil)
	mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(
		"DELETE FROM %s WHERE %s = $1 AND %s = $2",
		sqlCustomLabelsTableName, sqlCustomLabelsTableReleaseKeyColumn, sqlCustomLabelsTableReleaseNamespaceColumn,
	))).WithArgs(key, namespace).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	rls, err := sqlDriver.Delete(key)
	require.NoError(t, err, "a corrupt body must not block deletion")
	assert.Nil(t, rls)
	require.NoError(t, mock.ExpectationsWereMet(), "the rows must be deleted even though the body cannot be decoded")
}

func TestAI_SecretsDelete_StillReturnsDecodableRelease(t *testing.T) {
	rel := releaseStub("ok", 2, "default", common.StatusDeployed)
	secrets := newTestFixtureSecrets(t, rel)

	rls, err := secrets.Delete(testKey("ok", 2))
	require.NoError(t, err)
	require.NotNil(t, rls)
	assert.Equal(t, rel, rls)
}
