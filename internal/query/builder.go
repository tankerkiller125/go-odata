package query

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
)

// queryBuilder accumulates SQL clauses for building SELECT queries.
// It replaces GORM's chainable query interface with a database-agnostic
// SQL builder during the migration to database/sql.
//
// Phase 2 Implementation Note:
// This builder is created as part of Phase 2 of the GORM → database/sql migration.
// It provides the foundation for query building without GORM, but is not yet fully
// integrated into the codebase. Full integration will happen in subsequent phases
// when internal/handlers/ is also migrated. The unused warnings are expected and
// will be resolved as the migration progresses.
//
//nolint:unused // Part of Phase 2 migration infrastructure
type queryBuilder struct {
	db        *sql.DB
	dialect   string
	table     string
	wheres    []whereClause
	havings   []whereClause
	joins     []string
	selects   []string
	groupBys  []string
	orderBys  []string
	limit     *int
	offset    int
	logger    *slog.Logger
	instState map[string]any
}

// whereClause represents a SQL condition with parameterized arguments
//
//nolint:unused // Part of Phase 2 migration infrastructure
type whereClause struct {
	sql  string
	args []interface{}
}

// newQueryBuilder creates a new query builder for the given database and dialect
//
//nolint:unused // Part of Phase 2 migration infrastructure
func newQueryBuilder(db *sql.DB, dialect string) *queryBuilder {
	return &queryBuilder{
		db:        db,
		dialect:   dialect,
		instState: make(map[string]any),
		logger:    slog.Default(),
	}
}

// WithTable sets the target table for the query
func (qb *queryBuilder) WithTable(table string) *queryBuilder {
	qb.table = table
	return qb
}

// Where adds a WHERE condition to the query
func (qb *queryBuilder) Where(sql string, args ...interface{}) *queryBuilder {
	qb.wheres = append(qb.wheres, whereClause{sql: sql, args: args})
	return qb
}

// Having adds a HAVING condition to the query
func (qb *queryBuilder) Having(sql string, args ...interface{}) *queryBuilder {
	qb.havings = append(qb.havings, whereClause{sql: sql, args: args})
	return qb
}

// Join adds a JOIN clause to the query
func (qb *queryBuilder) Join(sql string) *queryBuilder {
	qb.joins = append(qb.joins, sql)
	return qb
}

// Select sets the SELECT columns for the query
func (qb *queryBuilder) Select(cols ...string) *queryBuilder {
	qb.selects = append(qb.selects, cols...)
	return qb
}

// GroupBy adds a GROUP BY clause to the query
func (qb *queryBuilder) GroupBy(cols ...string) *queryBuilder {
	qb.groupBys = append(qb.groupBys, cols...)
	return qb
}

// OrderBy adds an ORDER BY clause to the query
func (qb *queryBuilder) OrderBy(order string) *queryBuilder {
	qb.orderBys = append(qb.orderBys, order)
	return qb
}

// Limit sets the LIMIT for the query
func (qb *queryBuilder) Limit(n int) *queryBuilder {
	qb.limit = &n
	return qb
}

// Offset sets the OFFSET for the query
func (qb *queryBuilder) Offset(n int) *queryBuilder {
	qb.offset = n
	return qb
}

// Set stores a key-value pair in the query builder's instance state
func (qb *queryBuilder) Set(key string, val any) {
	if qb.instState == nil {
		qb.instState = make(map[string]any)
	}
	qb.instState[key] = val
}

// Get retrieves a value from the query builder's instance state
func (qb *queryBuilder) Get(key string) (any, bool) {
	if qb.instState == nil {
		return nil, false
	}
	val, ok := qb.instState[key]
	return val, ok
}

// WithLogger sets the logger for the query builder
func (qb *queryBuilder) WithLogger(logger *slog.Logger) *queryBuilder {
	if logger != nil {
		qb.logger = logger
	}
	return qb
}

// Clone creates a shallow copy of the query builder
func (qb *queryBuilder) Clone() *queryBuilder {
	clone := &queryBuilder{
		db:       qb.db,
		dialect:  qb.dialect,
		table:    qb.table,
		wheres:   append([]whereClause{}, qb.wheres...),
		havings:  append([]whereClause{}, qb.havings...),
		joins:    append([]string{}, qb.joins...),
		selects:  append([]string{}, qb.selects...),
		groupBys: append([]string{}, qb.groupBys...),
		orderBys: append([]string{}, qb.orderBys...),
		offset:   qb.offset,
		logger:   qb.logger,
	}
	if qb.limit != nil {
		limitCopy := *qb.limit
		clone.limit = &limitCopy
	}
	// Deep copy instance state
	clone.instState = make(map[string]any)
	for k, v := range qb.instState {
		clone.instState[k] = v
	}
	return clone
}

// ToSQL builds the final SELECT SQL statement with parameterized arguments
func (qb *queryBuilder) ToSQL() (string, []interface{}) {
	var sql strings.Builder
	var args []interface{}

	// SELECT clause
	sql.WriteString("SELECT ")
	if len(qb.selects) > 0 {
		sql.WriteString(strings.Join(qb.selects, ", "))
	} else {
		sql.WriteString("*")
	}

	// FROM clause
	if qb.table != "" {
		sql.WriteString(" FROM ")
		sql.WriteString(quoteTableName(qb.dialect, qb.table))
	}

	// JOIN clauses
	for _, join := range qb.joins {
		sql.WriteString(" ")
		sql.WriteString(join)
	}

	// WHERE clause
	if len(qb.wheres) > 0 {
		sql.WriteString(" WHERE ")
		whereClauses := make([]string, 0, len(qb.wheres))
		for _, w := range qb.wheres {
			whereClauses = append(whereClauses, w.sql)
			args = append(args, w.args...)
		}
		sql.WriteString(strings.Join(whereClauses, " AND "))
	}

	// GROUP BY clause
	if len(qb.groupBys) > 0 {
		sql.WriteString(" GROUP BY ")
		sql.WriteString(strings.Join(qb.groupBys, ", "))
	}

	// HAVING clause
	if len(qb.havings) > 0 {
		sql.WriteString(" HAVING ")
		havingClauses := make([]string, 0, len(qb.havings))
		for _, h := range qb.havings {
			havingClauses = append(havingClauses, h.sql)
			args = append(args, h.args...)
		}
		sql.WriteString(strings.Join(havingClauses, " AND "))
	}

	// ORDER BY clause
	if len(qb.orderBys) > 0 {
		sql.WriteString(" ORDER BY ")
		sql.WriteString(strings.Join(qb.orderBys, ", "))
	}

	// LIMIT and OFFSET
	if qb.limit != nil {
		sql.WriteString(fmt.Sprintf(" LIMIT %d", *qb.limit))
	} else if qb.offset > 0 && qb.dialect == "mysql" {
		// MySQL requires LIMIT when OFFSET is used
		sql.WriteString(" LIMIT 2147483647")
	}

	if qb.offset > 0 {
		sql.WriteString(fmt.Sprintf(" OFFSET %d", qb.offset))
	}

	query := sql.String()

	// Convert placeholders for PostgreSQL ($1, $2, ...)
	if qb.dialect == "postgres" || qb.dialect == "postgresql" {
		query = convertToPostgresPlaceholders(query)
	}

	return query, args
}

// ToCountSQL builds a COUNT(*) query based on the current query builder state
func (qb *queryBuilder) ToCountSQL() (string, []interface{}) {
	var sql strings.Builder
	var args []interface{}

	// For queries with GROUP BY, we need to count the grouped results
	if len(qb.groupBys) > 0 {
		// Use a subquery to count the grouped results
		innerSQL, innerArgs := qb.toCountInnerSQL()
		sql.WriteString("SELECT COUNT(*) FROM (")
		sql.WriteString(innerSQL)
		sql.WriteString(") AS count_subquery")
		args = innerArgs
	} else {
		// Simple COUNT without subquery
		sql.WriteString("SELECT COUNT(*)")

		// FROM clause
		if qb.table != "" {
			sql.WriteString(" FROM ")
			sql.WriteString(quoteTableName(qb.dialect, qb.table))
		}

		// JOIN clauses
		for _, join := range qb.joins {
			sql.WriteString(" ")
			sql.WriteString(join)
		}

		// WHERE clause
		if len(qb.wheres) > 0 {
			sql.WriteString(" WHERE ")
			whereClauses := make([]string, 0, len(qb.wheres))
			for _, w := range qb.wheres {
				whereClauses = append(whereClauses, w.sql)
				args = append(args, w.args...)
			}
			sql.WriteString(strings.Join(whereClauses, " AND "))
		}
	}

	query := sql.String()

	// Convert placeholders for PostgreSQL
	if qb.dialect == "postgres" || qb.dialect == "postgresql" {
		query = convertToPostgresPlaceholders(query)
	}

	return query, args
}

// toCountInnerSQL builds the inner SELECT for COUNT queries with GROUP BY
func (qb *queryBuilder) toCountInnerSQL() (string, []interface{}) {
	var sql strings.Builder
	var args []interface{}

	// SELECT clause - just select the grouped columns
	sql.WriteString("SELECT ")
	if len(qb.groupBys) > 0 {
		sql.WriteString(strings.Join(qb.groupBys, ", "))
	} else {
		sql.WriteString("1")
	}

	// FROM clause
	if qb.table != "" {
		sql.WriteString(" FROM ")
		sql.WriteString(quoteTableName(qb.dialect, qb.table))
	}

	// JOIN clauses
	for _, join := range qb.joins {
		sql.WriteString(" ")
		sql.WriteString(join)
	}

	// WHERE clause
	if len(qb.wheres) > 0 {
		sql.WriteString(" WHERE ")
		whereClauses := make([]string, 0, len(qb.wheres))
		for _, w := range qb.wheres {
			whereClauses = append(whereClauses, w.sql)
			args = append(args, w.args...)
		}
		sql.WriteString(strings.Join(whereClauses, " AND "))
	}

	// GROUP BY clause
	if len(qb.groupBys) > 0 {
		sql.WriteString(" GROUP BY ")
		sql.WriteString(strings.Join(qb.groupBys, ", "))
	}

	// HAVING clause
	if len(qb.havings) > 0 {
		sql.WriteString(" HAVING ")
		havingClauses := make([]string, 0, len(qb.havings))
		for _, h := range qb.havings {
			havingClauses = append(havingClauses, h.sql)
			args = append(args, h.args...)
		}
		sql.WriteString(strings.Join(havingClauses, " AND "))
	}

	return sql.String(), args
}

// QueryContext executes the query and returns the result rows
func (qb *queryBuilder) QueryContext(ctx context.Context) (*sql.Rows, error) {
	query, args := qb.ToSQL()

	if qb.logger != nil {
		qb.logger.Debug("Executing query", "sql", query, "args", args)
	}

	return qb.db.QueryContext(ctx, query, args...)
}

// QueryRowContext executes the query and returns a single row
func (qb *queryBuilder) QueryRowContext(ctx context.Context) *sql.Row {
	query, args := qb.ToSQL()

	if qb.logger != nil {
		qb.logger.Debug("Executing query row", "sql", query, "args", args)
	}

	return qb.db.QueryRowContext(ctx, query, args...)
}

// CountContext executes the count query and returns the count
func (qb *queryBuilder) CountContext(ctx context.Context) (int64, error) {
	query, args := qb.ToCountSQL()

	if qb.logger != nil {
		qb.logger.Debug("Executing count query", "sql", query, "args", args)
	}

	var count int64
	err := qb.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// convertToPostgresPlaceholders converts ? placeholders to $1, $2, ... for PostgreSQL
func convertToPostgresPlaceholders(query string) string {
	var result strings.Builder
	placeholderNum := 1

	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			result.WriteString(fmt.Sprintf("$%d", placeholderNum))
			placeholderNum++
		} else {
			result.WriteByte(query[i])
		}
	}

	return result.String()
}
