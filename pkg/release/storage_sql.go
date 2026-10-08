package release

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	migrate "github.com/rubenv/sql-migrate"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	"github.com/werf/nelm/v2/pkg/log"
)

const (
	sqlCustomLabelsKeyColumn              = "key"
	sqlCustomLabelsReleaseKeyColumn       = "releaseKey"
	sqlCustomLabelsReleaseNamespaceColumn = "releaseNamespace"
	sqlCustomLabelsTable                  = "custom_labels_v1"
	sqlCustomLabelsValueColumn            = "value"
	sqlDialect                            = "postgres"
	sqlReleaseBodyColumn                  = "body"
	sqlReleaseCreatedAtColumn             = "createdAt"
	sqlReleaseKeyColumn                   = "key"
	sqlReleaseModifiedAtColumn            = "modifiedAt"
	sqlReleaseNameColumn                  = "name"
	sqlReleaseNamespaceColumn             = "namespace"
	sqlReleaseOwnerColumn                 = "owner"
	sqlReleaseStatusColumn                = "status"
	sqlReleaseTable                       = "releases_v1"
	sqlReleaseTypeColumn                  = "type"
	sqlReleaseVersionColumn               = "version"
)

var _ storageBackend = (*sqlStorageBackend)(nil)

// sqlStorageBackend stores revisions in the PostgreSQL schema of the Helm SQL driver: system
// labels live in columns of the releases table, custom labels in a table of their own. The
// schema declares its camelCase columns unquoted, so PostgreSQL reports them in lower case.
type sqlStorageBackend struct {
	db               *sqlx.DB
	statementBuilder sq.StatementBuilderType
}

func newSQLStorageBackend(ctx context.Context, connectionString string) (*sqlStorageBackend, error) {
	db, err := sqlx.ConnectContext(ctx, sqlDialect, connectionString)
	if err != nil {
		// ConnectContext returns an open handle when only the ping failed.
		if db != nil {
			db.Close()
		}

		return nil, fmt.Errorf("connect to database: %w", err)
	}

	if err := ensureSQLStorageSchema(ctx, db); err != nil {
		db.Close()

		return nil, fmt.Errorf("set up database schema: %w", err)
	}

	return newSQLStorageBackendFromDB(db), nil
}

func newSQLStorageBackendFromDB(db *sqlx.DB) *sqlStorageBackend {
	return &sqlStorageBackend{
		db:               db,
		statementBuilder: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

func (b *sqlStorageBackend) create(ctx context.Context, obj *storedObject) error {
	version, err := strconv.Atoi(obj.Labels[storageLabelVersion])
	if err != nil {
		return fmt.Errorf("parse version label: %w", err)
	}

	createdAt, err := strconv.ParseInt(obj.Labels[storageLabelCreatedAt], 10, 64)
	if err != nil {
		return fmt.Errorf("parse createdAt label: %w", err)
	}

	tx, err := b.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer rollbackSQLTransaction(ctx, tx)

	query, args, err := b.statementBuilder.
		Insert(sqlReleaseTable).
		Columns(
			sqlReleaseKeyColumn,
			sqlReleaseTypeColumn,
			sqlReleaseBodyColumn,
			sqlReleaseNameColumn,
			sqlReleaseNamespaceColumn,
			sqlReleaseVersionColumn,
			sqlReleaseStatusColumn,
			sqlReleaseOwnerColumn,
			sqlReleaseCreatedAtColumn,
		).
		Values(
			obj.Key,
			storageObjectType,
			string(obj.Body),
			obj.Labels[storageLabelName],
			obj.Namespace,
			version,
			obj.Labels[storageLabelStatus],
			storageOwner,
			createdAt,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert release query: %w", err)
	}

	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == pqerror.UniqueViolation {
			return ErrReleaseExists
		}

		return fmt.Errorf("insert release: %w", err)
	}

	for key, value := range withoutSystemLabels(obj.Labels) {
		if err := b.insertCustomLabel(ctx, tx, obj.Namespace, obj.Key, key, value); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func (b *sqlStorageBackend) delete(ctx context.Context, namespace, key string) error {
	tx, err := b.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer rollbackSQLTransaction(ctx, tx)

	query, args, err := b.statementBuilder.
		Delete(sqlReleaseTable).
		Where(sq.Eq{sqlReleaseKeyColumn: key, sqlReleaseNamespaceColumn: namespace}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete release query: %w", err)
	}

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete release: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get deleted rows count: %w", err)
	}

	if affected == 0 {
		return ErrReleaseNotFound
	}

	query, args, err = b.statementBuilder.
		Delete(sqlCustomLabelsTable).
		Where(sq.Eq{sqlCustomLabelsReleaseKeyColumn: key, sqlCustomLabelsReleaseNamespaceColumn: namespace}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete custom labels query: %w", err)
	}

	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("delete custom labels: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func (b *sqlStorageBackend) get(ctx context.Context, namespace, key string) (*storedObject, error) {
	query, args, err := b.selectReleases(true).
		Where(sq.Eq{sqlReleaseKeyColumn: key, sqlReleaseNamespaceColumn: namespace}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select release query: %w", err)
	}

	var record sqlReleaseRecord
	if err := b.db.GetContext(ctx, &record, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrReleaseNotFound
		}

		return nil, fmt.Errorf("select release: %w", err)
	}

	return b.storedObjectWithCustomLabels(ctx, &record)
}

func (b *sqlStorageBackend) insertCustomLabel(ctx context.Context, tx *sqlx.Tx, namespace, key, labelKey, labelValue string) error {
	query, args, err := b.statementBuilder.
		Insert(sqlCustomLabelsTable).
		Columns(
			sqlCustomLabelsReleaseKeyColumn,
			sqlCustomLabelsReleaseNamespaceColumn,
			sqlCustomLabelsKeyColumn,
			sqlCustomLabelsValueColumn,
		).
		Values(key, namespace, labelKey, labelValue).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert custom label query: %w", err)
	}

	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("insert custom label %q: %w", labelKey, err)
	}

	return nil
}

func (b *sqlStorageBackend) listMetadata(ctx context.Context, namespace, releaseName string) ([]*storedObject, error) {
	return b.selectMetadata(ctx, namespace, releaseName, nil)
}

func (b *sqlStorageBackend) listWithBodies(ctx context.Context, namespace, releaseName string, versions []int, fn func(obj *storedObject) error) error {
	objects, err := b.selectMetadata(ctx, namespace, releaseName, versions)
	if err != nil {
		return err
	}

	if len(objects) == 0 {
		return nil
	}

	keys := make([]string, 0, len(objects))
	for _, obj := range objects {
		keys = append(keys, obj.Key)
	}

	// Custom labels are read before the bodies, so the cursor below is the only query in
	// flight and the read needs a single connection.
	query, args, err := b.statementBuilder.
		Select(sqlCustomLabelsReleaseKeyColumn, sqlCustomLabelsKeyColumn, sqlCustomLabelsValueColumn).
		From(sqlCustomLabelsTable).
		Where(sq.Eq{sqlCustomLabelsReleaseNamespaceColumn: namespace, sqlCustomLabelsReleaseKeyColumn: keys}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build select custom labels query: %w", err)
	}

	var labelRecords []sqlCustomLabelRecord
	if err := b.db.SelectContext(ctx, &labelRecords, query, args...); err != nil {
		return fmt.Errorf("select custom labels: %w", err)
	}

	customLabels := map[string]map[string]string{}
	for _, labelRecord := range labelRecords {
		if customLabels[labelRecord.ReleaseKey] == nil {
			customLabels[labelRecord.ReleaseKey] = map[string]string{}
		}

		customLabels[labelRecord.ReleaseKey][labelRecord.Key] = labelRecord.Value
	}

	query, args, err = filterSQLReleases(b.selectReleases(true), namespace, releaseName, versions).
		OrderBy(sqlReleaseVersionColumn + " ASC").
		ToSql()
	if err != nil {
		return fmt.Errorf("build select releases query: %w", err)
	}

	rows, err := b.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("select releases: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("iterate releases: %w", err)
		}

		var record sqlReleaseRecord
		if err := rows.StructScan(&record); err != nil {
			return fmt.Errorf("scan release: %w", err)
		}

		obj := storedObjectFromSQLRecord(&record)
		maps.Copy(obj.Labels, withoutSystemLabels(customLabels[record.Key]))

		if err := fn(obj); err != nil {
			return err
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate releases: %w", err)
	}

	if err := rows.Close(); err != nil {
		return fmt.Errorf("close releases: %w", err)
	}

	return nil
}

func (b *sqlStorageBackend) scanLatestCandidates(ctx context.Context, namespace string, selector labels.Selector, withBodies bool, fn func(obj *storedObject) error) error {
	builder := filterSQLReleases(b.selectReleases(withBodies), namespace, "", nil).
		Options("DISTINCT ON (namespace, name)").
		OrderBy("namespace", "name", "version DESC")
	if selector != nil {
		requirements, selectable := selector.Requirements()
		if !selectable {
			builder = builder.Where("FALSE")
		}

		for _, requirement := range requirements {
			predicate, err := sqlLabelRequirement(requirement)
			if err != nil {
				return fmt.Errorf("build release label selector: %w", err)
			}

			builder = builder.Where(predicate)
		}
	}

	const customLabelsColumn = "COALESCE((" +
		"SELECT json_object_agg(c.key, c.value ORDER BY c.ctid)::text " +
		"FROM custom_labels_v1 c " +
		"WHERE c.releaseKey = releases_v1.key AND c.releaseNamespace = releases_v1.namespace" +
		"), '{}') AS custom_labels"

	query, args, err := b.statementBuilder.
		Select("releases_v1.*", customLabelsColumn).
		FromSelect(builder, sqlReleaseTable).
		ToSql()
	if err != nil {
		return fmt.Errorf("build select latest releases query: %w", err)
	}

	rows, err := b.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("select latest releases: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("read release objects: %w", err)
		}

		var record struct {
			sqlReleaseRecord

			CustomLabels string `db:"custom_labels"`
		}
		if err := rows.StructScan(&record); err != nil {
			return fmt.Errorf("scan release: %w", err)
		}

		var customLabels map[string]string
		if err := json.Unmarshal([]byte(record.CustomLabels), &customLabels); err != nil {
			return fmt.Errorf("decode custom labels of release %q (namespace: %q): %w", record.Key, record.Namespace, err)
		}

		obj := storedObjectFromSQLRecord(&record.sqlReleaseRecord)
		maps.Copy(obj.Labels, withoutSystemLabels(customLabels))

		if err := fn(obj); err != nil {
			return err
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate releases: %w", err)
	}

	if err := rows.Close(); err != nil {
		return fmt.Errorf("close releases: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("read release objects: %w", err)
	}

	return nil
}

func (b *sqlStorageBackend) selectMetadata(ctx context.Context, namespace, releaseName string, versions []int) ([]*storedObject, error) {
	query, args, err := filterSQLReleases(b.selectReleases(false), namespace, releaseName, versions).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select releases query: %w", err)
	}

	var records []sqlReleaseRecord
	if err := b.db.SelectContext(ctx, &records, query, args...); err != nil {
		return nil, fmt.Errorf("select releases: %w", err)
	}

	objects := make([]*storedObject, 0, len(records))
	for i := range records {
		objects = append(objects, storedObjectFromSQLRecord(&records[i]))
	}

	return objects, nil
}

func (b *sqlStorageBackend) selectReleases(withBody bool) sq.SelectBuilder {
	columns := []string{
		sqlReleaseKeyColumn,
		sqlReleaseNamespaceColumn,
		sqlReleaseNameColumn,
		sqlReleaseVersionColumn,
		sqlReleaseStatusColumn,
		sqlReleaseOwnerColumn,
		sqlReleaseCreatedAtColumn,
		sqlReleaseModifiedAtColumn,
	}

	if withBody {
		columns = append(columns, sqlReleaseBodyColumn)
	}

	return b.statementBuilder.Select(columns...).From(sqlReleaseTable)
}

func (b *sqlStorageBackend) storedObjectWithCustomLabels(ctx context.Context, record *sqlReleaseRecord) (*storedObject, error) {
	query, args, err := b.statementBuilder.
		Select(sqlCustomLabelsKeyColumn, sqlCustomLabelsValueColumn).
		From(sqlCustomLabelsTable).
		Where(sq.Eq{sqlCustomLabelsReleaseKeyColumn: record.Key, sqlCustomLabelsReleaseNamespaceColumn: record.Namespace}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select custom labels query: %w", err)
	}

	var labelRecords []sqlCustomLabelRecord
	if err := b.db.SelectContext(ctx, &labelRecords, query, args...); err != nil {
		return nil, fmt.Errorf("select custom labels of release %q (namespace: %q): %w", record.Key, record.Namespace, err)
	}

	obj := storedObjectFromSQLRecord(record)

	customLabels := map[string]string{}
	for _, labelRecord := range labelRecords {
		customLabels[labelRecord.Key] = labelRecord.Value
	}

	maps.Copy(obj.Labels, withoutSystemLabels(customLabels))

	return obj, nil
}

func (b *sqlStorageBackend) update(ctx context.Context, obj *storedObject) error {
	version, err := strconv.Atoi(obj.Labels[storageLabelVersion])
	if err != nil {
		return fmt.Errorf("parse version label: %w", err)
	}

	modifiedAt, err := strconv.ParseInt(obj.Labels[storageLabelModifiedAt], 10, 64)
	if err != nil {
		return fmt.Errorf("parse modifiedAt label: %w", err)
	}

	query, args, err := b.statementBuilder.
		Update(sqlReleaseTable).
		Set(sqlReleaseBodyColumn, string(obj.Body)).
		Set(sqlReleaseNameColumn, obj.Labels[storageLabelName]).
		Set(sqlReleaseVersionColumn, version).
		Set(sqlReleaseStatusColumn, obj.Labels[storageLabelStatus]).
		Set(sqlReleaseOwnerColumn, storageOwner).
		Set(sqlReleaseModifiedAtColumn, modifiedAt).
		Where(sq.Eq{sqlReleaseKeyColumn: obj.Key, sqlReleaseNamespaceColumn: obj.Namespace}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update release query: %w", err)
	}

	result, err := b.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update release: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get updated rows count: %w", err)
	}

	if affected == 0 {
		return ErrReleaseNotFound
	}

	return nil
}

func (b *sqlStorageBackend) updateLabels(ctx context.Context, namespace, key string, labels map[string]string) error {
	tx, err := b.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer rollbackSQLTransaction(ctx, tx)

	query, args, err := b.statementBuilder.
		Select(sqlReleaseKeyColumn).
		From(sqlReleaseTable).
		Where(sq.Eq{sqlReleaseKeyColumn: key, sqlReleaseNamespaceColumn: namespace}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build select release query: %w", err)
	}

	var existingKey string
	if err := tx.GetContext(ctx, &existingKey, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrReleaseNotFound
		}

		return fmt.Errorf("select release: %w", err)
	}

	for labelKey, labelValue := range labels {
		query, args, err := b.statementBuilder.
			Delete(sqlCustomLabelsTable).
			Where(sq.Eq{
				sqlCustomLabelsReleaseKeyColumn:       key,
				sqlCustomLabelsReleaseNamespaceColumn: namespace,
				sqlCustomLabelsKeyColumn:              labelKey,
			}).
			ToSql()
		if err != nil {
			return fmt.Errorf("build delete custom label query: %w", err)
		}

		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("delete custom label %q: %w", labelKey, err)
		}

		if err := b.insertCustomLabel(ctx, tx, namespace, key, labelKey, labelValue); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

type sqlReleaseRecord struct {
	Body       string `db:"body"`
	CreatedAt  int64  `db:"createdat"`
	Key        string `db:"key"`
	ModifiedAt int64  `db:"modifiedat"`
	Name       string `db:"name"`
	Namespace  string `db:"namespace"`
	Owner      string `db:"owner"`
	Status     string `db:"status"`
	Version    int    `db:"version"`
}

type sqlCustomLabelRecord struct {
	Key        string `db:"key"`
	ReleaseKey string `db:"releasekey"`
	Value      string `db:"value"`
}

// ensureSQLStorageSchema applies the migrations of the Helm SQL driver, with the same ids and
// statements, so Helm and nelm can share a database. When every migration is already applied
// nothing is executed, which lets a database user without DDL privileges use the storage. A
// local migration set keeps the package-level sql-migrate settings of the host application
// out of play.
func ensureSQLStorageSchema(ctx context.Context, db *sqlx.DB) error {
	migrations := &migrate.MemoryMigrationSource{
		Migrations: []*migrate.Migration{
			{
				Id: "init",
				Up: []string{
					fmt.Sprintf(`
						CREATE TABLE %s (
							%s VARCHAR(90),
							%s VARCHAR(64) NOT NULL,
							%s TEXT NOT NULL,
							%s VARCHAR(64) NOT NULL,
							%s VARCHAR(64) NOT NULL,
							%s INTEGER NOT NULL,
							%s TEXT NOT NULL,
							%s TEXT NOT NULL,
							%s INTEGER NOT NULL,
							%s INTEGER NOT NULL DEFAULT 0,
							PRIMARY KEY(%s, %s)
						);
						CREATE INDEX ON %s (%s, %s);
						CREATE INDEX ON %s (%s);
						CREATE INDEX ON %s (%s);
						CREATE INDEX ON %s (%s);
						CREATE INDEX ON %s (%s);
						CREATE INDEX ON %s (%s);

						GRANT ALL ON %s TO PUBLIC;

						ALTER TABLE %s ENABLE ROW LEVEL SECURITY;
					`,
						sqlReleaseTable,
						sqlReleaseKeyColumn,
						sqlReleaseTypeColumn,
						sqlReleaseBodyColumn,
						sqlReleaseNameColumn,
						sqlReleaseNamespaceColumn,
						sqlReleaseVersionColumn,
						sqlReleaseStatusColumn,
						sqlReleaseOwnerColumn,
						sqlReleaseCreatedAtColumn,
						sqlReleaseModifiedAtColumn,
						sqlReleaseKeyColumn,
						sqlReleaseNamespaceColumn,
						sqlReleaseTable,
						sqlReleaseKeyColumn,
						sqlReleaseNamespaceColumn,
						sqlReleaseTable,
						sqlReleaseVersionColumn,
						sqlReleaseTable,
						sqlReleaseStatusColumn,
						sqlReleaseTable,
						sqlReleaseOwnerColumn,
						sqlReleaseTable,
						sqlReleaseCreatedAtColumn,
						sqlReleaseTable,
						sqlReleaseModifiedAtColumn,
						sqlReleaseTable,
						sqlReleaseTable,
					),
				},
				Down: []string{
					fmt.Sprintf(`
						DROP TABLE %s;
					`, sqlReleaseTable),
				},
			},
			{
				Id: "custom_labels",
				Up: []string{
					fmt.Sprintf(`
						CREATE TABLE %s (
							%s VARCHAR(64),
							%s VARCHAR(67),
							%s VARCHAR(%d),
							%s VARCHAR(%d)
						);
						CREATE INDEX ON %s (%s, %s);

						GRANT ALL ON %s TO PUBLIC;
						ALTER TABLE %s ENABLE ROW LEVEL SECURITY;
					`,
						sqlCustomLabelsTable,
						sqlCustomLabelsReleaseKeyColumn,
						sqlCustomLabelsReleaseNamespaceColumn,
						sqlCustomLabelsKeyColumn,
						253+1+63,
						sqlCustomLabelsValueColumn,
						63,
						sqlCustomLabelsTable,
						sqlCustomLabelsReleaseKeyColumn,
						sqlCustomLabelsReleaseNamespaceColumn,
						sqlCustomLabelsTable,
						sqlCustomLabelsTable,
					),
				},
				Down: []string{
					fmt.Sprintf(`
						DELETE TABLE %s;
					`, sqlCustomLabelsTable),
				},
			},
		},
	}

	if sqlStorageMigrationsApplied(ctx, db, migrations.Migrations) {
		return nil
	}

	if _, err := (migrate.MigrationSet{}).ExecContext(ctx, db.DB, sqlDialect, migrations, migrate.Up); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}

func filterSQLReleases(builder sq.SelectBuilder, namespace, releaseName string, versions []int) sq.SelectBuilder {
	builder = builder.Where(sq.Eq{sqlReleaseOwnerColumn: storageOwner})

	if namespace != "" {
		builder = builder.Where(sq.Eq{sqlReleaseNamespaceColumn: namespace})
	}

	if releaseName != "" {
		builder = builder.Where(sq.Eq{sqlReleaseNameColumn: releaseName})
	}

	if versions != nil {
		builder = builder.Where(sq.Eq{sqlReleaseVersionColumn: versions})
	}

	return builder
}

func rollbackSQLTransaction(ctx context.Context, tx *sqlx.Tx) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		log.Default.Warn(ctx, "Unable to roll back release storage transaction: %s", err)
	}
}

func sqlLabelRequirement(requirement labels.Requirement) (sq.Sqlizer, error) {
	column := ""
	switch requirement.Key() {
	case storageLabelName:
		column = sqlReleaseNameColumn
	case storageLabelOwner:
		column = sqlReleaseOwnerColumn
	case storageLabelStatus:
		column = sqlReleaseStatusColumn
	case storageLabelVersion:
		column = sqlReleaseVersionColumn + "::text"
	case storageLabelCreatedAt:
		column = sqlReleaseCreatedAtColumn + "::text"
	case storageLabelModifiedAt:
		column = "NULLIF(" + sqlReleaseModifiedAtColumn + ", 0)::text"
	}

	custom := column == ""
	if custom {
		column = "value"
	}

	values := requirement.Values().List()

	var predicate sq.Sqlizer

	negate := false
	switch requirement.Operator() {
	case selection.Equals, selection.DoubleEquals, selection.In:
		predicate = sq.Eq{column: values}
	case selection.NotEquals, selection.NotIn:
		if custom {
			predicate = sq.Eq{column: values}
			negate = true
		} else {
			predicate = sq.Or{sq.Expr(column + " IS NULL"), sq.NotEq{column: values}}
		}
	case selection.Exists:
		predicate = sq.Expr(column + " IS NOT NULL")
	case selection.DoesNotExist:
		if custom {
			predicate = sq.Expr(column + " IS NOT NULL")
			negate = true
		} else {
			predicate = sq.Expr(column + " IS NULL")
		}
	case selection.GreaterThan, selection.LessThan:
		value, err := strconv.ParseInt(values[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("label %q requires an integer comparison value: %w", requirement.Key(), err)
		}

		operator := ">"
		if requirement.Operator() == selection.LessThan {
			operator = "<"
		}

		// Kubernetes compares only labels that parse as int64; other values never match, so the
		// cast is guarded instead of failing the whole query on a non-numeric or huge value.
		number := fmt.Sprintf(
			"(CASE WHEN %[1]s ~ '^[+-]??[0-9]+$' AND length(regexp_replace(%[1]s, '^[+-]??0*', '')) <= 19 THEN (%[1]s)::numeric END)",
			column,
		)
		predicate = sq.Expr(
			fmt.Sprintf("%[1]s BETWEEN -9223372036854775808 AND 9223372036854775807 AND %[1]s %[2]s ?", number, operator),
			value,
		)
	default:
		return nil, fmt.Errorf("unsupported operator %q for label %q; use equality, set, existence, or integer comparisons", requirement.Operator(), requirement.Key())
	}

	if !custom {
		return predicate, nil
	}

	// Neither nelm nor Helm writes a custom label of a release twice, so the ctid order only
	// picks a deterministic row if a foreign writer did; it does not track write order.
	label := sq.Select(sqlCustomLabelsValueColumn).From(sqlCustomLabelsTable).
		Where("releaseKey = releases_v1.key AND releaseNamespace = releases_v1.namespace").
		Where(sq.Eq{sqlCustomLabelsKeyColumn: requirement.Key()}).OrderBy("ctid DESC").Limit(1)

	match := sq.Select("1").FromSelect(label, "label").Where(predicate)
	if negate {
		return sq.Expr("NOT EXISTS (?)", match), nil
	}

	return sq.Expr("EXISTS (?)", match), nil
}

func sqlStorageMigrationsApplied(ctx context.Context, db *sqlx.DB, migrations []*migrate.Migration) bool {
	pending := map[string]struct{}{}
	for _, migration := range migrations {
		pending[migration.Id] = struct{}{}
	}

	records, err := migrate.MigrationSet{DisableCreateTable: true}.GetMigrationRecords(db.DB, sqlDialect)
	if err != nil {
		log.Default.Debug(ctx, "Unable to read applied release storage migrations: %s", err)

		return false
	}

	for _, record := range records {
		delete(pending, record.Id)
	}

	return len(pending) == 0
}

func storedObjectFromSQLRecord(record *sqlReleaseRecord) *storedObject {
	labels := map[string]string{
		storageLabelName:      record.Name,
		storageLabelOwner:     record.Owner,
		storageLabelStatus:    record.Status,
		storageLabelVersion:   strconv.Itoa(record.Version),
		storageLabelCreatedAt: strconv.FormatInt(record.CreatedAt, 10),
	}

	if record.ModifiedAt != 0 {
		labels[storageLabelModifiedAt] = strconv.FormatInt(record.ModifiedAt, 10)
	}

	var body []byte
	if record.Body != "" {
		body = []byte(record.Body)
	}

	return &storedObject{
		Namespace: record.Namespace,
		Key:       record.Key,
		Labels:    labels,
		Body:      body,
	}
}
