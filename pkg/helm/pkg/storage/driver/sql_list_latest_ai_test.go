//go:build ai_tests

package driver

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	rspb "github.com/werf/nelm/pkg/helm/pkg/release"
)

func TestAI_SQLListLatestReleases(t *testing.T) {
	driver, mock := newTestFixtureSQL(t)
	first := releaseStub("one", 3, "default", rspb.StatusDeployed)
	second := releaseStub("two", 5, "default", rspb.StatusFailed)
	firstBody, err := encodeRelease(first)
	require.NoError(t, err)
	secondBody, err := encodeRelease(second)
	require.NoError(t, err)

	expectSQLListLatestReleases(mock, driver.namespace, sqlmock.NewRows(sqlLatestReleaseColumns()).
		AddRow(testKey(first.Name, first.Version), first.Namespace, first.Name, first.Version, firstBody).
		AddRow(testKey(second.Name, second.Version), second.Namespace, second.Name, second.Version, secondBody))
	mockGetReleaseCustomLabels(mock, testKey(first.Name, first.Version), first.Namespace, first.Labels)
	mockGetReleaseCustomLabels(mock, testKey(second.Name, second.Version), second.Namespace, second.Labels)

	releases, err := driver.ListLatestReleases(context.Background())
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"one": 3, "two": 5}, sqlRevisionsByName(releases))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAI_SQLListLatestReleases_AllNamespaces(t *testing.T) {
	driver, mock := newTestFixtureSQL(t)
	driver.namespace = ""
	first := releaseStub("one", 2, "ns-one", rspb.StatusDeployed)
	second := releaseStub("one", 4, "ns-two", rspb.StatusFailed)
	firstBody, err := encodeRelease(first)
	require.NoError(t, err)
	secondBody, err := encodeRelease(second)
	require.NoError(t, err)

	query := "SELECT DISTINCT ON (namespace, name) key, namespace, name, version, body FROM releases_v1 WHERE owner = $1 ORDER BY namespace, name, version DESC"
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(sqlReleaseDefaultOwner).
		WillReturnRows(sqlmock.NewRows(sqlLatestReleaseColumns()).
			AddRow(testKey(first.Name, first.Version), first.Namespace, first.Name, first.Version, firstBody).
			AddRow(testKey(second.Name, second.Version), second.Namespace, second.Name, second.Version, secondBody)).RowsWillBeClosed()
	mockGetReleaseCustomLabels(mock, testKey(first.Name, first.Version), first.Namespace, first.Labels)
	mockGetReleaseCustomLabels(mock, testKey(second.Name, second.Version), second.Namespace, second.Labels)

	releases, err := driver.ListLatestReleases(context.Background())
	require.NoError(t, err)
	require.Len(t, releases, 2)
	assert.Equal(t, "ns-one", releases[0].Namespace)
	assert.Equal(t, "ns-two", releases[1].Namespace)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAI_SQLListLatestReleases_FallsBackFromCorruptLatestRevision(t *testing.T) {
	driver, mock := newTestFixtureSQL(t)
	valid := releaseStub("one", 1, "default", rspb.StatusSuperseded)
	validBody, err := encodeRelease(valid)
	require.NoError(t, err)

	expectSQLListLatestReleases(mock, driver.namespace, sqlmock.NewRows(sqlLatestReleaseColumns()).
		AddRow(testKey("one", 3), "default", "one", 3, "corrupt"))
	expectSQLPreviousRelease(mock, "default", "one", 3, testKey("one", 2), 2, "corrupt")
	expectSQLPreviousRelease(mock, "default", "one", 2, testKey("one", 1), 1, validBody)
	mockGetReleaseCustomLabels(mock, testKey("one", 1), "default", valid.Labels)

	releases, err := driver.ListLatestReleases(context.Background())
	require.NoError(t, err)
	require.Len(t, releases, 1)
	assert.Equal(t, 1, releases[0].Version)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAI_SQLListLatestReleases_OmitsReleaseWhenEveryRevisionIsCorrupt(t *testing.T) {
	driver, mock := newTestFixtureSQL(t)

	expectSQLListLatestReleases(mock, driver.namespace, sqlmock.NewRows(sqlLatestReleaseColumns()).
		AddRow(testKey("one", 2), "default", "one", 2, "corrupt"))
	expectSQLPreviousRelease(mock, "default", "one", 2, testKey("one", 1), 1, "corrupt")
	expectSQLNoPreviousRelease(mock, "default", "one", 1)

	releases, err := driver.ListLatestReleases(context.Background())
	require.NoError(t, err)
	assert.Empty(t, releases)
	require.NoError(t, mock.ExpectationsWereMet())
}

func sqlLatestReleaseColumns() []string {
	return []string{
		sqlReleaseTableKeyColumn,
		sqlReleaseTableNamespaceColumn,
		sqlReleaseTableNameColumn,
		sqlReleaseTableVersionColumn,
		sqlReleaseTableBodyColumn,
	}
}

func expectSQLListLatestReleases(mock sqlmock.Sqlmock, namespace string, rows *sqlmock.Rows) {
	query := "SELECT DISTINCT ON (namespace, name) key, namespace, name, version, body FROM releases_v1 WHERE owner = $1 AND namespace = $2 ORDER BY namespace, name, version DESC"
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(sqlReleaseDefaultOwner, namespace).WillReturnRows(rows).RowsWillBeClosed()
}

func expectSQLPreviousRelease(mock sqlmock.Sqlmock, namespace, name string, beforeVersion int, key string, version int, body string) {
	query := "SELECT key, namespace, name, version, body FROM releases_v1 WHERE name = $1 AND namespace = $2 AND owner = $3 AND version < $4 ORDER BY version DESC LIMIT 1"
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(name, namespace, sqlReleaseDefaultOwner, beforeVersion).
		WillReturnRows(sqlmock.NewRows(sqlLatestReleaseColumns()).AddRow(key, namespace, name, version, body)).RowsWillBeClosed()
}

func expectSQLNoPreviousRelease(mock sqlmock.Sqlmock, namespace, name string, beforeVersion int) {
	query := "SELECT key, namespace, name, version, body FROM releases_v1 WHERE name = $1 AND namespace = $2 AND owner = $3 AND version < $4 ORDER BY version DESC LIMIT 1"
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(name, namespace, sqlReleaseDefaultOwner, beforeVersion).WillReturnError(sql.ErrNoRows)
}

func sqlRevisionsByName(releases []*rspb.Release) map[string]int {
	result := map[string]int{}
	for _, release := range releases {
		result[release.Name] = release.Version
	}

	return result
}
