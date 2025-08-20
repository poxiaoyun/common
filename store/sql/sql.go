package sql

import (
	"context"
	"database/sql"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"golang.org/x/exp/maps"
	gormmysql "gorm.io/driver/mysql"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/store"
)

var _ store.Store = &Storage{}

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

func NewGormStorage(ctx context.Context, options *Options) (*Storage, error) {
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
	db, err := gorm.Open(driver, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}
	core := &core{
		db:     db,
		helper: NewStructHelper(),
		driver: options.Driver,
	}
	return &Storage{core: core}, nil
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
}

// Count implements store.Store.
func (s *Storage) Count(ctx context.Context, obj store.Object, opts ...store.CountOption) (int, error) {
	options := store.CountOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	return s.core.count(ctx, s.conditions, obj, options)
}

// DeleteBatch implements store.Store.
func (s *Storage) DeleteBatch(ctx context.Context, obj store.ObjectList, opts ...store.DeleteBatchOption) error {
	options := store.DeleteBatchOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	return s.core.deleteBatch(ctx, s.conditions, obj, options)
}

// Patch implements store.Store.
func (s *Storage) Patch(ctx context.Context, obj store.Object, patch store.Patch, opts ...store.PatchOption) error {
	options := store.PatchOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	return s.core.patch(ctx, s.conditions, obj, patch, false, options)
}

// PatchBatch implements store.Store.
func (s *Storage) PatchBatch(ctx context.Context, obj store.ObjectList, patch store.PatchBatch, opts ...store.PatchBatchOption) error {
	options := store.PatchBatchOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	return s.core.patchBatch(ctx, s.conditions, obj, patch, options)
}

// Status implements store.Store.
func (s *Storage) Status() store.StatusStorage {
	return &StatusStorage{core: s.core, conditions: s.conditions}
}

// Watch implements store.Store.
func (s *Storage) Watch(ctx context.Context, obj store.ObjectList, opts ...store.WatchOption) (store.Watcher, error) {
	return nil, errors.NewUnsupported("watch not supported on this storage")
}

func (s *Storage) Scope(conds ...store.Scope) store.Store {
	return &Storage{
		conditions: append(s.conditions, conds...),
		core:       s.core,
	}
}

func (s *Storage) Create(ctx context.Context, in store.Object, options ...store.CreateOption) error {
	option := store.CreateOptions{}
	for _, opt := range options {
		opt(&option)
	}
	return s.core.create(ctx, s.conditions, in, option)
}

func (s *Storage) Get(ctx context.Context, name string, into store.Object, options ...store.GetOption) error {
	option := store.GetOptions{}
	for _, opt := range options {
		opt(&option)
	}
	return s.core.get(ctx, s.conditions, name, into, option)
}

func (s *Storage) Update(ctx context.Context, into store.Object, options ...store.UpdateOption) error {
	option := store.UpdateOptions{}
	for _, opt := range options {
		opt(&option)
	}
	return s.core.update(ctx, s.conditions, into, false, option)
}

func (s *Storage) List(ctx context.Context, list store.ObjectList, options ...store.ListOption) error {
	opts := store.ListOptions{}
	for _, opt := range options {
		opt(&opts)
	}
	return s.core.list(ctx, s.conditions, list, opts)
}

func (s *Storage) Delete(ctx context.Context, into store.Object, options ...store.DeleteOption) error {
	option := store.DeleteOptions{}
	for _, opt := range options {
		opt(&option)
	}
	return s.core.delete(ctx, s.conditions, into, option)
}

type StatusStorage struct {
	core       *core
	conditions []store.Scope
}

// Patch implements store.StatusStorage.
func (s *StatusStorage) Patch(ctx context.Context, obj store.Object, patch store.Patch, opts ...store.PatchOption) error {
	options := store.PatchOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	return s.core.patch(ctx, s.conditions, obj, patch, true, options)
}

// Update implements store.StatusStorage.
func (s *StatusStorage) Update(ctx context.Context, obj store.Object, opts ...store.UpdateOption) error {
	options := store.UpdateOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	return s.core.update(ctx, s.conditions, obj, true, options)
}

var _ store.StatusStorage = &StatusStorage{}

type core struct {
	db     *gorm.DB
	helper *StructHelper
	driver string
}

func (c *core) get(ctx context.Context, scope []store.Scope, name string, into store.Object, options store.GetOptions) error {
	resource, err := store.GetResource(into)
	if err != nil {
		return err
	}
	if name == "" {
		return NewEmptyNameStorageError(resource)
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
	rows, err := db.WithContext(ctx).Where("name = ?", name).Limit(1).Rows()
	if err != nil {
		return mapSQLError(err, resource, name)
	}
	defer rows.Close()

	if !rows.Next() {
		return errors.NewNotFound(resource, name)
	}
	if err := c.helper.ScanOne(rows, into); err != nil {
		return mapSQLError(err, resource, name)
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

func (c *core) create(ctx context.Context, scopes []store.Scope, in store.Object, _ store.CreateOptions) error {
	resource, err := store.GetResource(in)
	if err != nil {
		return err
	}
	name := in.GetName()
	if name == "" {
		return NewEmptyNameStorageError(resource)
	}
	in.SetCreationTimestamp(store.Now())
	save := c.helper.ToDriverValueMap(in)
	for _, cond := range scopes {
		save[cond.Resource] = cond.Name
	}
	if err := c.prepare(ctx, resource, nil).Create(save).Error; err != nil {
		return mapSQLError(err, resource, name)
	}
	return nil
}

var statusAllowedKeys = []string{
	"status",
	"annotations",
	"labels",
	"finalizers",
	"ownerReferences",
}

func (c *core) update(ctx context.Context, scope []store.Scope, into store.Object, status bool, options store.UpdateOptions) error {
	resource, err := store.GetResource(into)
	if err != nil {
		return err
	}
	name := into.GetName()
	if name == "" {
		return NewEmptyNameStorageError(resource)
	}
	save := c.helper.ToDriverValueMap(into)
	maps.DeleteFunc(save, func(key string, _ any) bool {
		return status && !slices.Contains(statusAllowedKeys, key) || !status && key == "status"
	})
	for _, cond := range scope {
		save[cond.Resource] = cond.Name
	}
	db := c.prepare(ctx, resource, scope).Where("name = ?", name)
	if options.FieldRequirements != nil {
		db = c.applyFields(db, options.FieldRequirements)
	}
	if options.LabelRequirements != nil {
		db = c.applyLabels(db, options.LabelRequirements)
	}
	if err := db.Updates(save).Error; err != nil {
		return mapSQLError(err, resource, name)
	}
	return nil
}

func (c *core) patchBatch(ctx context.Context, scope []store.Scope, list store.ObjectList, patch store.PatchBatch, options store.PatchBatchOptions) error {
	return errors.NewUnsupported("patch batch not supported on this storage")
}

func (c *core) patch(ctx context.Context, scope []store.Scope, into store.Object, patch store.Patch, status bool, options store.PatchOptions) error {
	resource, err := store.GetResource(into)
	if err != nil {
		return err
	}
	name := into.GetName()
	if name == "" {
		return NewEmptyNameStorageError(resource)
	}
	patchData, err := patch.Data(into)
	if err != nil {
		return fmt.Errorf("get patch data: %w", err)
	}
	var update map[string]any
	switch ptype := patch.Type(); ptype {
	case store.PatchTypeJSONPatch:
		patchlist := []map[string]any{}
		if err := json.Unmarshal(patchData, &patchlist); err != nil {
			return fmt.Errorf("invalid patch data: %w", err)
		}
		jsonupdate, err := JsonPatchToUpdate(patchlist, nil, nil)
		if err != nil {
			return errors.NewBadRequest(fmt.Sprintf("invalid json patch data: %s, error: %v", string(patchData), err))
		}
		update = jsonupdate
	case store.PatchTypeMergePatch:
		patchmap := map[string]any{}
		if err := json.Unmarshal(patchData, &patchmap); err != nil {
			return fmt.Errorf("invalid patch data: %w", err)
		}
		update = patchmap
	}
	maps.DeleteFunc(update, func(key string, _ any) bool {
		return status && !slices.Contains(statusAllowedKeys, key) || !status && key == "status"
	})
	db := c.prepare(ctx, resource, scope).Where("name = ?", name)
	if options.FieldRequirements != nil {
		db = c.applyFields(db, options.FieldRequirements)
	}
	if options.LabelRequirements != nil {
		db = c.applyLabels(db, options.LabelRequirements)
	}
	if err := db.Updates(update).Error; err != nil {
		return mapSQLError(err, resource, name)
	}
	return nil
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
	name := into.GetName()
	if name == "" {
		return NewEmptyNameStorageError(resource)
	}

	db := c.prepare(ctx, resource, scope)
	if options.FieldRequirements != nil {
		db = c.applyFields(db, options.FieldRequirements)
	}
	if options.LabelRequirements != nil {
		db = c.applyLabels(db, options.LabelRequirements)
	}
	intoV := c.helper.ToDriverValueMap(into)
	if err := db.Where("name = ?", name).Delete(intoV).Error; err != nil {
		return mapSQLError(err, resource, name)
	}
	if db.RowsAffected == 0 {
		return errors.NewNotFound(resource, name)
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
		if len(opts.SearchFields) > 0 {
			// search in specified fields
			conditions := make([]string, 0, len(opts.SearchFields))
			args := make([]any, 0, len(opts.SearchFields))
			for _, field := range opts.SearchFields {
				conditions = append(conditions, fmt.Sprintf("%s LIKE ?", c.quoteKey(field)))
				args = append(args, fmt.Sprintf("%%%s%%", opts.Search))
			}
			db = db.Where(strings.Join(conditions, " OR "), args...)
		} else {
			db = db.Where("name like ?", fmt.Sprintf("%%%s%%", opts.Search))
		}
	}
	if opts.FieldRequirements != nil {
		db = c.applyFields(db, opts.FieldRequirements)
	}
	if opts.LabelRequirements != nil {
		db = c.applyLabels(db, opts.LabelRequirements)
	}
	page, size := opts.Page, opts.Size
	if size > 0 {
		page = max(1, page) // ensure page is at least 1
		db = db.Offset((page - 1) * size).Limit(size)
	}
	// count total
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return mapSQLError(err, resource, "")
	}
	for _, sort := range store.ParseSorts(opts.Sort) {
		if sort.ASC {
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
	list.SetTotal(int(total))
	list.SetSize(int(size))
	list.SetPage(int(page))
	list.SetResource(resource)
	return nil
}

func (c *core) applyLabels(db *gorm.DB, requirements store.Requirements) *gorm.DB {
	for _, req := range requirements {
		key := fmt.Sprintf(`%s -> '$."%s"'`, c.quoteKey("labels"), req.Key)
		db = c.applyCondition(db, key, req.Operator, req.Values)
	}
	return db
}

func (c *core) applyFields(db *gorm.DB, requirements store.Requirements) *gorm.DB {
	for _, req := range requirements {
		key := c.quoteKey(req.Key)
		db = c.applyCondition(db, key, req.Operator, req.Values)
	}
	return db
}

func (c *core) applyCondition(db *gorm.DB, key string, op store.Operator, vals []any) *gorm.DB {
	switch op {
	case store.Equals, store.DoubleEquals:
		return db.Where(key+" = ?", vals[0])
	case store.NotEquals:
		return db.Where(key+" != ?", vals[0])
	case store.In:
		return db.Where(key+" IN ?", vals)
	case store.NotIn:
		return db.Where(key+" NOT IN ?", vals)
	case store.DoesNotExist:
		return db.Where(key + " IS NULL")
	case store.Exists:
		return db.Where(key + " IS NOT NULL")
	case store.GreaterThan:
		return db.Where(key+" > ?", vals[0])
	case store.LessThan:
		return db.Where(key+" < ?", vals[0])
	case store.GreaterThanOrEqual:
		return db.Where(key+" >= ?", vals[0])
	case store.LessThanOrEqual:
		return db.Where(key+" <= ?", vals[0])
	case store.Like:
		return db.Where(key+" LIKE ?", fmt.Sprintf("%%%s%%", vals[0]))
	default:
		return db
	}
}

func (c *core) prepare(ctx context.Context, tablename string, scopes []store.Scope) *gorm.DB {
	db := c.db.WithContext(ctx)
	for _, cond := range scopes {
		key, val := c.quoteKey(cond.Resource), cond.Name
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

func NewEmptyNameStorageError(resource string) error {
	return errors.NewBadRequest(fmt.Sprintf("empty name for resource %s", resource))
}
