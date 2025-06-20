package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	querydsl "github.com/asaidimu/querydsl/pkg/core"
	"maps"
)

// --- Test Setup Functions ---

func setupTestDB(t *testing.T) *sql.DB {
	// Use a shared in-memory database for concurrency tests and consistent state across calls.
	// ":memory:" creates a new db per connection. "file::memory:?cache=shared" shares it across connections.
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared") // IMPORTANT CHANGE HERE
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Set a reasonable max connections for better concurrency behavior, though SQLite's single-writer model
	// still serializes writes. Reads can be concurrent.
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Use a mutex to prevent race conditions during table creation across concurrent tests
	// if multiple `go test` instances or test functions call `setupTestDB`.
	// For simple `go test ./...`, this might not be strictly needed if tests run sequentially.
	// However, it's good practice for shared resources.
	// We'll manage this by checking if tables exist inside the function.
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users';").Scan(&count)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to check if table exists: %v", err)
	}

	if count == 0 { // Only create tables and insert data if they don't exist
		// Create tables and insert data
		createTableSQL := `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			first_name TEXT,
			last_name TEXT,
			age INTEGER,
			access_level TEXT,
			is_active BOOLEAN,
			balance REAL,
			created_at TEXT
		);
		CREATE TABLE products (
			product_id INTEGER PRIMARY KEY,
			name TEXT,
			price REAL,
			category TEXT
		);
		`
		_, err = db.Exec(createTableSQL)
		if err != nil {
			db.Close()
			t.Fatalf("Failed to create table: %v", err)
		}

		insertUsersSQL := `
		INSERT INTO users (id, first_name, last_name, age, access_level, is_active, balance, created_at) VALUES
		(1, 'Alice', 'Smith', 25, 'standard', TRUE, 100.50, '2023-01-15'),
		(2, 'Bob', 'Johnson', 16, 'standard', TRUE, 50.25, '2023-03-20'),
		(3, 'Charlie', 'Brown', 30, 'premium', TRUE, 1200.75, '2023-02-10'),
		(4, 'Diana', 'Prince', 17, 'premium', FALSE, 250.00, '2023-04-01'),
		(5, 'Eve', 'Adams', 42, 'standard', TRUE, 75.00, '2023-05-05');
		`
		_, err = db.Exec(insertUsersSQL)
		if err != nil {
			db.Close()
			t.Fatalf("Failed to insert user data: %v", err)
		}

		insertProductsSQL := `
		INSERT INTO products (product_id, name, price, category) VALUES
		(101, 'Laptop', 1200.00, 'Electronics'),
		(102, 'Mouse', 25.50, 'Electronics'),
		(103, 'Keyboard', 75.00, 'Electronics'),
		(104, 'Desk Chair', 150.00, 'Furniture');
		`
		_, err = db.Exec(insertProductsSQL)
		if err != nil {
			db.Close()
			t.Fatalf("Failed to insert product data: %v", err)
		}
	}

	return db
}

// --- Custom Go Functions for Testing ---

func testGoComputeFullName(row querydsl.Row) (any, error) {
	firstName, ok1 := row["first_name"].(string)
	lastName, ok2 := row["last_name"].(string)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("missing first_name or last_name for full_name computation in row: %+v", row)
	}
	return fmt.Sprintf("%s %s", firstName, lastName), nil
}

func testGoFilterIsAdult(row querydsl.Row) (bool, error) {
	age, ok := row["age"].(int64)
	if !ok {
		return false, fmt.Errorf("age not found or not int64 for is_adult filter in row: %+v", row)
	}
	return age >= 18, nil
}

func testGoFilterHasAccessLevel(row querydsl.Row) (bool, error) {
	accessLevel, ok := row["access_level"].(string)
	if !ok {
		return false, fmt.Errorf("access_level not found or not string for has_access_level filter in row: %+v", row)
	}
	// For demonstration, let's say "premium" access is the "true" condition
	return accessLevel == "premium", nil
}

// Helper to sort results for consistent comparison
func sortResultsByID(results []map[string]any) {
	sort.Slice(results, func(i, j int) bool {
		id1, ok1 := results[i]["id"].(int64)
		id2, ok2 := results[j]["id"].(int64)
		if ok1 && ok2 {
			return id1 < id2
		}
		// Fallback for non-numeric or missing IDs
		return fmt.Sprintf("%v", results[i]["id"]) < fmt.Sprintf("%v", results[j]["id"])
	})
}

// Helper to compare actual vs expected results
func assertResultsEqual(t *testing.T, actual, expected []map[string]any) {
	if len(actual) != len(expected) {
		t.Fatalf("Result count mismatch: got %d, want %d.\nActual: %+v\nExpected: %+v", len(actual), len(expected), actual, expected)
	}

	// Sort both actual and expected to ensure order-independent comparison
	sortResultsByID(actual)
	sortResultsByID(expected)

	for i := range actual {
		// Reflect.DeepEqual for maps might have issues with nil vs missing keys, or specific types
		// A more robust comparison might iterate keys and check values.
		// Ensure types are consistent (e.g. int64 from DB vs int in expected)
		if !reflect.DeepEqual(actual[i], expected[i]) {
			t.Errorf("Row %d mismatch:\n  Got: %+v\n  Want: %+v", i, actual[i], expected[i])
		}
	}
}

// --- Tests ---

func TestExecutorBasicQuery(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	exec := NewSqliteExecutor(db, nil)

	dsl := &querydsl.QueryDSL{
		Filters: &querydsl.QueryFilter{
			Condition: &querydsl.FilterCondition{
				Field:    "age",
				Operator: querydsl.ComparisonOperatorGt,
				Value:    20,
			},
		},
		Projection: &querydsl.ProjectionConfiguration{
			Include: []querydsl.ProjectionField{
				{Name: "first_name"},
				{Name: "age"},
			},
		},
		Sort: []querydsl.SortConfiguration{
			{Field: "age", Direction: querydsl.SortDirectionAsc},
		},
	}

	ctx := context.Background()
	result, err := exec.Execute(ctx, "users", dsl)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	expected := []map[string]any{
		{"first_name": "Alice", "age": int64(25)},
		{"first_name": "Charlie", "age": int64(30)},
		{"first_name": "Eve", "age": int64(42)},
	}

	actualRows, ok := result.Data.([]querydsl.Row)
	if !ok {
		t.Fatalf("Expected result.Data to be []core.Row, got %T", result.Data)
	}

	actualMaps := make([]map[string]any, len(actualRows))
	for i, row := range actualRows {
		rowMap := make(map[string]any)
		maps.Copy(rowMap, row)
		actualMaps[i] = rowMap
	}

	assertResultsEqual(t, actualMaps, expected)
}

func TestExecutorGoComputedFunction(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	exec := NewSqliteExecutor(db, nil)

	exec.RegisterComputeFunction("full_name_calc", testGoComputeFullName)

	dsl := &querydsl.QueryDSL{
		Projection: &querydsl.ProjectionConfiguration{
			Include: []querydsl.ProjectionField{
				{Name: "first_name"},
				{Name: "last_name"},
			},
			Computed: []querydsl.ProjectionComputedItem{
				{
					ComputedFieldExpression: &querydsl.ComputedFieldExpression{
						Type:       "computed",
						Expression: querydsl.FunctionCall{Function: "full_name_calc"},
						Alias:      "full_name",
					},
				},
			},
		},
		Sort: []querydsl.SortConfiguration{
			{Field: "id", Direction: querydsl.SortDirectionAsc},
		},
	}

	ctx := context.Background()
	result, err := exec.Execute(ctx, "users", dsl)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	expected := []map[string]any{
		{"first_name": "Alice", "last_name": "Smith", "full_name": "Alice Smith"},
		{"first_name": "Bob", "last_name": "Johnson", "full_name": "Bob Johnson"},
		{"first_name": "Charlie", "last_name": "Brown", "full_name": "Charlie Brown"},
		{"first_name": "Diana", "last_name": "Prince", "full_name": "Diana Prince"},
		{"first_name": "Eve", "last_name": "Adams", "full_name": "Eve Adams"},
	}

	actualRows, ok := result.Data.([]querydsl.Row)
	if !ok {
		t.Fatalf("Expected result.Data to be []core.Row, got %T", result.Data)
	}

	actualMaps := make([]map[string]any, len(actualRows))
	for i, row := range actualRows {
		rowMap := make(map[string]any)
		maps.Copy(rowMap, row)
		actualMaps[i] = rowMap
	}

	assertResultsEqual(t, actualMaps, expected)

}

func TestExecutorGoFilterFunction(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	exec := NewSqliteExecutor(db, nil)

	exec.RegisterFilterFunction("is_adult_check", testGoFilterIsAdult)

	dsl := &querydsl.QueryDSL{
		Filters: &querydsl.QueryFilter{
			Condition: &querydsl.FilterCondition{
				Field:    "age", // This field will be read by the Go filter function
				Operator: "is_adult_check",
				Value:    nil, // Value is not used by this Go filter
			},
		},
		Projection: &querydsl.ProjectionConfiguration{
			Include: []querydsl.ProjectionField{
				{Name: "first_name"},
				{Name: "age"}, // IMPORTANT: Must include 'age' for the Go filter to work
			},
		},
		Sort: []querydsl.SortConfiguration{
			{Field: "id", Direction: querydsl.SortDirectionAsc},
		},
	}

	ctx := context.Background()
	result, err := exec.Execute(ctx, "users", dsl)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	expected := []map[string]any{
		{"first_name": "Alice", "age": int64(25)},
		{"first_name": "Charlie", "age": int64(30)},
		{"first_name": "Eve", "age": int64(42)},
	}

	actualRows, ok := result.Data.([]querydsl.Row)
	if !ok {
		t.Fatalf("Expected result.Data to be []core.Row, got %T", result.Data)
	}

	actualMaps := make([]map[string]any, len(actualRows))
	for i, row := range actualRows {
		rowMap := make(map[string]any)
		maps.Copy(rowMap, row)
		actualMaps[i] = rowMap
	}

	assertResultsEqual(t, actualMaps, expected)
}

func TestExecutorMixedFilters(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	exec := NewSqliteExecutor(db, nil)

	exec.RegisterFilterFunction("is_adult_check", testGoFilterIsAdult)     // Go filter
	exec.RegisterFilterFunction("has_premium", testGoFilterHasAccessLevel) // Go filter

	dsl := &querydsl.QueryDSL{
		Filters: &querydsl.QueryFilter{
			Group: &querydsl.FilterGroup{
				Operator: querydsl.LogicalOperatorAnd,
				Conditions: []querydsl.QueryFilter{
					{ // DB-native filter
						Condition: &querydsl.FilterCondition{
							Field:    "age",
							Operator: querydsl.ComparisonOperatorLt,
							Value:    30,
						},
					},
					{ // Go-based filter
						Condition: &querydsl.FilterCondition{
							Field:    "access_level", // Go function reads this
							Operator: "has_premium",
							Value:    nil,
						},
					},
					{ // Another DB-native filter
						Condition: &querydsl.FilterCondition{
							Field:    "is_active",
							Operator: querydsl.ComparisonOperatorEq,
							Value:    true,
						},
					},
				},
			},
		},
		Projection: &querydsl.ProjectionConfiguration{
			Include: []querydsl.ProjectionField{
				{Name: "first_name"},
				{Name: "age"},          // Required by is_adult_check (if used)
				{Name: "access_level"}, // Required by has_premium
				{Name: "is_active"},
			},
		},
		Sort: []querydsl.SortConfiguration{
			{Field: "id", Direction: querydsl.SortDirectionAsc},
		},
	}

	ctx := context.Background()
	result, err := exec.Execute(ctx, "users", dsl)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Expected: No user is <30 AND premium AND active
	// Alice (age 25, standard, active) -> Fails "has_premium"
	// Bob (age 16, standard, active) -> Fails "has_premium"
	// Charlie (age 30, premium, active) -> Fails "age < 30"
	// Diana (age 17, premium, inactive) -> Fails "is_active = true"
	// Eve (age 42, standard, active) -> Fails "age < 30" and "has_premium"
	expected := []map[string]any{}

	actualRows, ok := result.Data.([]querydsl.Row)
	if !ok {
		t.Fatalf("Expected result.Data to be []core.Row, got %T", result.Data)
	}

	actualMaps := make([]map[string]any, len(actualRows))
	for i, row := range actualRows {
		rowMap := make(map[string]any)
		maps.Copy(rowMap, row)
		actualMaps[i] = rowMap
	}

	assertResultsEqual(t, actualMaps, expected)

	// Test a scenario where results *are* expected
	dsl2 := &querydsl.QueryDSL{
		Filters: &querydsl.QueryFilter{
			Group: &querydsl.FilterGroup{
				Operator: querydsl.LogicalOperatorAnd,
				Conditions: []querydsl.QueryFilter{
					{ // DB-native filter: active users
						Condition: &querydsl.FilterCondition{
							Field:    "is_active",
							Operator: querydsl.ComparisonOperatorEq,
							Value:    true,
						},
					},
					{ // Go-based filter: age >= 18
						Condition: &querydsl.FilterCondition{
							Field:    "age",
							Operator: "is_adult_check",
							Value:    nil,
						},
					},
				},
			},
		},
		Projection: &querydsl.ProjectionConfiguration{
			Include: []querydsl.ProjectionField{
				{Name: "first_name"},
				{Name: "age"}, // Required for is_adult_check
				{Name: "is_active"},
			},
		},
		Sort: []querydsl.SortConfiguration{
			{Field: "id", Direction: querydsl.SortDirectionAsc},
		},
	}
	results2, err := exec.Execute(ctx, "users", dsl2)
	if err != nil {
		t.Fatalf("Execute 2 failed: %v", err)
	}
	expected2 := []map[string]any{
		{"first_name": "Alice", "age": int64(25), "is_active": true},
		{"first_name": "Charlie", "age": int64(30), "is_active": true},
		{"first_name": "Eve", "age": int64(42), "is_active": true},
	}

	actualRows2, ok := results2.Data.([]querydsl.Row)
	if !ok {
		t.Fatalf("Expected result.Data to be []core.Row, got %T", result.Data)
	}

	actualMaps2 := make([]map[string]any, len(actualRows2))
	for i, row := range actualRows2 {
		rowMap := make(map[string]any)
		maps.Copy(rowMap, row)
		actualMaps2[i] = rowMap
	}

	assertResultsEqual(t, actualMaps2, expected2)
}

func TestExecutorLogicalOperators(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	exec := NewSqliteExecutor(db, nil)
	exec.RegisterFilterFunction("is_adult_check", testGoFilterIsAdult)     // Go filter
	exec.RegisterFilterFunction("has_premium", testGoFilterHasAccessLevel) // Go filter

	tests := []struct {
		name     string
		dsl      *querydsl.QueryDSL
		expected []map[string]any
	}{
		{
			name: "AND operator (DB + Go)",
			dsl: &querydsl.QueryDSL{
				Filters: &querydsl.QueryFilter{
					Group: &querydsl.FilterGroup{
						Operator: querydsl.LogicalOperatorAnd,
						Conditions: []querydsl.QueryFilter{
							{Condition: &querydsl.FilterCondition{Field: "access_level", Operator: querydsl.ComparisonOperatorEq, Value: "standard"}},
							{Condition: &querydsl.FilterCondition{Field: "age", Operator: "is_adult_check", Value: nil}},
						},
					},
				},
				Projection: &querydsl.ProjectionConfiguration{
					Include: []querydsl.ProjectionField{
						{Name: "first_name"},
						{Name: "age"},          // REQUIRED for Go filter
						{Name: "access_level"}, // REQUIRED for DB filter
					},
				},
				Sort: []querydsl.SortConfiguration{{Field: "id", Direction: querydsl.SortDirectionAsc}},
			},
			expected: []map[string]any{
				{"first_name": "Alice", "age": int64(25), "access_level": "standard"},
				{"first_name": "Eve", "age": int64(42), "access_level": "standard"},
			},
		},
		{
			name: "OR operator (DB + Go)",
			dsl: &querydsl.QueryDSL{
				Filters: &querydsl.QueryFilter{
					Group: &querydsl.FilterGroup{
						Operator: querydsl.LogicalOperatorOr,
						Conditions: []querydsl.QueryFilter{
							{Condition: &querydsl.FilterCondition{Field: "age", Operator: querydsl.ComparisonOperatorLt, Value: 18}},
							{Condition: &querydsl.FilterCondition{Field: "access_level", Operator: querydsl.ComparisonOperatorEq, Value: "premium"}},
						},
					},
				},
				Projection: &querydsl.ProjectionConfiguration{
					Include: []querydsl.ProjectionField{
						{Name: "first_name"},
						{Name: "age"},
						{Name: "access_level"},
					},
				},
				Sort: []querydsl.SortConfiguration{{Field: "id", Direction: querydsl.SortDirectionAsc}},
			},
			expected: []map[string]any{
				{"first_name": "Bob", "age": int64(16), "access_level": "standard"},    // age 16 (<18)
				{"first_name": "Charlie", "age": int64(30), "access_level": "premium"}, // premium
				{"first_name": "Diana", "age": int64(17), "access_level": "premium"},   // age 17 (<18) OR premium
			},
		},
		{
			name: "NOT operator (Go filter)",
			dsl: &querydsl.QueryDSL{
				Filters: &querydsl.QueryFilter{
					Group: &querydsl.FilterGroup{
						Operator: querydsl.LogicalOperatorNot,
						Conditions: []querydsl.QueryFilter{
							{Condition: &querydsl.FilterCondition{Field: "age", Operator: "is_adult_check", Value: nil}},
						},
					},
				},
				Projection: &querydsl.ProjectionConfiguration{
					Include: []querydsl.ProjectionField{
						{Name: "first_name"},
						{Name: "age"}, // REQUIRED for Go filter
					},
				},
				Sort: []querydsl.SortConfiguration{{Field: "id", Direction: querydsl.SortDirectionAsc}},
			},
			expected: []map[string]any{
				{"first_name": "Bob", "age": int64(16)},   // 16 is not adult
				{"first_name": "Diana", "age": int64(17)}, // 17 is not adult
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := exec.Execute(ctx, "users", tt.dsl)
			if err != nil {
				t.Fatalf("Execute failed for %s: %v", tt.name, err)
			}
			actualRows, ok := result.Data.([]querydsl.Row)
			if !ok {
				t.Fatalf("Expected result.Data to be []core.Row, got %T", result.Data)
			}

			actualMaps := make([]map[string]any, len(actualRows))
			for i, row := range actualRows {
				rowMap := make(map[string]any)
				maps.Copy(rowMap, row)
				actualMaps[i] = rowMap
			}

			assertResultsEqual(t, actualMaps, tt.expected)
		})
	}
}

func TestExecutorProjectionIncludeExclude(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	exec := NewSqliteExecutor(db, nil)

	tests := []struct {
		name     string
		dsl      *querydsl.QueryDSL
		expected []map[string]any
	}{
		{
			name: "Include specific fields",
			dsl: &querydsl.QueryDSL{
				Projection: &querydsl.ProjectionConfiguration{
					Include: []querydsl.ProjectionField{
						{Name: "first_name"},
						{Name: "age"},
					},
				},
				Sort: []querydsl.SortConfiguration{{Field: "id", Direction: querydsl.SortDirectionAsc}},
			},
			expected: []map[string]any{
				{"first_name": "Alice", "age": int64(25)},
				{"first_name": "Bob", "age": int64(16)},
				{"first_name": "Charlie", "age": int64(30)},
				{"first_name": "Diana", "age": int64(17)},
				{"first_name": "Eve", "age": int64(42)},
			},
		},
		{
			name: "Exclude specific fields",
			dsl: &querydsl.QueryDSL{
				Projection: &querydsl.ProjectionConfiguration{
					Exclude: []querydsl.ProjectionField{
						{Name: "created_at"},
						{Name: "balance"},
					},
				},
				Sort: []querydsl.SortConfiguration{{Field: "id", Direction: querydsl.SortDirectionAsc}},
			},
			expected: []map[string]any{
				{"id": int64(1), "first_name": "Alice", "last_name": "Smith", "age": int64(25), "access_level": "standard", "is_active": true},
				{"id": int64(2), "first_name": "Bob", "last_name": "Johnson", "age": int64(16), "access_level": "standard", "is_active": true},
				{"id": int64(3), "first_name": "Charlie", "last_name": "Brown", "age": int64(30), "access_level": "premium", "is_active": true},
				{"id": int64(4), "first_name": "Diana", "last_name": "Prince", "age": int64(17), "access_level": "premium", "is_active": false},
				{"id": int64(5), "first_name": "Eve", "last_name": "Adams", "age": int64(42), "access_level": "standard", "is_active": true},
			},
		},
		{
			name: "No projection specified (return all)",
			dsl: &querydsl.QueryDSL{
				Sort: []querydsl.SortConfiguration{{Field: "id", Direction: querydsl.SortDirectionAsc}},
			},
			expected: []map[string]any{
				{"id": int64(1), "first_name": "Alice", "last_name": "Smith", "age": int64(25), "access_level": "standard", "is_active": true, "balance": 100.5, "created_at": "2023-01-15"},
				{"id": int64(2), "first_name": "Bob", "last_name": "Johnson", "age": int64(16), "access_level": "standard", "is_active": true, "balance": 50.25, "created_at": "2023-03-20"},
				{"id": int64(3), "first_name": "Charlie", "last_name": "Brown", "age": int64(30), "access_level": "premium", "is_active": true, "balance": 1200.75, "created_at": "2023-02-10"},
				{"id": int64(4), "first_name": "Diana", "last_name": "Prince", "age": int64(17), "access_level": "premium", "is_active": false, "balance": 250.0, "created_at": "2023-04-01"},
				{"id": int64(5), "first_name": "Eve", "last_name": "Adams", "age": int64(42), "access_level": "standard", "is_active": true, "balance": 75.0, "created_at": "2023-05-05"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := exec.Execute(ctx, "users", tt.dsl)
			if err != nil {
				t.Fatalf("Execute failed for %s: %v", tt.name, err)
			}
			actualRows, ok := result.Data.([]querydsl.Row)
			if !ok {
				t.Fatalf("Expected result.Data to be []core.Row, got %T", result.Data)
			}

			actualMaps := make([]map[string]any, len(actualRows))
			for i, row := range actualRows {
				rowMap := make(map[string]any)
				maps.Copy(rowMap, row)
				actualMaps[i] = rowMap
			}

			assertResultsEqual(t, actualMaps, tt.expected)
		})
	}
}

func TestExecutorPagination(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	exec := NewSqliteExecutor(db, nil)

	// Ensure predictable order for pagination tests
	baseSort := []querydsl.SortConfiguration{
		{Field: "id", Direction: querydsl.SortDirectionAsc},
	}

	tests := []struct {
		name     string
		limit    int
		offset   *int
		expected []map[string]any
	}{
		{
			name:   "Limit 2, Offset 0",
			limit:  2,
			offset: IntPtr(0),
			expected: []map[string]any{
				{"id": int64(1), "first_name": "Alice"},
				{"id": int64(2), "first_name": "Bob"},
			},
		},
		{
			name:   "Limit 2, Offset 2",
			limit:  2,
			offset: IntPtr(2),
			expected: []map[string]any{
				{"id": int64(3), "first_name": "Charlie"},
				{"id": int64(4), "first_name": "Diana"},
			},
		},
		{
			name:   "Limit 1, Offset 4 (last record)",
			limit:  1,
			offset: IntPtr(4),
			expected: []map[string]any{
				{"id": int64(5), "first_name": "Eve"},
			},
		},
		{
			name:   "Limit 10, Offset 0 (all records)",
			limit:  10,
			offset: IntPtr(0),
			expected: []map[string]any{
				{"id": int64(1), "first_name": "Alice"},
				{"id": int64(2), "first_name": "Bob"},
				{"id": int64(3), "first_name": "Charlie"},
				{"id": int64(4), "first_name": "Diana"},
				{"id": int64(5), "first_name": "Eve"},
			},
		},
		{
			name:   "Limit 2, no offset (should default to 0)",
			limit:  2,
			offset: nil, // This should result in offset 0
			expected: []map[string]any{
				{"id": int64(1), "first_name": "Alice"},
				{"id": int64(2), "first_name": "Bob"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsl := &querydsl.QueryDSL{
				Pagination: &querydsl.PaginationOptions{
					Type:   "offset",
					Limit:  tt.limit,
					Offset: tt.offset,
				},
				Projection: &querydsl.ProjectionConfiguration{
					Include: []querydsl.ProjectionField{{Name: "id"}, {Name: "first_name"}},
				},
				Sort: baseSort,
			}
			ctx := context.Background()
			result, err := exec.Execute(ctx, "users", dsl)
			if err != nil {
				t.Fatalf("Execute failed for %s: %v", tt.name, err)
			}

			actualRows, ok := result.Data.([]querydsl.Row)
			if !ok {
				t.Fatalf("Expected result.Data to be []core.Row, got %T", result.Data)
			}

			actualMaps := make([]map[string]any, len(actualRows))
			for i, row := range actualRows {
				rowMap := make(map[string]any)
				maps.Copy(rowMap, row)
				actualMaps[i] = rowMap
			}

			assertResultsEqual(t, actualMaps, tt.expected)
		})
	}
}

func TestExecutorSQLInjectionProtection(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	exec := NewSqliteExecutor(db, nil)
	// Attempt to inject malicious SQL into table name
	maliciousTableName := `users"; DROP TABLE users; --`
	dsl := &querydsl.QueryDSL{
		Projection: &querydsl.ProjectionConfiguration{
			Include: []querydsl.ProjectionField{{Name: "id"}},
		},
	}

	ctx := context.Background()
	_, err := exec.Execute(ctx, maliciousTableName, dsl)
	if err == nil {
		t.Error("SQL injection attempt on table name succeeded unexpectedly")
	} else {
		// Verify that the table was NOT dropped
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query users table after injection attempt: %v", err)
		}
		if count != 5 { // Expecting original 5 users
			t.Errorf("Users table was affected by injection attempt (expected 5 rows, got %d)", count)
		}
		t.Logf("SQL injection attempt on table name safely handled: %v", err)
	}

	// Attempt to inject malicious SQL into field name
	maliciousFieldName := `id"; DROP TABLE users; --`
	dsl2 := &querydsl.QueryDSL{
		Projection: &querydsl.ProjectionConfiguration{
			Include: []querydsl.ProjectionField{{Name: maliciousFieldName}},
		},
	}

	_, err = exec.Execute(ctx, "users", dsl2)
	if err != nil {
		// Verify that the table was NOT dropped
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query users table after injection attempt: %v", err)
		}
		if count != 5 {
			t.Errorf("Users table was affected by injection attempt (expected 5 rows, got %d)", count)
		}
		t.Logf("SQL injection attempt on field name safely handled: %v", err)
	}

	// Attempt to inject malicious SQL into a filter value (should be handled by parameters)
	maliciousFilterValue := `Alice' OR 1=1; --`
	dsl3 := &querydsl.QueryDSL{
		Filters: &querydsl.QueryFilter{
			Condition: &querydsl.FilterCondition{
				Field:    "first_name",
				Operator: querydsl.ComparisonOperatorEq,
				Value:    maliciousFilterValue,
			},
		},
	}
	result3, err := exec.Execute(ctx, "users", dsl3)
	if err != nil {
		t.Fatalf("Execute failed for value injection test: %v", err)
	}

	actualRows, ok := result3.Data.([]querydsl.Row)
	if !ok {
		t.Fatalf("Expected result.Data to be []core.Row, got %T", result3.Data)
	}

	// Expected: Only rows where first_name is literally "Alice' OR 1=1; --"
	// (which should be none)
	if len(actualRows) != 0 {
		t.Errorf("SQL injection attempt on filter value returned unexpected results: %+v", result3.Data)
	}

	t.Logf("SQL injection attempt on filter value safely handled (returned 0 results for a non-matching string).")
}

func TestExecutorErrorHandling(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	exec := NewSqliteExecutor(db, nil)

	// Test invalid table name
	dsl := &querydsl.QueryDSL{}
	ctx := context.Background()
	_, err := exec.Execute(ctx, "", dsl)
	if err == nil || !strings.Contains(err.Error(), "table name cannot be empty") {
		t.Errorf("Expected error for empty table name, got: %v", err)
	}

	// Test unregistered Go compute function
	dsl2 := &querydsl.QueryDSL{
		Projection: &querydsl.ProjectionConfiguration{
			Computed: []querydsl.ProjectionComputedItem{
				{
					ComputedFieldExpression: &querydsl.ComputedFieldExpression{
						Type:       "computed",
						Expression: querydsl.FunctionCall{Function: "non_existent_func"},
						Alias:      "bad_field",
					},
				},
			},
		},
	}
	_, err = exec.Execute(ctx, "users", dsl2)
	if err == nil || !strings.Contains(err.Error(), "unregistered Go compute function: non_existent_func") {
		t.Errorf("Expected error for unregistered compute func, got: %v", err)
	}

	// Test unregistered Go filter function
	dsl3 := &querydsl.QueryDSL{
		Filters: &querydsl.QueryFilter{
			Condition: &querydsl.FilterCondition{
				Field:    "name",
				Operator: "non_existent_filter",
				Value:    nil,
			},
		},
	}
	_, err = exec.Execute(ctx, "users", dsl3)
	if err == nil || !strings.Contains(err.Error(), "unregistered Go filter function for operator: non_existent_filter") {
		t.Errorf("Expected error for unregistered filter func, got: %v", err)
	}
}

func TestExecutorConcurrencySafety(t *testing.T) {
	db := setupTestDB(t) // This now uses a shared in-memory database
	defer db.Close()
	exec := NewSqliteExecutor(db, nil)

	var wg sync.WaitGroup
	numGoroutines := 100
	executedQueries := make(chan error, numGoroutines)

	// Ensure essential compute/filter functions are registered BEFORE concurrent execution
	exec.RegisterComputeFunction("full_name_calc", testGoComputeFullName)
	exec.RegisterFilterFunction("is_adult_check", testGoFilterIsAdult)

	// Execute queries concurrently
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Create a DSL that might use a Go function, ensuring fields needed are projected
			dsl := &querydsl.QueryDSL{
				Filters: &querydsl.QueryFilter{
					Condition: &querydsl.FilterCondition{
						Field:    "id",
						Operator: querydsl.ComparisonOperatorEq,
						Value:    id%5 + 1, // Query different IDs
					},
				},
				Projection: &querydsl.ProjectionConfiguration{
					Include: []querydsl.ProjectionField{
						{Name: "first_name"},
						{Name: "last_name"}, // Required for full_name_calc
						{Name: "id"},
					},
					Computed: []querydsl.ProjectionComputedItem{
						{
							ComputedFieldExpression: &querydsl.ComputedFieldExpression{
								Type:       "computed",
								Expression: querydsl.FunctionCall{Function: "full_name_calc"},
								Alias:      "computed_name",
							},
						},
					},
				},
			}

			_, err := exec.Execute(context.Background(), "users", dsl)
			executedQueries <- err
		}(i)
	}

	wg.Wait()
	close(executedQueries)

	for err := range executedQueries {
		if err != nil {
			t.Errorf("Concurrent query execution failed: %v", err)
		}
	}
	// No panics or race conditions and no "no such table" errors indicate basic concurrency safety
}
