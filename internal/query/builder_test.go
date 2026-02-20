package query

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3" // SQLite driver for testing
)

func setupQueryBuilderTestDB(t *testing.T) (*sql.DB, string) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create a test table
	_, err = db.Exec(`
		CREATE TABLE products (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			price REAL,
			category TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert test data
	_, err = db.Exec(`
		INSERT INTO products (id, name, price, category) VALUES
		(1, 'Product 1', 10.5, 'A'),
		(2, 'Product 2', 20.0, 'B'),
		(3, 'Product 3', 15.5, 'A'),
		(4, 'Product 4', 30.0, 'C')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	return db, "sqlite"
}

func TestQueryBuilder_BasicSelect(t *testing.T) {
	db, dialect := setupQueryBuilderTestDB(t)
	defer db.Close()

	qb := newQueryBuilder(db, dialect)
	qb = qb.WithTable("products")

	sql, args := qb.ToSQL()
	expectedSQL := "SELECT * FROM \"products\""
	if sql != expectedSQL {
		t.Errorf("Expected SQL %q, got %q", expectedSQL, sql)
	}
	if len(args) != 0 {
		t.Errorf("Expected no args, got %v", args)
	}
}

func TestQueryBuilder_SelectWithColumns(t *testing.T) {
	db, dialect := setupQueryBuilderTestDB(t)
	defer db.Close()

	qb := newQueryBuilder(db, dialect)
	qb = qb.WithTable("products").Select("id", "name", "price")

	sql, args := qb.ToSQL()
	expectedSQL := "SELECT id, name, price FROM \"products\""
	if sql != expectedSQL {
		t.Errorf("Expected SQL %q, got %q", expectedSQL, sql)
	}
	if len(args) != 0 {
		t.Errorf("Expected no args, got %v", args)
	}
}

func TestQueryBuilder_Where(t *testing.T) {
	db, dialect := setupQueryBuilderTestDB(t)
	defer db.Close()

	qb := newQueryBuilder(db, dialect)
	qb = qb.WithTable("products").
		Where("price > ?", 15.0).
		Where("category = ?", "A")

	sql, args := qb.ToSQL()
	expectedSQL := "SELECT * FROM \"products\" WHERE price > ? AND category = ?"
	if sql != expectedSQL {
		t.Errorf("Expected SQL %q, got %q", expectedSQL, sql)
	}
	if len(args) != 2 {
		t.Fatalf("Expected 2 args, got %d", len(args))
	}
	if args[0] != 15.0 {
		t.Errorf("Expected first arg to be 15.0, got %v", args[0])
	}
	if args[1] != "A" {
		t.Errorf("Expected second arg to be 'A', got %v", args[1])
	}
}

func TestQueryBuilder_OrderBy(t *testing.T) {
	db, dialect := setupQueryBuilderTestDB(t)
	defer db.Close()

	qb := newQueryBuilder(db, dialect)
	qb = qb.WithTable("products").
		OrderBy("price DESC").
		OrderBy("name ASC")

	sql, _ := qb.ToSQL()
	expectedSQL := "SELECT * FROM \"products\" ORDER BY price DESC, name ASC"
	if sql != expectedSQL {
		t.Errorf("Expected SQL %q, got %q", expectedSQL, sql)
	}
}

func TestQueryBuilder_LimitOffset(t *testing.T) {
	db, dialect := setupQueryBuilderTestDB(t)
	defer db.Close()

	qb := newQueryBuilder(db, dialect)
	qb = qb.WithTable("products").
		Limit(10).
		Offset(5)

	sql, _ := qb.ToSQL()
	expectedSQL := "SELECT * FROM \"products\" LIMIT 10 OFFSET 5"
	if sql != expectedSQL {
		t.Errorf("Expected SQL %q, got %q", expectedSQL, sql)
	}
}

func TestQueryBuilder_MySQLOffsetRequiresLimit(t *testing.T) {
	db, _ := setupQueryBuilderTestDB(t)
	defer db.Close()

	qb := newQueryBuilder(db, "mysql")
	qb = qb.WithTable("products").Offset(5)

	sql, _ := qb.ToSQL()
	// MySQL should add a large LIMIT when OFFSET is used without explicit LIMIT
	expectedSQL := "SELECT * FROM `products` LIMIT 2147483647 OFFSET 5"
	if sql != expectedSQL {
		t.Errorf("Expected SQL %q, got %q", expectedSQL, sql)
	}
}

func TestQueryBuilder_GroupByHaving(t *testing.T) {
	db, dialect := setupQueryBuilderTestDB(t)
	defer db.Close()

	qb := newQueryBuilder(db, dialect)
	qb = qb.WithTable("products").
		Select("category", "COUNT(*) as count").
		GroupBy("category").
		Having("COUNT(*) > ?", 1)

	sql, args := qb.ToSQL()
	expectedSQL := "SELECT category, COUNT(*) as count FROM \"products\" GROUP BY category HAVING COUNT(*) > ?"
	if sql != expectedSQL {
		t.Errorf("Expected SQL %q, got %q", expectedSQL, sql)
	}
	if len(args) != 1 || args[0] != 1 {
		t.Errorf("Expected args [1], got %v", args)
	}
}

func TestQueryBuilder_Join(t *testing.T) {
	db, dialect := setupQueryBuilderTestDB(t)
	defer db.Close()

	qb := newQueryBuilder(db, dialect)
	qb = qb.WithTable("products").
		Join("LEFT JOIN categories ON products.category_id = categories.id").
		Where("categories.active = ?", true)

	sql, args := qb.ToSQL()
	expectedSQL := "SELECT * FROM \"products\" LEFT JOIN categories ON products.category_id = categories.id WHERE categories.active = ?"
	if sql != expectedSQL {
		t.Errorf("Expected SQL %q, got %q", expectedSQL, sql)
	}
	if len(args) != 1 {
		t.Errorf("Expected 1 arg, got %d", len(args))
	}
}

func TestQueryBuilder_ToCountSQL(t *testing.T) {
	db, dialect := setupQueryBuilderTestDB(t)
	defer db.Close()

	qb := newQueryBuilder(db, dialect)
	qb = qb.WithTable("products").
		Where("price > ?", 15.0)

	sql, args := qb.ToCountSQL()
	expectedSQL := "SELECT COUNT(*) FROM \"products\" WHERE price > ?"
	if sql != expectedSQL {
		t.Errorf("Expected SQL %q, got %q", expectedSQL, sql)
	}
	if len(args) != 1 {
		t.Errorf("Expected 1 arg, got %d", len(args))
	}
}

func TestQueryBuilder_ToCountSQLWithGroupBy(t *testing.T) {
	db, dialect := setupQueryBuilderTestDB(t)
	defer db.Close()

	qb := newQueryBuilder(db, dialect)
	qb = qb.WithTable("products").
		Select("category", "COUNT(*) as count").
		GroupBy("category").
		Having("COUNT(*) > ?", 1)

	sql, args := qb.ToCountSQL()
	// With GROUP BY, should use a subquery
	expectedSQL := "SELECT COUNT(*) FROM (SELECT category FROM \"products\" GROUP BY category HAVING COUNT(*) > ?) AS count_subquery"
	if sql != expectedSQL {
		t.Errorf("Expected SQL %q, got %q", expectedSQL, sql)
	}
	if len(args) != 1 {
		t.Errorf("Expected 1 arg, got %d", len(args))
	}
}

func TestQueryBuilder_QueryContext(t *testing.T) {
	db, dialect := setupQueryBuilderTestDB(t)
	defer db.Close()

	qb := newQueryBuilder(db, dialect)
	qb = qb.WithTable("products").
		Where("price > ?", 15.0).
		OrderBy("price ASC")

	ctx := context.Background()
	rows, err := qb.QueryContext(ctx)
	if err != nil {
		t.Fatalf("QueryContext failed: %v", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var id int
		var name string
		var price float64
		var category string
		if err := rows.Scan(&id, &name, &price, &category); err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		count++
	}

	// Should return 3 products (price > 15.0): Product 2 (20.0), Product 3 (15.5), and Product 4 (30.0)
	if count != 3 {
		t.Errorf("Expected 3 results, got %d", count)
	}
}

func TestQueryBuilder_CountContext(t *testing.T) {
	db, dialect := setupQueryBuilderTestDB(t)
	defer db.Close()

	qb := newQueryBuilder(db, dialect)
	qb = qb.WithTable("products").
		Where("price > ?", 15.0)

	ctx := context.Background()
	count, err := qb.CountContext(ctx)
	if err != nil {
		t.Fatalf("CountContext failed: %v", err)
	}

	// Should count 3 products with price > 15.0
	if count != 3 {
		t.Errorf("Expected count 3, got %d", count)
	}
}

func TestQueryBuilder_Clone(t *testing.T) {
	db, dialect := setupQueryBuilderTestDB(t)
	defer db.Close()

	original := newQueryBuilder(db, dialect)
	original = original.WithTable("products").
		Where("price > ?", 10.0).
		OrderBy("price DESC")

	clone := original.Clone()
	clone = clone.Where("category = ?", "A")

	// Original should not be affected by changes to clone
	originalSQL, originalArgs := original.ToSQL()
	cloneSQL, cloneArgs := clone.ToSQL()

	if originalSQL == cloneSQL {
		t.Errorf("Clone should have different SQL after modification")
	}

	if len(originalArgs) != 1 {
		t.Errorf("Original should have 1 arg, got %d", len(originalArgs))
	}

	if len(cloneArgs) != 2 {
		t.Errorf("Clone should have 2 args, got %d", len(cloneArgs))
	}
}

func TestQueryBuilder_PostgreSQLPlaceholders(t *testing.T) {
	db, _ := setupQueryBuilderTestDB(t)
	defer db.Close()

	qb := newQueryBuilder(db, "postgres")
	qb = qb.WithTable("products").
		Where("price > ?", 15.0).
		Where("category = ?", "A").
		Having("COUNT(*) > ?", 1)

	sql, _ := qb.ToSQL()
	// PostgreSQL should use $1, $2, $3 placeholders
	expectedSQL := `SELECT * FROM "products" WHERE price > $1 AND category = $2 HAVING COUNT(*) > $3`
	if sql != expectedSQL {
		t.Errorf("Expected SQL %q, got %q", expectedSQL, sql)
	}
}

func TestQueryBuilder_SetGet(t *testing.T) {
	db, dialect := setupQueryBuilderTestDB(t)
	defer db.Close()

	qb := newQueryBuilder(db, dialect)

	// Set a value
	qb.Set("test_key", "test_value")

	// Get the value
	val, ok := qb.Get("test_key")
	if !ok {
		t.Error("Expected key to exist")
	}
	if val != "test_value" {
		t.Errorf("Expected value 'test_value', got %v", val)
	}

	// Get non-existent key
	_, ok = qb.Get("nonexistent")
	if ok {
		t.Error("Expected key to not exist")
	}
}
