package release

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	migrate "github.com/rubenv/sql-migrate"

	"github.com/werf/nelm/v2/pkg/log"
)

const (
	sqlDialect = "postgres"

	sqlReleaseTable            = "releases_v1"
	sqlReleaseKeyColumn        = "key"
	sqlReleaseTypeColumn       = "type"
	sqlReleaseBodyColumn       = "body"
	sqlReleaseNameColumn       = "name"
	sqlReleaseNamespaceColumn  = "namespace"
	sqlReleaseVersionColumn    = "version"
	sqlReleaseStatusColumn     = "status"
	sqlReleaseOwnerColumn      = "owner"
	sqlReleaseCreatedAtColumn  = "createdAt"
	sqlReleaseModifiedAtColumn = "modifiedAt"

	sqlCustomLabelsTable                 = "custom_labels_v1"
	sqlCustomLabelsReleaseKeyColumn      = "releaseKey"
	sqlCustomLabelsReleaseNamespaceColumn = "releaseNamespace"
	sqlCustomLabelsKeyColumn             = "key"
	sqlCustomLabelsValueColumn           = "value"
)

var _ storageBackend = (*sqlStorageBackend)(nil)

// sqlStorageBackend stores revisions in the PostgreSQL schema of the Helm SQL driver: system
// labels live in columns of the releases table, custom labels in a table of their own. The
// schema declares its camelCase columns unquoted, so PostgreSQL reports them in lower case.
type sqlStorageBackend struct {
	db               *sqlx.DB
	statementBuilder sq.StatementBuilderType
}

type sqlReleaseRecord struct {
	Key        string `db:"key"`
	Namespace  string `db:"namespace"`
	Name       string `db:"name"`
	Version    int    `db:"version"`
	Status     string `db:"status"`
	Owner      string `db:"owner"`
	CreatedAt  int64  `db:"createdat"`
	ModifiedAt int64  `db:"modifiedat"`
	Body       string `db:"body"`
}

type sqlCustomLabelRecord struct {
	ReleaseKey string `db:"releasekey"`
	Key        string `db:"key"`
	Value      string `db:"value"`
}

func newSQLStorageBackend(ctx context.Context, connectionString string) (*sqlStorageBackend, error) {
	db, err := sqlx.ConnectContext(ctx, sqlDialect, connectionString)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	if err := ensureSQLStorageSchema(ctx, db); err != nil {
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
	defer tx.Rollback()

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
	defer tx.Rollback()

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

func (b *sqlStorageBackend) listMetadata(ctx context.Context, namespace, releaseName string) ([]*storedObject, error) {
	query, args, err := b.filterReleases(b.selectReleases(false), namespace, releaseName).ToSql()
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

func (b *sqlStorageBackend) listWithBodies(ctx context.Context, namespace, releaseName string, fn func(obj *storedObject) error) error {
	objects, err := b.listMetadata(ctx, namespace, releaseName)
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

	query, args, err = b.filterReleases(b.selectReleases(true), namespace, releaseName).
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
		var record sqlReleaseRecord
		if err := rows.StructScan(&record); err != nil {
			return fmt.Errorf("scan release: %w", err)
		}

		obj := storedObjectFromSQLRecord(&record)
		for key, value := range withoutSystemLabels(customLabels[record.Key]) {
			obj.Labels[key] = value
		}

		if err := fn(obj); err != nil {
			return err
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate releases: %w", err)
	}

	return nil
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
	defer tx.Rollback()

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

func (b *sqlStorageBackend) filterReleases(builder sq.SelectBuilder, namespace, releaseName string) sq.SelectBuilder {
	builder = builder.Where(sq.Eq{sqlReleaseOwnerColumn: storageOwner})

	if namespace != "" {
		builder = builder.Where(sq.Eq{sqlReleaseNamespaceColumn: namespace})
	}

	if releaseName != "" {
		builder = builder.Where(sq.Eq{sqlReleaseNameColumn: releaseName})
	}

	return builder
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

	for key, value := range withoutSystemLabels(customLabels) {
		obj.Labels[key] = value
	}

	return obj, nil
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

// ensureSQLStorageSchema applies the migrations of the Helm SQL driver, with the same ids and
// statements, so Helm and nelm can share a database. When every migration is already applied
// nothing is executed, which lets a database user without DDL privileges use the storage.
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

	if _, err := migrate.ExecContext(ctx, db.DB, sqlDialect, migrations, migrate.Up); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}

func sqlStorageMigrationsApplied(ctx context.Context, db *sqlx.DB, migrations []*migrate.Migration) bool {
	pending := map[string]struct{}{}
	for _, migration := range migrations {
		pending[migration.Id] = struct{}{}
	}

	migrate.SetDisableCreateTable(true)
	records, err := migrate.GetMigrationRecords(db.DB, sqlDialect)
	migrate.SetDisableCreateTable(false)

	if err != nil {
		log.Default.Debug(ctx, "Unable to read applied release storage migrations: %s", err)
		return false
	}

	for _, record := range records {
		delete(pending, record.Id)
	}

	return len(pending) == 0
}
