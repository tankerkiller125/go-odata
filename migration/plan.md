# Migration Plan: GORM → ent

This document describes every change required to replace GORM with
[ent](https://entgo.io/) throughout the `go-odata` library.  After the
migration is complete, GORM must not appear anywhere in the library — not in
imports, struct tags, or documentation.

The library is designed to sit on top of whatever ent models a consuming
project already defines.  That constraint shapes many of the decisions below:
the library must accept the caller's `*ent.Client`, work with the caller's
generated types, and never force a particular schema shape on the caller.

---

## Table of Contents

1. [Inventory of GORM Usage](#1-inventory-of-gorm-usage)
2. [Architectural Changes](#2-architectural-changes)
3. [Public API Changes](#3-public-api-changes)
4. [Internal Package Changes](#4-internal-package-changes)
5. [Struct Tag Migration](#5-struct-tag-migration)
6. [Internal Storage Models](#6-internal-storage-models)
7. [cmd/ Server Changes](#7-cmd-server-changes)
8. [Test Changes](#8-test-changes)
9. [Dependency Changes](#9-dependency-changes)
10. [Migration Checklist](#10-migration-checklist)

---

## 1. Inventory of GORM Usage

### 1.1 Root package (`odata.go`, `transaction_context.go`, `geospatial.go`)

| File | Usage |
|---|---|
| `odata.go` | Imports `gorm.io/gorm`; `Service.db *gorm.DB`; `NewService(db *gorm.DB)`; `NewServiceWithConfig(db *gorm.DB, cfg ServiceConfig)`; `ReadHook` interface uses `[]func(*gorm.DB) *gorm.DB` return types; doc comments reference `*gorm.DB` scopes |
| `transaction_context.go` | `TransactionFromContext(ctx) (*gorm.DB, bool)` — exposes the active transaction to hook authors |
| `geospatial.go` | `checkGeospatialSupport(db *gorm.DB, ...)` — uses `db.Dialector.Name()` to detect the database dialect |

### 1.2 `internal/handlers/`

| File | Usage |
|---|---|
| `entity.go` | `EntityHandler.db *gorm.DB`; `NewEntityHandler(db *gorm.DB, ...)` |
| `entity_read.go` | `db.WithContext`, `db.First`, `db.Session`, `gorm.ErrRecordNotFound` |
| `entity_write.go` | `tx.Delete`, `tx.First`, `tx.Model(...).Updates(...)`, `tx.Model(...).Select("*").Updates(...)` |
| `entity_media.go` | `gorm.ErrRecordNotFound`; `db.Session(&gorm.Session{...})` |
| `entity_helpers.go` | `fetchEntityByKey(... scopes []func(*gorm.DB) *gorm.DB)`; `fetchAndVerifyEntity(db *gorm.DB, ...)` |
| `collection_read.go` | `db.WithContext`, `db.Find`, `gorm/clause`; scope functions `[]func(*gorm.DB) *gorm.DB`; skip-token filter builds raw `db.Where` calls |
| `collection_write.go` | `tx.Create` |
| `collection_count.go` | `db.WithContext`, `db.Model(...).Count(...)` |
| `collection_executor.go` | `BeforeRead func(*query.QueryOptions) ([]func(*gorm.DB) *gorm.DB, error)`; `CountFunc`; `FetchFunc` — all parameterised with `*gorm.DB` scopes |
| `navigation_properties.go` | `buildNavigationRelatedQuery(...) *gorm.DB`; `createNavBeforeRead` / `createNavCountFunc` / `createNavFetchFunc` — return closures over `*gorm.DB` |
| `structural_properties.go` | `applyStructuralPropertySelect(db *gorm.DB, ...) *gorm.DB`; `gorm.ErrRecordNotFound` |
| `stream_properties.go` | `applyStreamPropertySelect(db *gorm.DB, ...) *gorm.DB` |
| `complex_properties.go` | `var db *gorm.DB` |
| `bind.go` | All helper methods accept `db *gorm.DB`; `gorm.ErrRecordNotFound` |
| `transaction.go` | `runInTransaction(... fn func(tx *gorm.DB, ...) error)` |
| `hooks.go` | `callBeforeReadCollection` / `callBeforeReadEntity` return `[]func(*gorm.DB) *gorm.DB` |
| `context.go` | `withTransaction(ctx, tx *gorm.DB)`; `TransactionFromContext` returns `*gorm.DB` |
| `batch.go` | `BatchHandler.db *gorm.DB`; `NewBatchHandler(db *gorm.DB, ...)`; `executeRequestInTransaction(... tx *gorm.DB, ...)` |
| `helpers.go` | `buildKeyQuery(db *gorm.DB, ...) *gorm.DB` |

### 1.3 `internal/query/`

| File | Usage |
|---|---|
| `applier.go` | `ApplyQueryOptions(db *gorm.DB, ...) *gorm.DB`; `ApplyQueryOptionsWithFTS`; `ApplyFilterOnly`; `ApplyExpandOnly`; `applyOffsetWithLimit` |
| `apply_filter.go` | `getDatabaseDialect(db *gorm.DB)`; all filter-building functions take / return `*gorm.DB`; uses `db.Where`, `db.Joins` |
| `apply_select.go` | `applySelect(db *gorm.DB, ...) *gorm.DB`; uses `db.Select` |
| `apply_expand.go` | `applyExpand(db *gorm.DB, ...) *gorm.DB`; `db.Preload`; `ApplyExpandOption` |
| `expand_per_parent.go` | `ApplyPerParentExpand(db *gorm.DB, ...)`; `db.Session(&gorm.Session{NewDB: true})`; `applyParentKeyFilter(db *gorm.DB, ...) *gorm.DB` |
| `apply_transform.go` | `applyTransformations(db *gorm.DB, ...) *gorm.DB`; `gorm.io/gorm/clause`; all group-by, aggregate, compute, and order-by helpers |
| `logger.go` | `setLoggerInDB(db *gorm.DB, ...) *gorm.DB`; uses `db.Set(key, val)` |
| `fts.go` | `FTSManager.db *gorm.DB`; `NewFTSManager(db *gorm.DB)`; `db.Dialector.Name()` for dialect; `gorm.ErrRecordNotFound`; `ApplyFTSSearch(db *gorm.DB, ...) (*gorm.DB, error)` |

### 1.4 `internal/observability/gorm.go`

Entire file is GORM-specific: registers GORM before/after callbacks for query
tracing and server-timing metrics.

### 1.5 `internal/trackchanges/tracker.go`

* `Tracker.db *gorm.DB`
* `NewTrackerWithDB(db *gorm.DB)`
* `changeRecord` struct carries `gorm:"..."` field tags
* `db.AutoMigrate`, `db.Create`, `db.Find`, `db.Order`

### 1.6 `internal/async/manager.go` and `job_record.go`

* `Manager.db *gorm.DB`
* `NewManager(db *gorm.DB, ...)`
* `JobRecord` struct carries `gorm:"..."` field tags
* `db.AutoMigrate`, `db.Create`, `db.Model`, `db.First`, `db.Where`, `db.Delete`

### 1.7 `internal/metadata/analyzer.go`

* `PropertyMetadata.GormTag string` — stores the raw `gorm:"..."` tag for
  each field.
* Every column-name and foreign-key lookup parses `gorm:"column:..."`,
  `gorm:"foreignKey:..."`, `gorm:"references:..."`, `gorm:"many2many:..."`,
  `gorm:"embedded"`, `gorm:"embeddedPrefix:..."`.
* Nullability logic reads `gorm:"not null"` and `gorm:"default:..."`.
* Auto-increment detection reads `gorm:"autoincrement"` and
  `gorm:"autoincrement:false"`.

---

## 2. Architectural Changes

### 2.1 The core abstraction problem

GORM exposes a single generic `*gorm.DB` query builder that can target any
table.  ent generates a **type-safe client per entity** — `*ent.ProductClient`,
`*ent.CategoryClient`, etc.  There is no single generic handle that works for
all entities.

This means the library cannot simply swap `*gorm.DB` for `*ent.Client`.
Instead it must adopt **one of the following strategies**, and the chosen
strategy drives every other decision.

#### Strategy A — Database/SQL Passthrough (Recommended)

Use ent to obtain the underlying `*sql.DB` handle (via `client.Driver().DB()`
from the `entgo.io/ent/dialect/sql` package) and build queries with raw SQL,
`database/sql`, or a lightweight SQL builder (e.g. `squirrel`).

* The library never calls any ent-generated API directly.
* Struct tags shift from `gorm:"..."` to custom `odata:"..."` tags (column
  names, foreign keys, etc.) or the library reflects on `ent` schema metadata
  at startup.
* The caller passes `*ent.Client`; the library extracts `*sql.DB` and the
  dialect string, then runs its own SQL.

**Pros:** Keeps the query layer generic across any ent-backed database;
no dependency on generated code inside the library.  
**Cons:** Requires rewriting every query-building function to use raw SQL or a
SQL builder.

#### Strategy B — Overwrite/Delegate Everything

Remove all automatic database operations and require the caller to provide
overwrite handlers for every CRUD operation.

* `NewService` accepts no database handle.
* Callers implement the `Overwrite` handlers using their own ent client.

**Pros:** Zero ORM coupling inside the library.  
**Cons:** Eliminates automatic CRUD — callers must implement everything manually,
which removes the primary value proposition of the library.

#### Strategy C — ent Generic Query Builder

Use the ent `dialect/sql` generic node API to build queries without generated
types.

* Queries are expressed through `sqlgraph` or `dialect/sql` predicates.
* The library would call `entClient.Client()` to access the generic driver.

**Pros:** Stays within the ent ecosystem.  
**Cons:** ent's generic APIs are lower-level and less stable; complex joins and
aggregates are cumbersome.

**The plan below assumes Strategy A (Database/SQL Passthrough)** as it
provides the least coupling and the most flexibility for callers.

### 2.2 Dialect detection

GORM exposes `db.Dialector.Name()`.  With Strategy A, after extracting
`*sql.DB` from `*ent.Client`, the dialect must be passed explicitly or probed
by querying the SQL driver name (`sql.DB.Driver()` reflection or a known
mapping).  A new `Dialect` field (or `DialectFunc`) should be added to
`ServiceConfig`.

---

## 3. Public API Changes

### 3.1 `odata.go`

#### `NewService` / `NewServiceWithConfig`

**Before:**
```go
func NewService(db *gorm.DB) (*Service, error)
func NewServiceWithConfig(db *gorm.DB, cfg ServiceConfig) (*Service, error)
```

**After:**
```go
func NewService(db *sql.DB, dialect string) (*Service, error)
func NewServiceWithConfig(db *sql.DB, dialect string, cfg ServiceConfig) (*Service, error)
```

Or, using an `EntAdapter` helper that wraps `*ent.Client`:

```go
// EntAdapter extracts what go-odata needs from an ent client.
type EntAdapter struct {
    DB      *sql.DB
    Dialect string
}

func NewEntAdapter(client interface{ Driver() dialect.Driver }) (*EntAdapter, error)

func NewService(adapter *EntAdapter) (*Service, error)
func NewServiceWithConfig(adapter *EntAdapter, cfg ServiceConfig) (*Service, error)
```

Using the adapter approach is preferred — it keeps the public API idiomatic
and avoids importing `database/sql` directly.

#### `Service.db` field

Change from `*gorm.DB` to `*sql.DB` (unexported).

#### `ReadHook` interface

**Before:**
```go
type ReadHook interface {
    ODataBeforeReadCollection(ctx context.Context, r *http.Request, opts *QueryOptions) ([]func(*gorm.DB) *gorm.DB, error)
    ODataBeforeReadEntity(ctx context.Context, r *http.Request, opts *QueryOptions) ([]func(*gorm.DB) *gorm.DB, error)
}
```

The GORM scope pattern (`[]func(*gorm.DB) *gorm.DB`) has no direct ent
equivalent.  Replace with a `QueryScope` abstraction that carries raw SQL
predicates:

**After:**
```go
// QueryScope represents a SQL condition that may be added to a query.
// Use NewQueryScope to construct one.
type QueryScope struct {
    Condition string
    Args      []interface{}
}

type ReadHook interface {
    ODataBeforeReadCollection(ctx context.Context, r *http.Request, opts *QueryOptions) ([]QueryScope, error)
    ODataBeforeReadEntity(ctx context.Context, r *http.Request, opts *QueryOptions) ([]QueryScope, error)
}
```

All callers (including internal hooks) that previously returned
`[]func(*gorm.DB) *gorm.DB` must be updated to return `[]QueryScope`.

#### `ServiceConfig.RegisterGORMCallbacks` / observability

Remove `RegisterGORMCallbacks` and `RegisterServerTimingCallbacks` from the
public surface.  Replace with a new `RegisterDBHooks(db *sql.DB, cfg ...)` or
use `database/sql`'s `driver.QueryerContext`-wrapping approach (e.g.
`sqlhooks`) for tracing and server timing.

### 3.2 `transaction_context.go`

**Before:**
```go
func TransactionFromContext(ctx context.Context) (*gorm.DB, bool)
```

**After:**
```go
// TransactionFromContext returns the active *sql.Tx stored for hook execution.
func TransactionFromContext(ctx context.Context) (*sql.Tx, bool)
```

Hook authors who previously called `tx.Create(...)` on a `*gorm.DB` must
instead use the standard `*sql.Tx` interface or their ent client's transaction
variant (`ent.NewTxFromClient` / `client.BeginTx`).

---

## 4. Internal Package Changes

### 4.1 `internal/query/`

This is the largest change surface.  Every function signature that carries
`*gorm.DB` must be refactored.

#### Proposed internal query type

Introduce a new unexported `queryBuilder` struct (in `internal/query/`) that
wraps `*sql.DB` and accumulates clauses:

```go
type queryBuilder struct {
    db        *sql.DB
    dialect   string
    table     string
    wheres    []clause         // {sql string, args}
    havings   []clause
    joins     []string
    selects   []string
    orderBys  []string
    limit     *int
    offset    int
    logger    *slog.Logger
    instState map[string]any   // replaces db.InstanceSet / db.Set
}
```

All existing `applyXxx(db *gorm.DB, ...) *gorm.DB` functions become
`applyXxx(qb *queryBuilder, ...) *queryBuilder` and operate on this struct.

The final query is materialised by `qb.ToSQL()` (returns a parameterised SQL
string + args) which is then executed via `db.QueryContext` / `db.ExecContext`.

#### `applier.go`

Replace `ApplyQueryOptions(db *gorm.DB, ...)` with
`ApplyQueryOptions(qb *queryBuilder, ...)`.

#### `apply_filter.go`

* Remove `getDatabaseDialect(db *gorm.DB)` — dialect is now stored on
  `queryBuilder.dialect`.
* All `buildFilterConditionWithDB(db *gorm.DB, ...)` functions become
  `buildFilterConditionWithDialect(dialect string, ...)`.

#### `apply_select.go`

`applySelect` populates `qb.selects`.

#### `apply_expand.go`

GORM's `db.Preload(name, func(*gorm.DB)*gorm.DB)` is not available in raw
SQL.  Navigation property expansion must shift to post-query in-memory
association loading (as `ApplyPerParentExpand` already partially does):

* Remove `applyExpand` which relies on GORM preloads.
* Always use `ApplyPerParentExpand` style loading: run parent query, collect
  keys, run child queries per navigation property.
* `ApplyExpandOption` becomes a pure SQL query runner for a given child entity
  table.

#### `expand_per_parent.go`

Remove `gorm.Session{NewDB: true}` usage.  Replace with `db.QueryContext` on
a fresh `*sql.DB` connection.

#### `apply_transform.go`

* Remove `gorm.io/gorm/clause` import — reimplement `ORDER BY` building with
  string construction.
* `setAliasExprInDB` / `getAliasExprFromDB` currently abuse `db.Set` /
  `db.Get` to store per-query state.  Move this state into `queryBuilder.instState`.

#### `logger.go`

Remove `setLoggerInDB` — the logger is stored in `queryBuilder.logger` directly.

#### `fts.go`

* Remove `FTSManager.db *gorm.DB`.  Store `*sql.DB` and `dialect string`
  instead.
* `NewFTSManager(db *sql.DB, dialect string)`.
* `gorm.ErrRecordNotFound` check becomes `errors.Is(err, sql.ErrNoRows)`.
* `ApplyFTSSearch` populates the `queryBuilder` with FTS-specific WHERE
  clauses instead of returning `*gorm.DB`.

### 4.2 `internal/handlers/`

#### `entity.go`

```go
// Before
type EntityHandler struct {
    db *gorm.DB
    ...
}
func NewEntityHandler(db *gorm.DB, ...) *EntityHandler

// After
type EntityHandler struct {
    db      *sql.DB
    dialect string
    ...
}
func NewEntityHandler(db *sql.DB, dialect string, ...) *EntityHandler
```

#### `transaction.go`

```go
// Before
func (h *EntityHandler) runInTransaction(ctx context.Context, r *http.Request, fn func(tx *gorm.DB, hookReq *http.Request) error) error

// After
func (h *EntityHandler) runInTransaction(ctx context.Context, r *http.Request, fn func(tx *sql.Tx, hookReq *http.Request) error) error
```

All write operations (`INSERT`, `UPDATE`, `DELETE`) inside the closure use the
`*sql.Tx` directly via parameterised `tx.ExecContext` calls.

#### `context.go`

```go
// Before
func withTransaction(ctx context.Context, tx *gorm.DB) context.Context
func TransactionFromContext(ctx context.Context) (*gorm.DB, bool)

// After
func withTransaction(ctx context.Context, tx *sql.Tx) context.Context
func TransactionFromContext(ctx context.Context) (*sql.Tx, bool)
```

#### `hooks.go`

`callBeforeReadCollection` and `callBeforeReadEntity` return `[]QueryScope`
instead of `[]func(*gorm.DB) *gorm.DB`.

The `invokeReadHook` reflection code that type-asserts `[]func(*gorm.DB)*gorm.DB`
must be updated to assert `[]QueryScope`.

#### `collection_executor.go`

```go
// Before
type collectionExecutor struct {
    BeforeRead func(*query.QueryOptions) ([]func(*gorm.DB) *gorm.DB, error)
    CountFunc  func(*query.QueryOptions, []func(*gorm.DB) *gorm.DB) (*int64, error)
    FetchFunc  func(*query.QueryOptions, []func(*gorm.DB) *gorm.DB) (interface{}, error)
}

// After
type collectionExecutor struct {
    BeforeRead func(*query.QueryOptions) ([]QueryScope, error)
    CountFunc  func(*query.QueryOptions, []QueryScope) (*int64, error)
    FetchFunc  func(*query.QueryOptions, []QueryScope) (interface{}, error)
}
```

#### `collection_read.go`

* Remove `gorm.io/gorm/clause` import.
* `fetchResults` and `collectionCountFunc` build `queryBuilder` objects
  instead of chaining on `*gorm.DB`.
* `buildTypeCastScope` returns a `QueryScope` instead of a
  `func(*gorm.DB) *gorm.DB`.
* `applySkipTokenFilter(db *gorm.DB, ...) *gorm.DB` becomes
  `applySkipTokenFilter(qb *queryBuilder, ...) *queryBuilder`.

#### `entity_read.go`, `entity_write.go`, `entity_media.go`

Replace all `db.First(...)`, `tx.Create(...)`, `tx.Model(...).Updates(...)`,
`tx.Delete(...)` calls with explicit parameterised SQL via
`db.QueryRowContext` / `tx.ExecContext`.

`gorm.ErrRecordNotFound` checks become `errors.Is(err, sql.ErrNoRows)`.

#### `entity_helpers.go`

```go
// Before
func (h *EntityHandler) fetchEntityByKey(ctx context.Context, key string, opts *query.QueryOptions, scopes []func(*gorm.DB) *gorm.DB) (interface{}, error)

// After
func (h *EntityHandler) fetchEntityByKey(ctx context.Context, key string, opts *query.QueryOptions, scopes []QueryScope) (interface{}, error)
```

#### `navigation_properties.go`

```go
// Before
func (h *EntityHandler) buildNavigationRelatedQuery(parent interface{}, target *metadata.EntityMetadata) *gorm.DB
func (h *EntityHandler) createNavBeforeRead(r *http.Request, target *metadata.EntityMetadata) func(*query.QueryOptions) ([]func(*gorm.DB) *gorm.DB, error)

// After
func (h *EntityHandler) buildNavigationRelatedQuery(parent interface{}, target *metadata.EntityMetadata) *query.queryBuilder
func (h *EntityHandler) createNavBeforeRead(r *http.Request, target *metadata.EntityMetadata) func(*query.QueryOptions) ([]QueryScope, error)
```

#### `structural_properties.go`, `stream_properties.go`

Replace `applyStructuralPropertySelect(db *gorm.DB, ...) *gorm.DB` and
`applyStreamPropertySelect(db *gorm.DB, ...) *gorm.DB` with equivalents on
`*queryBuilder`.

#### `bind.go`

All `db *gorm.DB` parameters become `tx *sql.Tx`.
`gorm.ErrRecordNotFound` becomes `sql.ErrNoRows`.

#### `helpers.go`

`buildKeyQuery(db *gorm.DB, key string) (*gorm.DB, error)` becomes
`buildKeyQuery(qb *query.queryBuilder, key string) (*query.queryBuilder, error)`.

#### `complex_properties.go`

Replace `var db *gorm.DB` variable with `var qb *queryBuilder`.

#### `batch.go`

```go
// Before
type BatchHandler struct { db *gorm.DB }
func NewBatchHandler(db *gorm.DB, ...) *BatchHandler
func (h *BatchHandler) executeRequestInTransaction(req *batchRequest, tx *gorm.DB, ...) batchResponse

// After
type BatchHandler struct { db *sql.DB }
func NewBatchHandler(db *sql.DB, ...) *BatchHandler
func (h *BatchHandler) executeRequestInTransaction(req *batchRequest, tx *sql.Tx, ...) batchResponse
```

### 4.3 `internal/observability/`

`internal/observability/gorm.go` must be replaced entirely.

* Remove all GORM callback registration (`db.Callback().Query().Before(...)`,
  etc.).
* Implement database tracing and server-timing using `sqlhooks`
  (`github.com/qustavo/sqlhooks`) or a custom `driver.Driver` wrapper so that
  the same hooks fire regardless of the underlying driver.
* Expose:

```go
// RegisterDBHooks wraps the provided driver to inject tracing and timing hooks.
func RegisterDBHooks(driverName, dataSourceName string, cfg *Config) (wrappedDriverName string, err error)
```

Rename the file to `internal/observability/sql.go`.

### 4.4 `internal/trackchanges/tracker.go`

The `changeRecord` persistent storage relies on GORM's `AutoMigrate` and
tagged structs.

**Options:**
1. Keep the in-memory implementation (`NewTracker()`) as-is (no GORM needed).
2. For persistent tracking (`NewTrackerWithDB`), replace GORM with raw SQL:
   * `CREATE TABLE IF NOT EXISTS` on startup instead of `AutoMigrate`.
   * Remove `gorm:"..."` struct tags from `changeRecord`.
   * Use `db.ExecContext` for `INSERT` and `db.QueryContext` for `SELECT`.

The `Tracker` signature changes:

```go
// Before
func NewTrackerWithDB(db *gorm.DB) (*Tracker, error)

// After
func NewTrackerWithDB(db *sql.DB) (*Tracker, error)
```

### 4.5 `internal/async/manager.go` and `job_record.go`

Same pattern as the tracker:

* `JobRecord` struct: remove all `gorm:"..."` tags.
* Replace `db.AutoMigrate(&JobRecord{})` with explicit `CREATE TABLE IF NOT
  EXISTS` DDL.
* Replace all chained GORM calls with parameterised `db.ExecContext` /
  `db.QueryRowContext` / `db.QueryContext`.

```go
// Before
func NewManager(db *gorm.DB, ttl time.Duration, opts ...ManagerOption) (*Manager, error)

// After
func NewManager(db *sql.DB, dialect string, ttl time.Duration, opts ...ManagerOption) (*Manager, error)
```

`gorm.ErrRecordNotFound` becomes `sql.ErrNoRows`.

### 4.6 `internal/metadata/analyzer.go`

The metadata analyzer currently relies on GORM struct tags for:

| GORM Tag | Purpose | ent / replacement |
|---|---|---|
| `gorm:"column:name"` | Database column name | Add `odata:"column:name"` tag, or derive from `ent` schema field name |
| `gorm:"primaryKey"` | Mark key field | Already superseded by `odata:"key"` tag |
| `gorm:"autoincrement"` / `gorm:"autoincrement:false"` | Whether key is DB-generated | Add `odata:"autoincrement"` / `odata:"autoincrement:false"` tags |
| `gorm:"not null"` | Field is not nullable | Add `odata:"nullable=false"` tag (already partially supported) |
| `gorm:"default:…"` | Field has a DB default (affects nullability) | Add `odata:"default"` tag |
| `gorm:"foreignKey:…"` | Relationship FK column | Add `odata:"foreignKey:…"` tag |
| `gorm:"references:…"` | Relationship reference column | Add `odata:"references:…"` tag |
| `gorm:"many2many:…"` | Many-to-many join table | Add `odata:"many2many:…"` tag |
| `gorm:"embedded"` | Embedded struct | Add `odata:"embedded"` tag |
| `gorm:"embeddedPrefix:…"` | Embedded prefix | Add `odata:"embeddedPrefix:…"` tag |

**Changes required:**

1. Remove `PropertyMetadata.GormTag string` field.
2. Add the new `odata` sub-tags listed above (many are already partly
   supported by the existing `odata:"..."` tag parser).
3. Update `extractReferentialConstraints`, `extractEmbeddedPrefix`,
   `hasGormNotNull`, `hasGormDefault`, `isDatabaseGenerated`, and
   `computeColumnName` to read from `odata` tags instead.
4. Provide a compatibility note for callers who currently define structs with
   `gorm:"..."` tags: those tags will no longer be read; callers must add
   `odata:"column:…"` etc. where the GORM-derived default would differ from
   snake_case.

---

## 5. Struct Tag Migration

All entity structs in consuming projects that rely on GORM tags for OData
behaviour must be updated.

### 5.1 Column names

```go
// Before (gorm-derived column name)
Name string `gorm:"column:product_name" json:"name"`

// After
Name string `odata:"column:product_name" json:"name"`
```

Fields where the GORM default (snake_case of the field name) matches the
actual column name require no change.

### 5.2 Primary key / autoincrement

```go
// Before
ID int `gorm:"primaryKey;autoincrement" odata:"key"`

// After
ID int `odata:"key;autoincrement"`
```

### 5.3 Nullability

```go
// Before
Price *float64 `gorm:"not null"`

// After
Price *float64 `odata:"nullable=false"`
```

### 5.4 Relationships (navigation properties)

```go
// Before
Products []Product `gorm:"foreignKey:CategoryID;references:ID"`

// After
Products []Product `odata:"foreignKey:CategoryID;references:ID"`
```

### 5.5 Many-to-many

```go
// Before
Tags []Tag `gorm:"many2many:product_tags"`

// After
Tags []Tag `odata:"many2many:product_tags"`
```

### 5.6 Embedded structs

```go
// Before
Address Address `gorm:"embedded;embeddedPrefix:addr_"`

// After
Address Address `odata:"embedded;embeddedPrefix:addr_"`
```

### 5.7 Internal structs

The library's own internal structs (`changeRecord` in `trackchanges/tracker.go`
and `JobRecord` in `async/job_record.go`) use `gorm:"..."` tags for schema
migration.  Because these tables are managed by the library's own DDL (see
§4.4 and §4.5), those tags are no longer needed and will be removed entirely.

---

## 6. Internal Storage Models

The library maintains two internal tables:

| Table | Purpose | File |
|---|---|---|
| `_odata_change_records` | Persistent delta/change tracking | `internal/trackchanges/tracker.go` |
| `_odata_async_jobs` | Async job state | `internal/async/job_record.go` |

Both currently rely on GORM `AutoMigrate`.  After the migration:

* Each package will expose an `EnsureSchema(ctx context.Context, db *sql.DB, dialect string) error` function.
* That function runs `CREATE TABLE IF NOT EXISTS` DDL appropriate for the
  dialect.
* Callers who want these features call `EnsureSchema` at startup.
* The `gorm:"..."` struct tags are removed from both structs.

---

## 7. cmd/ Server Changes

The `cmd/devserver`, `cmd/complianceserver`, and `cmd/perfserver` directories
contain example/reference server implementations.  These are the primary
users of the GORM API and will need to be rewritten to use ent.

### 7.1 Entity struct tags

All structs in `cmd/*/entities/` that use `gorm:"..."` tags must be migrated
to `odata:"..."` tags per §5.

### 7.2 `main.go` in each cmd

* Replace `gorm.Open(...)` with an ent client initialisation.
* Replace `db.AutoMigrate(...)` with ent schema migration (`client.Schema.Create(ctx)`).
* Pass the extracted `*sql.DB` and dialect string (or `EntAdapter`) to
  `odata.NewService`.

### 7.3 `reseed.go` in each cmd

Replace all GORM `db.Create`, `db.Delete`, `db.Where`, etc. with equivalent
ent builder calls using the generated client.

### 7.4 `actions_functions.go` in each cmd

Any GORM calls inside action/function handlers must be replaced with ent
client calls.

### 7.5 `internal/observability/gorm.go` usage in cmd

`RegisterGORMCallbacks` / `RegisterServerTimingCallbacks` calls in each
server must be replaced with the new `RegisterDBHooks` API (§4.3).

---

## 8. Test Changes

### 8.1 Unit tests (`internal/*/`)

Every test file that sets up a `*gorm.DB` in-process (typically
`gorm.Open(sqlite.Open(":memory:"), ...)`) must be updated to either:

* Open a `*sql.DB` with the `mattn/go-sqlite3` driver directly, or
* Use `enttest.Open` to spin up a test ent client and pass the extracted
  `*sql.DB`.

### 8.2 Integration tests (`test/`)

`test/test_helpers_test.go` (or equivalent setup file) likely creates a
shared GORM database for all integration tests.  This must be replaced with:

```go
// Before
func newTestDB(t *testing.T) *gorm.DB {
    db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    return db
}

// After
func newTestDB(t *testing.T) (*sql.DB, string) {
    db, err := sql.Open("sqlite3", ":memory:")
    ...
    return db, "sqlite"
}
```

Entity structs used exclusively in tests must have their `gorm:"..."` tags
replaced with `odata:"..."` tags per §5.

### 8.3 Hook tests

Tests that assert `TransactionFromContext` returns a `*gorm.DB` must be
updated to assert `*sql.Tx`.

Tests that provide `ODataBeforeReadCollection` / `ODataBeforeReadEntity` hook
implementations returning `[]func(*gorm.DB) *gorm.DB` must be updated to
return `[]QueryScope`.

---

## 9. Dependency Changes

### 9.1 Remove from `go.mod`

```
gorm.io/gorm
gorm.io/driver/sqlite
gorm.io/driver/postgres
github.com/jinzhu/inflection
github.com/jinzhu/now
```

Remove any transitive-only dependencies that were brought in solely by GORM
(e.g. `github.com/mattn/go-sqlite3` if no longer used by tests — see §8.1).

### 9.2 Add to `go.mod` (root library)

No new ORM dependency is required for the core library under Strategy A.
The library only needs `database/sql` from the standard library.

Optional additions:
* `github.com/qustavo/sqlhooks` (or equivalent) for the observability layer
  (§4.3).

### 9.3 Test dependencies

Tests will need a direct SQLite driver:
```
github.com/mattn/go-sqlite3
```
(or `modernc.org/sqlite` for a CGo-free alternative).

### 9.4 `cmd/` dependencies

Each `cmd/` module gains:
```
entgo.io/ent
```
and the appropriate ent database driver.

---

## 10. Migration Checklist

Work should be completed in the following order to keep the codebase
buildable at each step:

- [ ] **Step 1 – Extend `odata` struct tags in `internal/metadata/analyzer.go`**
  - Add parsing for `odata:"column:…"`, `odata:"autoincrement"`,
    `odata:"autoincrement:false"`, `odata:"foreignKey:…"`,
    `odata:"references:…"`, `odata:"many2many:…"`, `odata:"embedded"`,
    `odata:"embeddedPrefix:…"`, `odata:"nullable=false"`, `odata:"default"`.
  - Keep GORM tag parsing temporarily so both work in parallel during transition.

- [ ] **Step 2 – Introduce `QueryScope` type in public API (`odata.go`)**
  - Define `QueryScope` struct with `Condition string` and `Args []interface{}`.
  - Update `ReadHook` interface to use `[]QueryScope`.
  - Keep backward-compat shim for `[]func(*gorm.DB)*gorm.DB` behind a build
    tag if needed.

- [ ] **Step 3 – Introduce `EntAdapter` and update `NewService`**
  - Add `EntAdapter` type.
  - Add `NewService(adapter *EntAdapter)` and `NewServiceWithConfig`.
  - Keep old `NewService(db *gorm.DB)` temporarily behind a `//go:build gorm`
    tag or remove immediately if a clean break is preferred.

- [ ] **Step 4 – Implement internal `queryBuilder`**
  - Create `internal/query/builder.go` with the `queryBuilder` struct.
  - Implement `ToSQL() (string, []interface{})`.

- [ ] **Step 5 – Migrate `internal/query/` functions**
  - `apply_filter.go` — remove GORM dependency; use `queryBuilder`.
  - `apply_select.go` — remove GORM dependency.
  - `apply_expand.go` — remove GORM preload; rely on per-parent expansion.
  - `expand_per_parent.go` — remove `gorm.Session`; use raw SQL.
  - `apply_transform.go` — remove `gorm/clause`; build ORDER BY strings.
  - `applier.go` — update all exported functions.
  - `logger.go` — move logger storage into `queryBuilder`.
  - `fts.go` — replace `*gorm.DB` with `*sql.DB` + dialect.

- [ ] **Step 6 – Migrate `internal/handlers/`**
  - `entity.go`, `batch.go` — change `*gorm.DB` fields to `*sql.DB`.
  - `context.go`, `transaction.go` — swap `*gorm.DB` for `*sql.Tx`.
  - `hooks.go` — update scope return types.
  - `collection_executor.go`, `collection_read.go` — update scope signatures.
  - `entity_read.go`, `entity_write.go`, `entity_media.go` — replace GORM
    method chains with `queryBuilder` / raw SQL.
  - `entity_helpers.go`, `helpers.go`, `navigation_properties.go`,
    `structural_properties.go`, `stream_properties.go`, `complex_properties.go`,
    `bind.go` — update all remaining GORM references.

- [ ] **Step 7 – Migrate `internal/observability/`**
  - Remove `gorm.go`; create `sql.go` using `sqlhooks` or a custom driver
    wrapper.

- [ ] **Step 8 – Migrate `internal/trackchanges/`**
  - Remove GORM tags from `changeRecord`.
  - Implement `EnsureSchema(ctx, db, dialect)`.
  - Replace GORM calls with raw SQL.

- [ ] **Step 9 – Migrate `internal/async/`**
  - Remove GORM tags from `JobRecord`.
  - Implement `EnsureSchema(ctx, db, dialect)`.
  - Replace GORM calls with raw SQL.

- [ ] **Step 10 – Migrate `geospatial.go`**
  - Replace `checkGeospatialSupport(db *gorm.DB, ...)` with
    `checkGeospatialSupport(db *sql.DB, dialect string, ...)`.

- [ ] **Step 11 – Remove GORM tag fallback from metadata analyzer**
  - Once all entity structs (including tests and cmd/) have been migrated to
    `odata:"..."` tags, remove the GORM tag reading code added in Step 1.

- [ ] **Step 12 – Migrate `cmd/` servers**
  - Replace GORM bootstrapping with ent in each server.
  - Update entity structs to use `odata:"..."` tags.
  - Rewrite `reseed.go` and `actions_functions.go` using ent builders.

- [ ] **Step 13 – Migrate tests**
  - Update all unit test DB setup.
  - Update integration test helpers.
  - Update hook tests for new scope types and `*sql.Tx`.

- [ ] **Step 14 – Remove GORM from `go.mod`**
  - Delete all GORM `require` entries.
  - Run `go mod tidy`.

- [ ] **Step 15 – Update documentation and README**
  - Remove all GORM references from `README.md`, doc comments, and the
    `documentation/examples/` directory.
  - Update `odata.go` doc comments that reference `*gorm.DB` scopes.
  - Add usage examples showing how to pass an `EntAdapter` and define entity
    structs with ent-compatible `odata` tags.
