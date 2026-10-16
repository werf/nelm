package release

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helmreleasecommon "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
)

func TestSQLStorageBackend_AnnotationWriteErrors(t *testing.T) {
	permissionError := &pq.Error{Code: pqerror.InsufficientPrivilege, Message: "permission denied"}
	otherPQError := &pq.Error{Code: pqerror.UniqueViolation, Message: "duplicate annotation"}
	sentinel := errors.New("annotation write failed")

	for _, operation := range []string{"create", "update"} {
		for _, statement := range []string{"delete", "insert"} {
			for _, tt := range []struct {
				name       string
				err        error
				pqErr      *pq.Error
				permission bool
			}{
				{name: "permission", err: permissionError, pqErr: permissionError, permission: true},
				{name: "wrapped permission", err: fmt.Errorf("driver: %w", permissionError), pqErr: permissionError, permission: true},
				{name: "sentinel", err: sentinel},
				{name: "other PostgreSQL code", err: otherPQError, pqErr: otherPQError},
			} {
				t.Run(operation+"/"+statement+"/"+tt.name, func(t *testing.T) {
					backend, mock := newTestSQLBackend(t)
					obj := newTestStoredObject(t, newTestReleaseWithStatus("myrel", 1, helmreleasecommon.StatusDeployed))
					obj.Labels[storageLabelModifiedAt] = "200"
					obj.Annotations = map[string]string{"team": "platform"}

					mock.ExpectBegin()

					if operation == "create" {
						mock.ExpectExec(`INSERT INTO releases_v1 (key,type,body,name,namespace,version,status,owner,createdAt) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`).WillReturnResult(sqlmock.NewResult(1, 1))
					} else {
						mock.ExpectExec(`UPDATE releases_v1 SET body = $1, name = $2, version = $3, status = $4, owner = $5, modifiedAt = $6 WHERE key = $7 AND namespace = $8`).WillReturnResult(sqlmock.NewResult(0, 1))
						mock.ExpectExec(`DELETE FROM custom_labels_v1 WHERE releaseKey = $1 AND releaseNamespace = $2`).WithArgs(obj.Key, obj.Namespace).WillReturnResult(sqlmock.NewResult(0, 0))
					}

					deletion := mock.ExpectExec(`DELETE FROM nelm_release_annotations_v1 WHERE releaseKey = $1 AND releaseNamespace = $2`).WithArgs(obj.Key, obj.Namespace)
					if statement == "delete" {
						deletion.WillReturnError(tt.err)
					} else {
						deletion.WillReturnResult(sqlmock.NewResult(0, 1))
						mock.ExpectExec(testSQLAnnotationsInsert).
							WithArgs(obj.Key, obj.Namespace, pq.Array([]string{"team"}), pq.Array([]string{"platform"})).
							WillReturnError(tt.err)
					}

					mock.ExpectRollback()

					var err error
					if operation == "create" {
						err = backend.create(context.Background(), obj)
					} else {
						err = backend.update(context.Background(), obj)
					}

					require.ErrorIs(t, err, tt.err)

					if tt.pqErr != nil {
						var pqErr *pq.Error
						require.ErrorAs(t, err, &pqErr)
						assert.Same(t, tt.pqErr, pqErr)
						require.ErrorIs(t, err, tt.pqErr)
					}

					if tt.permission {
						require.ErrorContains(t, err, statement+" annotations:")

						for _, hint := range []string{sqlAnnotationsTable, obj.Namespace, "check", "current role", "table privileges", "row-level security (RLS) policies"} {
							require.ErrorContains(t, err, hint)
						}
					} else {
						require.EqualError(t, err, statement+" annotations: "+tt.err.Error())
					}
				})
			}
		}
	}
}
