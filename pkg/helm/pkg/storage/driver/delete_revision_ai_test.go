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
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
)

const undecodableReleaseBody = "not-base64-at-all!!!"

func TestAI_SecretsDeleteRevision_RemovesObjectWithoutDecoding(t *testing.T) {
	key := testKey("broken", 1)
	secrets := newTestFixtureSecrets(t, releaseStub("broken", 1, "default", common.StatusDeployed))
	secrets.impl.(*MockSecretsInterface).objects[key].Data["release"] = []byte(undecodableReleaseBody)

	require.NoError(t, secrets.DeleteRevision(context.Background(), key), "an undecodable body must not block deletion")

	_, err := secrets.impl.Get(context.Background(), key, metav1.GetOptions{})
	require.Error(t, err, "the object must be gone")

	assert.ErrorIs(t, secrets.DeleteRevision(context.Background(), key), ErrReleaseNotFound)
}

func TestAI_ConfigMapsDeleteRevision_RemovesObjectWithoutDecoding(t *testing.T) {
	key := testKey("broken", 1)
	cfgmaps := newTestFixtureCfgMaps(t, releaseStub("broken", 1, "default", common.StatusDeployed))
	cfgmaps.impl.(*MockConfigMapsInterface).objects[key].Data["release"] = undecodableReleaseBody

	require.NoError(t, cfgmaps.DeleteRevision(context.Background(), key))

	_, err := cfgmaps.impl.Get(context.Background(), key, metav1.GetOptions{})
	require.Error(t, err, "the object must be gone")

	assert.ErrorIs(t, cfgmaps.DeleteRevision(context.Background(), key), ErrReleaseNotFound)
}

func TestAI_SecretsDeleteRevision_HonoursCanceledContext(t *testing.T) {
	key := testKey("live", 1)
	secrets := newTestFixtureSecrets(t, releaseStub("live", 1, "default", common.StatusDeployed))
	secrets.impl = &ctxAwareSecrets{SecretInterface: secrets.impl}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := secrets.DeleteRevision(ctx, key)
	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, ErrReleaseNotFound)
}

func TestAI_SQLDeleteRevision_DeletesRowsWithoutSelectingBody(t *testing.T) {
	key := testKey("broken", 1)
	sqlDriver, mock := newTestFixtureSQL(t)
	namespace := sqlDriver.namespace

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(
		"DELETE FROM %s WHERE %s = $1 AND %s = $2",
		sqlReleaseTableName, sqlReleaseTableKeyColumn, sqlReleaseTableNamespaceColumn,
	))).WithArgs(key, namespace).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(
		"DELETE FROM %s WHERE %s = $1 AND %s = $2",
		sqlCustomLabelsTableName, sqlCustomLabelsTableReleaseKeyColumn, sqlCustomLabelsTableReleaseNamespaceColumn,
	))).WithArgs(key, namespace).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, sqlDriver.DeleteRevision(context.Background(), key))
	require.NoError(t, mock.ExpectationsWereMet(), "no SELECT of the body may be issued")
}

func TestAI_SQLDeleteRevision_NotFoundWhenNoRowDeleted(t *testing.T) {
	key := testKey("missing", 1)
	sqlDriver, mock := newTestFixtureSQL(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(
		"DELETE FROM %s WHERE %s = $1 AND %s = $2",
		sqlReleaseTableName, sqlReleaseTableKeyColumn, sqlReleaseTableNamespaceColumn,
	))).WithArgs(key, sqlDriver.namespace).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	assert.ErrorIs(t, sqlDriver.DeleteRevision(context.Background(), key), ErrReleaseNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAI_MemoryDeleteRevision_RemovesRecord(t *testing.T) {
	mem := tsFixtureMemory(t)
	mem.SetNamespace("default")
	key := testKey("rls-a", 1)

	require.NoError(t, mem.DeleteRevision(context.Background(), key))
	assert.ErrorIs(t, mem.DeleteRevision(context.Background(), key), ErrReleaseNotFound)
}

func TestAI_SecretsDelete_StillFetchesAndReturnsRelease(t *testing.T) {
	rel := releaseStub("ok", 2, "default", common.StatusDeployed)
	secrets := newTestFixtureSecrets(t, rel)

	rls, err := secrets.Delete(testKey("ok", 2))
	require.NoError(t, err)
	assert.Equal(t, rel, rls, "the legacy Delete keeps its fetch-then-return contract")
}

type ctxAwareSecrets struct {
	corev1.SecretInterface
}

func (c *ctxAwareSecrets) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.SecretInterface.Delete(ctx, name, opts)
}
