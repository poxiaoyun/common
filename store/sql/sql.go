package sql

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode"

	"github.com/go-logr/logr"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	gormmysql "gorm.io/driver/mysql"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormschema "gorm.io/gorm/schema"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/selector"
	"xiaoshiai.cn/common/store"
)

var _ store.PingableStore = &Storage{}

const (
	DBDriverPostgres = "postgres"
	DBDriverMySQL    = "mysql"
)

func NewDefaultOptions() *Options {
	return &Options{
		Addr:     "postgres:5432",
		Username: "postgres",
		Password: "",
		Driver:   DBDriverPostgres,
		Database: "",
		Params:   map[string]string{},
	}
}

// nolint: tagalign
type Options struct {
	Addr     string            `json:"addr" description:"database host addr"`
	Driver   string            `json:"driver" description:"databse driver, mysql or postgres"`
	Username string            `json:"username" description:"database username"`
	Password string            `json:"password" description:"database password"`
	Database string            `json:"database" description:"database to use"`
	Params   map[string]string `json:"params" description:"additional database connection parameters"`
}

// ConnectionString returns the connection string for the database without the driver schema.
// for mysql, it returns "user:password@tcp(host:port)/database".
// for postgres, it returns "user:password@host:port/database".
func (o *Options) ConnectionString() string {
	switch o.Driver {
	case DBDriverMySQL:
		values := url.Values{}
		values.Add("parseTime", "True")
		values.Add("loc", "UTC")
		return fmt.Sprintf("mysql://%s:%s@tcp(%s)/%s?%s", o.Username, o.Password, o.Addr, o.Database, values.Encode())
	case DBDriverPostgres:
		values := url.Values{}
		values.Add("sslmode", "disable")
		values.Add("timezone", "UTC")
		if o.Database == "" {
			return fmt.Sprintf("postgres://%s:%s@%s?%s", o.Username, o.Password, o.Addr, values.Encode())
		}
		return fmt.Sprintf("postgres://%s:%s@%s/%s?%s", o.Username, o.Password, o.Addr, o.Database, values.Encode())
	default:
		return ""
	}
}

func NewGormStorage(ctx context.Context, schema *store.Schema, options *Options) (*Storage, error) {
	schema, err := schema.Clone()
	if err != nil {
		return nil, err
	}
	log := logr.FromContextOrDiscard(ctx)
	dburl := options.ConnectionString()
	log.Info("database check", "database", options.Database)
	if err := createDatabaseIfNotExists(ctx, options); err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}
	var driver gorm.Dialector
	switch options.Driver {
	case DBDriverMySQL:
		driver = gormmysql.Open(strings.TrimPrefix(dburl, "mysql://"))
	case DBDriverPostgres:
		driver = gormpostgres.Open(dburl)
	default:
		return nil, fmt.Errorf("empty or unsupported database driver: [%s]", options.Driver)
	}
	db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: jsonNamingStrategy{NamingStrategy: gormschema.NamingStrategy{}}})
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}
	core := &core{
		db:     db,
		helper: NewStructHelper(),
		driver: options.Driver,
	}
	storage := &Storage{core: core, schema: schema}
	if err := storage.ensureSchema(ctx); err != nil {
		return nil, err
	}
	return storage, nil
}

type jsonNamingStrategy struct {
	gormschema.NamingStrategy
}

func (n jsonNamingStrategy) ColumnName(_ string, column string) string {
	runes := []rune(column)
	end := 1
	for end < len(runes) && unicode.IsUpper(runes[end]) {
		if end+1 < len(runes) && unicode.IsLower(runes[end+1]) {
			break
		}
		end++
	}
	for i := 0; i < end; i++ {
		runes[i] = unicode.ToLower(runes[i])
	}
	return string(runes)
}

func createDatabaseIfNotExists(ctx context.Context, options *Options) error {
	nodboptions := *options
	nodboptions.Database = ""
	driver, dsn, dbname := nodboptions.Driver, nodboptions.ConnectionString(), options.Database

	switch driver {
	case DBDriverMySQL:
		db, err := sql.Open("mysql", strings.TrimPrefix(dsn, "mysql://"))
		if err != nil {
			return fmt.Errorf("failed to open database connection: %w", err)
		}
		defer db.Close()
		_, err = db.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS `"+dbname+"`")
		return err
	case DBDriverPostgres:
		db, err := sql.Open("pgx/v5", dsn) // gorm use the pgx driver
		if err != nil {
			return fmt.Errorf("failed to open database connection: %w", err)
		}
		defer db.Close()
		_, err = db.ExecContext(ctx, `CREATE DATABASE "`+dbname+`"`)
		// https://www.postgresql.org/docs/8.2/errcodes-appendix.html
		// 42P04	DUPLICATE DATABASE	duplicate_database
		pge := &pgconn.PgError{} // pgx driver
		if stderrors.As(err, &pge) && pge.Code == "42P04" {
			return nil
		}
		pqe := &pq.Error{} // lib/pq driver
		if stderrors.As(err, &pqe) && pqe.Code == "42P04" {
			return nil
		}
		return err
	default:
		return nil
	}
}

type Storage struct {
	conditions []store.Scope
	core       *core
	schema     *store.Schema
}

// Schema implements store.Store.
func (s *Storage) Schema() *store.Schema {
	return s.schema.Snapshot()
}

// Capabilities implements store.Store.
func (s *Storage) Capabilities() store.Capabilities {
	return store.Capabilities{
		LabelSelector:    true,
		FieldSelector:    true,
		Search:           true,
		Sort:             true,
		Page:             true,
		Projection:       true,
		OptimisticLock:   true,
		SecondaryIndexes: true,
		UniqueIndexes:    true,
	}
}

func (s *Storage) ensureSchema(ctx context.Context) error {
	for _, resource := range s.schema.Resources() {
		definition, err := s.schema.Resource(resource)
		if err != nil {
			return err
		}
		db := s.core.db.WithContext(ctx).Table(resource)
		if err := db.AutoMigrate(definition.Object); err != nil {
			return fmt.Errorf("auto migrate resource %q: %w", resource, err)
		}
		columns, err := db.Migrator().ColumnTypes(definition.Object)
		if err != nil {
			return fmt.Errorf("list columns for resource %q: %w", resource, err)
		}
		columnSet := make(map[string]struct{}, len(columns))
		idIsPrimary := false
		for _, column := range columns {
			columnSet[column.Name()] = struct{}{}
			primary, _ := column.PrimaryKey()
			if column.Name() == "id" && primary {
				idIsPrimary = true
			}
		}
		existing, err := db.Migrator().GetIndexes(definition.Object)
		if err != nil {
			return fmt.Errorf("list indexes for resource %q: %w", resource, err)
		}
		existingByName := make(map[string]gorm.Index, len(existing))
		automaticPrimaryName := ""
		var automaticPrimaryFields []string
		for _, index := range existing {
			existingByName[index.Name()] = index
			primary, _ := index.PrimaryKey()
			if primary && slices.Equal(index.Columns(), []string{"id"}) {
				automaticPrimaryName = index.Name()
				automaticPrimaryFields = index.Columns()
			}
		}
		if s.core.driver == DBDriverPostgres && idIsPrimary {
			primaryColumns := []struct {
				Name   string `gorm:"column:constraint_name"`
				Column string `gorm:"column:column_name"`
			}{}
			if err := s.core.db.WithContext(ctx).Raw(`
SELECT tc.constraint_name, kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_schema = kcu.constraint_schema
 AND tc.constraint_name = kcu.constraint_name
WHERE tc.table_schema = current_schema()
  AND tc.table_name = ?
  AND tc.constraint_type = 'PRIMARY KEY'
ORDER BY kcu.ordinal_position`, resource).Scan(&primaryColumns).Error; err != nil {
				return fmt.Errorf("find primary key for resource %q: %w", resource, err)
			}
			if len(primaryColumns) > 0 {
				automaticPrimaryName = primaryColumns[0].Name
				automaticPrimaryFields = make([]string, 0, len(primaryColumns))
				for _, column := range primaryColumns {
					automaticPrimaryFields = append(automaticPrimaryFields, column.Column)
				}
			}
		}
		for _, index := range definition.Indexes {
			for _, field := range index.Fields {
				if _, ok := columnSet[field]; !ok {
					return fmt.Errorf("resource %q index %q references missing column %q", resource, index.Name, field)
				}
			}
			if current, ok := existingByName[index.Name]; ok {
				unique, _ := current.Unique()
				if !slices.Equal(current.Columns(), index.Fields) || unique != index.Unique {
					return fmt.Errorf("resource %q index %q conflicts with existing definition", resource, index.Name)
				}
				continue
			}
			if definition.IsPrimaryIndex(index) && automaticPrimaryName != "" && slices.Equal(automaticPrimaryFields, index.Fields) {
				continue
			}
			if err := s.createIndex(ctx, resource, index); err != nil {
				return err
			}
		}
		if automaticPrimaryName != "" && len(definition.ScopeKeys) > 0 && slices.Equal(automaticPrimaryFields, []string{"id"}) {
			if err := s.dropAutomaticPrimaryKey(ctx, resource, automaticPrimaryName); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Storage) dropAutomaticPrimaryKey(ctx context.Context, resource, name string) error {
	query := buildDropPrimaryKeyQuery(s.core.driver, resource, name)
	if err := s.core.db.WithContext(ctx).Exec(query).Error; err != nil {
		return fmt.Errorf("drop automatic primary key %q for scoped resource %q: %w", name, resource, err)
	}
	return nil
}

func buildDropPrimaryKeyQuery(driver, resource, name string) string {
	core := core{driver: driver}
	if driver == DBDriverMySQL {
		return fmt.Sprintf("ALTER TABLE %s DROP PRIMARY KEY", core.quoteKey(resource))
	}
	return fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", core.quoteKey(resource), core.quoteKey(name))
}

func (s *Storage) createIndex(ctx context.Context, resource string, index store.Index) error {
	query := buildCreateIndexQuery(s.core.driver, resource, index)
	if err := s.core.db.WithContext(ctx).Exec(query).Error; err != nil {
		return fmt.Errorf("create index %q for resource %q: %w", index.Name, resource, err)
	}
	return nil
}

func buildCreateIndexQuery(driver, resource string, index store.Index) string {
	core := core{driver: driver}
	unique := ""
	if index.Unique {
		unique = "UNIQUE "
	}
	fields := append([]string(nil), index.Fields...)
	for i := range fields {
		fields[i] = core.quoteKey(fields[i])
	}
	query := fmt.Sprintf("CREATE %sINDEX %s ON %s (%s)", unique, core.quoteKey(index.Name), core.quoteKey(resource), strings.Join(fields, ", "))
	if index.Nullable && driver == DBDriverPostgres {
		predicates := make([]string, 0, len(index.Fields))
		for _, field := range index.Fields {
			predicates = append(predicates, core.quoteKey(field)+" IS NOT NULL")
		}
		query += " WHERE " + strings.Join(predicates, " AND ")
	}
	return query
}

func (s *Storage) Ping(ctx context.Context) error {
	db, err := s.core.db.DB()
	if err != nil {
		return err
	}
	return db.PingContext(ctx)
}

// Count implements store.Store.
func (s *Storage) Count(ctx context.Context, obj store.Object, opts ...store.CountOption) (int, error) {
	options := store.ApplyCountOptions(opts)
	return s.core.count(ctx, s.conditions, obj, options)
}

// DeleteBatch implements store.Store.
func (s *Storage) DeleteBatch(ctx context.Context, obj store.ObjectList, opts ...store.DeleteBatchOption) error {
	options := store.ApplyDeleteBatchOptions(opts)
	return s.core.deleteBatch(ctx, s.conditions, obj, options)
}

// Patch implements store.Store.
func (s *Storage) Patch(ctx context.Context, obj store.Object, patch store.Patch, opts ...store.PatchOption) error {
	options := store.ApplyPatchOptions(opts)
	return s.core.patch(ctx, s.conditions, obj, patch, false, options)
}

// PatchBatch implements store.Store.
func (s *Storage) PatchBatch(ctx context.Context, obj store.ObjectList, patch store.PatchBatch, opts ...store.PatchBatchOption) error {
	options := store.ApplyPatchBatchOptions(opts)
	return s.core.patchBatch(ctx, s.conditions, obj, patch, options)
}

// Status implements store.Store.
func (s *Storage) Status() store.StatusStorage {
	return &StatusStorage{core: s.core, conditions: s.conditions}
}

// Watch implements store.Store.
func (s *Storage) Watch(ctx context.Context, obj store.ObjectList, opts ...store.WatchOption) (store.Watcher, error) {
	return nil, errors.NewUnsupported("sql store does not support watch")
}

func (s *Storage) Scope(conds ...store.Scope) store.Store {
	return &Storage{
		conditions: append(s.conditions, conds...),
		core:       s.core,
		schema:     s.schema,
	}
}

func (s *Storage) Create(ctx context.Context, in store.Object, options ...store.CreateOption) error {
	option := store.ApplyCreateOptions(options)
	return s.core.create(ctx, s.conditions, in, option)
}

func (s *Storage) Get(ctx context.Context, name string, into store.Object, options ...store.GetOption) error {
	option := store.ApplyGetOptions(options)
	return s.core.get(ctx, s.conditions, name, into, option)
}

func (s *Storage) Update(ctx context.Context, into store.Object, options ...store.UpdateOption) error {
	option := store.ApplyUpdateOptions(options)
	return s.core.update(ctx, s.conditions, into, false, option)
}

func (s *Storage) List(ctx context.Context, list store.ObjectList, options ...store.ListOption) error {
	opts := store.ApplyListOptions(options)
	if opts.Limit > 0 {
		return errors.NewUnsupported("SQL store does not support continuation pagination")
	}
	return s.core.list(ctx, s.conditions, list, opts)
}

func (s *Storage) Delete(ctx context.Context, into store.Object, options ...store.DeleteOption) error {
	option := store.ApplyDeleteOptions(options)
	return s.core.delete(ctx, s.conditions, into, option)
}

type StatusStorage struct {
	core       *core
	conditions []store.Scope
}

// Patch implements store.StatusStorage.
func (s *StatusStorage) Patch(ctx context.Context, obj store.Object, patch store.Patch, opts ...store.PatchOption) error {
	options := store.ApplyPatchOptions(opts)
	return s.core.patch(ctx, s.conditions, obj, patch, true, options)
}

// Update implements store.StatusStorage.
func (s *StatusStorage) Update(ctx context.Context, obj store.Object, opts ...store.UpdateOption) error {
	options := store.ApplyUpdateOptions(opts)
	return s.core.update(ctx, s.conditions, obj, true, options)
}

var _ store.StatusStorage = &StatusStorage{}

type core struct {
	db     *gorm.DB
	helper *StructHelper
	driver string
}

func (c *core) get(ctx context.Context, scope []store.Scope, id string, into store.Object, options store.GetOptions) error {
	resource, err := store.GetResource(into)
	if err != nil {
		return err
	}
	if id == "" {
		return NewEmptyIDStorageError(resource)
	}
	db := c.prepare(ctx, resource, scope)
	if options.FieldRequirements != nil {
		db = c.applyFields(db, options.FieldRequirements)
	}
	if options.LabelRequirements != nil {
		db = c.applyLabels(db, options.LabelRequirements)
	}
	if len(options.Fields) > 0 {
		db = db.Select(options.Fields)
	}
	rows, err := db.WithContext(ctx).Where("id = ?", id).Limit(1).Rows()
	if err != nil {
		return mapSQLError(err, resource, id)
	}
	defer rows.Close()

	if !rows.Next() {
		return errors.NewNotFound(resource, id)
	}
	if err := c.helper.ScanOne(rows, into); err != nil {
		return mapSQLError(err, resource, id)
	}
	return nil
}

func (c *core) count(ctx context.Context, scope []store.Scope, obj store.Object, options store.CountOptions) (int, error) {
	resource, err := store.GetResource(obj)
	if err != nil {
		return 0, err
	}
	db := c.prepare(ctx, resource, scope)
	if options.FieldRequirements != nil {
		db = c.applyFields(db, options.FieldRequirements)
	}
	if options.LabelRequirements != nil {
		db = c.applyLabels(db, options.LabelRequirements)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, mapSQLError(err, resource, "")
	}
	return int(count), nil
}

func (c *core) create(ctx context.Context, scopes []store.Scope, in store.Object, options store.CreateOptions) error {
	resource, err := store.GetResource(in)
	if err != nil {
		return err
	}
	if options.TTL != 0 {
		return errors.NewUnsupported("SQL store does not support TTL")
	}
	if options.DryRun {
		return errors.NewUnsupported("SQL store does not support dry-run")
	}
	store.PrepareObjectForCreate(in, resource, scopes)
	id := in.GetID()
	in.SetResourceVersion(1)
	save := c.helper.ToDriverValueMap(in)
	for _, cond := range scopes {
		save[store.ScopeResourceToFieldName(cond.Resource)] = cond.Name
	}
	if err := c.prepare(ctx, resource, nil).Create(save).Error; err != nil {
		return mapSQLError(err, resource, id)
	}
	return nil
}

func (c *core) update(ctx context.Context, scope []store.Scope, into store.Object, status bool, options store.UpdateOptions) error {
	requestedVersion := into.GetResourceVersion()
	return c.replace(ctx, scope, into, options.LabelRequirements, options.FieldRequirements, status, requestedVersion, func(current, desired store.Object) error {
		return store.CopyObject(into, desired)
	})
}

func (c *core) patchBatch(ctx context.Context, scope []store.Scope, list store.ObjectList, patch store.PatchBatch, options store.PatchBatchOptions) error {
	return errors.NewUnsupported("patch batch not supported on this storage")
}

func (c *core) patch(ctx context.Context, scope []store.Scope, into store.Object, patch store.Patch, status bool, options store.PatchOptions) error {
	return c.replace(ctx, scope, into, options.LabelRequirements, options.FieldRequirements, status, 0, func(current, desired store.Object) error {
		if err := store.CopyObject(current, desired); err != nil {
			return err
		}
		return store.ApplyPatch(desired, into, patch)
	})
}

var jsonPatchUnescape = strings.NewReplacer("~1", "/", "~0", "~")

func matchFieldFunc(list []string, pathes []string) bool {
	return len(pathes) > 0 && slices.Contains(list, pathes[0])
}

type JSONOperation struct {
	Set     []any
	Remove  []any
	Replace []any
}

func JsonPatchToUpdate(patches []map[string]any, excludes []string, includes []string) (map[string]any, error) {
	update := map[string]any{}
	jsonupdate := map[string]JSONOperation{}
	for _, patch := range patches {
		pathval, opval, value := patch["path"], patch["op"], patch["value"]
		path, ok := pathval.(string)
		if !ok || path == "" {
			return nil, fmt.Errorf("invalid patch path: %v", pathval)
		}
		op, ok := opval.(string)
		if !ok || op == "" {
			return nil, fmt.Errorf("invalid patch op: %v", opval)
		}
		if path[0] == '/' {
			path = path[1:]
		}
		jsonpathes := strings.Split(jsonPatchUnescape.Replace(path), "/")
		// filter fields
		if matchFieldFunc(excludes, jsonpathes) || len(includes) > 0 && !matchFieldFunc(includes, jsonpathes) {
			continue
		}
		switch op {
		case "add":
			if len(jsonpathes) == 1 {
				update[jsonpathes[0]] = value
			} else {
				// to JSON_SET()
				val := jsonupdate[jsonpathes[0]]
				val.Set = append(val.Set, "$."+strings.Join(jsonpathes[1:], "."), value)
				jsonupdate[jsonpathes[0]] = val
			}
		case "remove":
			if len(jsonpathes) == 1 {
				update[jsonpathes[0]] = nil
			} else {
				// to JSON_REMOVE
				val := jsonupdate[jsonpathes[0]]
				val.Remove = append(val.Remove, "$."+strings.Join(jsonpathes[1:], "."))
				jsonupdate[jsonpathes[0]] = val
			}
		case "replace":
			if len(jsonpathes) == 1 {
				update[jsonpathes[0]] = value
			} else {
				// to JSON_REPLACE
				val := jsonupdate[jsonpathes[0]]
				val.Replace = append(val.Replace, "$."+strings.Join(jsonpathes[1:], "."), value)
				jsonupdate[jsonpathes[0]] = val
			}
		default:
			return nil, fmt.Errorf("invalid patch op: %v", op)
		}
	}
	// Merge jsonupdate into update
	for key, ops := range jsonupdate {
		_, _ = key, ops
	}
	return update, nil
}

func (c *core) delete(ctx context.Context, scope []store.Scope, into store.Object, options store.DeleteOptions) error {
	resource, err := store.GetResource(into)
	if err != nil {
		return err
	}
	id := into.GetID()
	if id == "" {
		return NewEmptyIDStorageError(resource)
	}
	if err := store.ValidateSelectorRequirements(options.LabelRequirements, options.FieldRequirements); err != nil {
		return errors.NewBadRequest(err.Error())
	}

	for {
		current := store.NewObject(into)
		if err := c.get(ctx, scope, id, current, store.GetOptions{}); err != nil {
			return err
		}
		if err := store.ValidateDeletePreconditions(current, options.Preconditions); err != nil {
			return err
		}
		if err := store.ValidateDeleteRequirements(current, options.LabelRequirements, options.FieldRequirements); err != nil {
			return err
		}
		policy := store.DeletePropagationBackground
		if options.PropagationPolicy != nil {
			policy = *options.PropagationPolicy
		}
		if store.PrepareObjectForDelete(current, policy) {
			if err := c.deleteCurrent(ctx, scope, current); err != nil {
				if errors.IsConflict(err) {
					continue
				}
				return err
			}
			return store.CopyObject(current, into)
		}
		current.SetResourceVersion(current.GetResourceVersion() + 1)
		if err := c.saveCurrent(ctx, scope, current, current.GetResourceVersion()-1); err != nil {
			if errors.IsConflict(err) {
				continue
			}
			return err
		}
		return store.CopyObject(current, into)
	}
}

func (c *core) replace(
	ctx context.Context,
	scope []store.Scope,
	into store.Object,
	labelRequirements store.Requirements,
	fieldRequirements store.Requirements,
	status bool,
	requestedVersion int64,
	change func(current, desired store.Object) error,
) error {
	resource, err := store.GetResource(into)
	if err != nil {
		return err
	}
	id := into.GetID()
	if id == "" {
		return NewEmptyIDStorageError(resource)
	}
	current := store.NewObject(into)
	getOptions := store.GetOptions{
		LabelRequirements: labelRequirements,
		FieldRequirements: fieldRequirements,
	}
	if err := c.get(ctx, scope, id, current, getOptions); err != nil {
		return err
	}
	if requestedVersion != 0 && requestedVersion != current.GetResourceVersion() {
		return errors.NewConflict(resource, id, fmt.Errorf("resourceVersion %d does not match", requestedVersion))
	}
	desired := store.NewObject(into)
	if err := change(current, desired); err != nil {
		return err
	}
	completeDeletion, err := store.PrepareObjectForUpdate(current, desired, status)
	if err != nil {
		return err
	}
	if completeDeletion {
		if err := c.deleteCurrent(ctx, scope, current); err != nil {
			return err
		}
		return store.CopyObject(desired, into)
	}
	desired.SetResourceVersion(current.GetResourceVersion() + 1)
	if err := c.saveCurrent(ctx, scope, desired, current.GetResourceVersion()); err != nil {
		return err
	}
	return store.CopyObject(desired, into)
}

func (c *core) saveCurrent(ctx context.Context, scope []store.Scope, object store.Object, currentVersion int64) error {
	resource, err := store.GetResource(object)
	if err != nil {
		return err
	}
	save := c.helper.ToDriverValueMap(object)
	for _, condition := range scope {
		save[store.ScopeResourceToFieldName(condition.Resource)] = condition.Name
	}
	db := c.prepare(ctx, resource, scope).
		Where("id = ?", object.GetID()).
		Where("resourceVersion = ?", currentVersion).
		Updates(save)
	if db.Error != nil {
		return mapSQLError(db.Error, resource, object.GetID())
	}
	if db.RowsAffected == 0 {
		return errors.NewConflict(resource, object.GetID(), fmt.Errorf("object changed during update"))
	}
	return nil
}

func (c *core) deleteCurrent(ctx context.Context, scope []store.Scope, object store.Object) error {
	resource, err := store.GetResource(object)
	if err != nil {
		return err
	}
	db := c.prepare(ctx, resource, scope).
		Where("id = ?", object.GetID()).
		Where("resourceVersion = ?", object.GetResourceVersion()).
		Delete(c.helper.ToDriverValueMap(object))
	if db.Error != nil {
		return mapSQLError(db.Error, resource, object.GetID())
	}
	if db.RowsAffected == 0 {
		return errors.NewConflict(resource, object.GetID(), fmt.Errorf("object changed during delete"))
	}
	return nil
}

func (c *core) deleteBatch(ctx context.Context, scope []store.Scope, list store.ObjectList, options store.DeleteBatchOptions) error {
	resource, err := store.GetResource(list)
	if err != nil {
		return err
	}
	items, err := store.GetItemsPtr(list)
	if err != nil {
		return err
	}
	db := c.prepare(ctx, resource, scope)
	if options.FieldRequirements != nil {
		db = c.applyFields(db, options.FieldRequirements)
	}
	if options.LabelRequirements != nil {
		db = c.applyLabels(db, options.LabelRequirements)
	}
	if err := db.Delete(items).Error; err != nil {
		return mapSQLError(err, resource, "")
	}
	return nil
}

func (c *core) list(ctx context.Context, scope []store.Scope, list store.ObjectList, opts store.ListOptions) error {
	resource, err := store.GetResource(list)
	if err != nil {
		return fmt.Errorf("get resource name from list: %w", err)
	}
	items, err := store.GetItemsPtr(list)
	if err != nil {
		return fmt.Errorf("get items pointer from list: %w", err)
	}

	db := c.prepare(ctx, resource, scope)
	if opts.Search != "" {
		searchFields := opts.SearchFields
		if len(searchFields) == 0 {
			searchFields = []string{"id", "name"}
		}
		conditions := make([]string, 0, len(searchFields))
		args := make([]any, 0, len(searchFields))
		for _, field := range searchFields {
			conditions = append(conditions, fmt.Sprintf("LOWER(%s) LIKE ?", c.quoteKey(field)))
			args = append(args, "%"+strings.ToLower(opts.Search)+"%")
		}
		db = db.Where(strings.Join(conditions, " OR "), args...)
	}
	if opts.FieldRequirements != nil {
		db = c.applyFields(db, opts.FieldRequirements)
	}
	if opts.LabelRequirements != nil {
		db = c.applyLabels(db, opts.LabelRequirements)
	}
	// count total
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return mapSQLError(err, resource, "")
	}
	page := max(opts.Page, 1)
	if opts.Size > 0 {
		pageIndex := int64(page - 1)
		size := int64(opts.Size)
		offset := total
		if pageIndex <= total/size {
			offset = pageIndex * size
		}
		maxOffset := int64(^uint(0) >> 1)
		db = db.Offset(int(min(offset, maxOffset))).Limit(opts.Size)
	}
	for _, sort := range store.ParseSorts(opts.Sort) {
		if sort.Direction == meta.SortDirectionAsc {
			db = db.Order(c.quoteKey(sort.Field) + " ASC")
		} else {
			db = db.Order(c.quoteKey(sort.Field) + " DESC")
		}
	}
	if len(opts.Fields) > 0 {
		db = db.Select(c.quoteKeys(opts.Fields))
	} else {
		db = db.Select(c.quoteKeys(c.helper.Fields(list)))
	}
	rows, err := db.Rows()
	if err != nil {
		return mapSQLError(err, resource, "")
	}
	defer rows.Close()

	if err := c.helper.ScanAll(rows, items); err != nil {
		return mapSQLError(err, resource, "")
	}
	if opts.Size > 0 {
		store.SetPageListMetadata(list, page, opts.Size, int(total))
	} else {
		store.SetUnpaginatedListMetadata(list, int(total))
	}
	list.SetResource(resource)
	return nil
}

func (c *core) applyLabels(db *gorm.DB, requirements store.Requirements) *gorm.DB {
	expression, args, err := c.requirementsSQL(requirements, func(key string) string {
		return fmt.Sprintf(`%s -> '$."%s"'`, c.quoteKey("labels"), key)
	})
	if err != nil {
		db.AddError(err)
		return db
	}
	if expression == "" {
		return db
	}
	return db.Where(expression, args...)
}

func (c *core) applyFields(db *gorm.DB, requirements store.Requirements) *gorm.DB {
	expression, args, err := c.requirementsSQL(requirements, c.quoteKey)
	if err != nil {
		db.AddError(err)
		return db
	}
	if expression == "" {
		return db
	}
	return db.Where(expression, args...)
}

func (c *core) requirementsSQL(requirements store.Requirements, key func(string) string) (string, []any, error) {
	if err := requirements.Validate(); err != nil {
		return "", nil, errors.NewBadRequest(err.Error())
	}
	return c.requirementsSQLWithSeparator(requirements, key, " AND ")
}

func (c *core) requirementsSQLWithSeparator(requirements store.Requirements, key func(string) string, separator string) (string, []any, error) {
	expressions := make([]string, 0, len(requirements))
	var args []any
	for _, requirement := range requirements {
		expression, requirementArgs, err := c.requirementSQL(requirement, key)
		if err != nil {
			return "", nil, err
		}
		expressions = append(expressions, expression)
		args = append(args, requirementArgs...)
	}
	return strings.Join(expressions, separator), args, nil
}

func (c *core) requirementSQL(requirement selector.Requirement, key func(string) string) (string, []any, error) {
	switch requirement.Operator {
	case selector.None:
		return "1 = 0", nil, nil
	case selector.All:
		return "1 = 1", nil, nil
	case selector.And, selector.Or:
		separator := " AND "
		if requirement.Operator == selector.Or {
			separator = " OR "
		}
		expression, args, err := c.requirementsSQLWithSeparator(requirement.Requirements, key, separator)
		if err != nil {
			return "", nil, err
		}
		if expression == "" {
			if requirement.Operator == selector.And {
				return "1 = 1", nil, nil
			}
			return "1 = 0", nil, nil
		}
		return "(" + expression + ")", args, nil
	case selector.Not:
		expression, args, err := c.requirementSQL(requirement.Requirements[0], key)
		if err != nil {
			return "", nil, err
		}
		return "NOT (" + expression + ")", args, nil
	}

	field := key(requirement.Key)
	switch requirement.Operator {
	case selector.Equals, selector.DoubleEquals:
		return "(" + field + " IS NOT NULL AND " + field + " = ?)", requirement.Values, nil
	case selector.NotEquals:
		return "(" + field + " IS NULL OR " + field + " != ?)", requirement.Values, nil
	case selector.In:
		return "(" + field + " IS NOT NULL AND " + field + " IN ?)", []any{requirement.Values}, nil
	case selector.NotIn:
		return "(" + field + " IS NULL OR " + field + " NOT IN ?)", []any{requirement.Values}, nil
	case selector.DoesNotExist:
		return field + " IS NULL", nil, nil
	case selector.Exists:
		return field + " IS NOT NULL", nil, nil
	case selector.GreaterThan:
		return "(" + field + " IS NOT NULL AND " + field + " > ?)", requirement.Values, nil
	case selector.LessThan:
		return "(" + field + " IS NOT NULL AND " + field + " < ?)", requirement.Values, nil
	case selector.GreaterThanOrEqual:
		return "(" + field + " IS NOT NULL AND " + field + " >= ?)", requirement.Values, nil
	case selector.LessThanOrEqual:
		return "(" + field + " IS NOT NULL AND " + field + " <= ?)", requirement.Values, nil
	case selector.Like:
		return "(" + field + " IS NOT NULL AND " + field + " LIKE ?)", []any{fmt.Sprintf("%%%s%%", requirement.Values[0])}, nil
	case selector.Contains:
		return "", nil, errors.NewUnsupported("SQL store does not support Contains requirements")
	default:
		return "", nil, errors.NewUnsupported(fmt.Sprintf("SQL store does not support requirement operator %q", requirement.Operator))
	}
}

func (c *core) prepare(ctx context.Context, tablename string, scopes []store.Scope) *gorm.DB {
	db := c.db.WithContext(ctx)
	for _, cond := range scopes {
		key, val := c.quoteKey(store.ScopeResourceToFieldName(cond.Resource)), cond.Name
		db = db.Where(key+" = ?", val)
	}
	return db.Table(tablename)
}

func (c *core) quoteKeys(key []string) []string {
	for i, k := range key {
		key[i] = c.quoteKey(k)
	}
	return key
}

func (c *core) quoteKey(key string) string {
	switch c.driver {
	case DBDriverMySQL, "":
		return fmt.Sprintf("`%s`", key)
	case DBDriverPostgres:
		return fmt.Sprintf(`"%s"`, key)
	default:
		return key
	}
}

func mapSQLError(err error, resource string, name string) error {
	if err == nil {
		return nil
	}
	switch err {
	case gorm.ErrRecordNotFound:
		return errors.NewNotFound(resource, name)
	case gorm.ErrDuplicatedKey:
		return errors.NewAlreadyExists(resource, name)
	}
	mysqle := &mysql.MySQLError{}
	if stderrors.As(err, &mysqle) {
		switch mysqle.Number {
		case 1062: // duplicate key
			return errors.NewAlreadyExists(resource, name)
		case 1048: // column cannot be null
			return errors.NewBadRequest(fmt.Sprintf("column cannot be null for resource %s:", resource))
		case 1452: // foreign key constraint fails
			return errors.NewNotFound(resource, name)
		default:
			log.Error(err, "mysql error", "code", mysqle.Number, "message", mysqle.Message)
			// omit the message for security reasons
			return errors.NewBadRequest(fmt.Sprintf("mysql error %d for resource %s", mysqle.Number, resource))
		}
	}
	return err
}

func NewEmptyIDStorageError(resource string) error {
	return errors.NewBadRequest(fmt.Sprintf("empty id for resource %s", resource))
}
