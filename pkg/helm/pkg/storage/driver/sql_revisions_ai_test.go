//go:build ai_tests

package driver

import (
	"context"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/pkg/helm/pkg/release/common"
)

func TestAI_SQLRevisions_SelectsMetadataOrderedByVersion(t *testing.T) {
	sqlDriver, mock := newTestFixtureSQL(t)

	query := "SELECT name, namespace, version, status FROM releases_v1 WHERE name = $1 AND owner = $2 AND namespace = $3 ORDER BY version ASC"
	mock.ExpectQuery(regexp.QuoteMeta(query)).
		WithArgs("myrelease", sqlReleaseDefaultOwner, sqlDriver.namespace).
		WillReturnRows(sqlmock.NewRows([]string{
			sqlReleaseTableNameColumn,
			sqlReleaseTableNamespaceColumn,
			sqlReleaseTableVersionColumn,
			sqlReleaseTableStatusColumn,
		}).
			AddRow("myrelease", sqlDriver.namespace, 1, common.StatusSuperseded.String()).
			AddRow("myrelease", sqlDriver.namespace, 2, common.StatusDeployed.String())).
		RowsWillBeClosed()

	records, err := sqlDriver.Revisions(context.Background(), "myrelease")
	require.NoError(t, err)
	require.Len(t, records, 2)

	assert.Equal(t, 1, records[0].Version)
	assert.Equal(t, common.StatusSuperseded.String(), records[0].Status)
	assert.Equal(t, 2, records[1].Version)
	assert.Equal(t, common.StatusDeployed.String(), records[1].Status)
	assert.Equal(t, sqlDriver.namespace, records[1].Namespace)
	assert.Equal(t, "myrelease", records[1].Name)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAI_SQLRevisions_EmptyNamespaceIsRejected(t *testing.T) {
	sqlDriver, mock := newTestFixtureSQL(t)
	sqlDriver.namespace = ""

	_, err := sqlDriver.Revisions(context.Background(), "myrelease")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace")
	require.NoError(t, mock.ExpectationsWereMet())
}
