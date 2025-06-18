# `querydsl`

[![Go Reference](https://pkg.go.dev/badge/github.com/asaidimu/querydsl.svg)](https://pkg.go.dev/github.com/asaidimu/querydsl)
[![Build Status](https://github.com/asaidimu/querydsl/workflows/Test%20Workflow/badge.svg)](https://github.com/asaidimu/querydsl/actions)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.24.4+-00ADD8?logo=go)](https://go.dev/doc/go1.24)

## Table of Contents
- [Overview & Features](#overview--features)
- [Installation & Setup](#installation--setup)
- [Usage Documentation](#usage-documentation)
  - [Defining a QueryDSL](#defining-a-querydsl)
  - [Basic Query Execution](#basic-query-execution)
  - [Registering Custom Go Functions](#registering-custom-go-functions)
  - [Using Go Functions in Queries](#using-go-functions-in-queries)
  - [Advanced Usage Examples](#advanced-usage-examples)
- [Project Architecture](#project-architecture)
- [Development & Contributing](#development--contributing)
- [Additional Information](#additional-information)

---

## Overview & Features

`querydsl` is a Go companion to [`@asidimu/query`](https://www.npmjs.com/package/@asaidimu/query) designed to provide a flexible, declarative Query Domain Specific Language (DSL) for interacting with databases. It aims to bridge the gap between simple queries and complex application-specific logic by offering a hybrid query execution model. This model intelligently delegates standard filtering, sorting, and pagination operations to the underlying database for performance, while allowing complex data transformations and custom filter conditions to be executed in-memory using Go functions.

This design enables developers to define sophisticated data retrieval patterns without writing raw SQL for every scenario, improving code readability, maintainability, and reusability. The hybrid approach ensures that database capabilities are leveraged where they excel, and Go's power is utilized for business logic that might be cumbersome or inefficient to express purely in SQL.

### Key Features

*   ⚙️ **Declarative Query DSL**: Define data retrieval and manipulation using a structured Go object (`QueryDSL`) instead of concatenating strings. Supports filtering (`WHERE`), projection (`SELECT`), sorting (`ORDER BY`), and pagination (`LIMIT`/`OFFSET`).
*   ⚡ **Hybrid Query Execution**: Automatically generates efficient SQL for standard database operations and then applies custom logic using registered Go functions in memory.
*   🧩 **Extensible with Go Functions**: Register custom `GoComputeFunction`s to derive new fields from existing row data and `GoFilterFunction`s to implement complex, application-specific filtering logic.
*   📦 **SQLite Implementation**: Provides a concrete `SqliteExecutor` for seamless integration with SQLite databases, leveraging the `github.com/mattn/go-sqlite3` driver.
*   🔒 **SQL Injection Protection**: Built-in mechanisms (parameterized queries, identifier quoting) protect against common SQL injection vulnerabilities.
*   🎯 **Intelligent Field Selection**: Automatically determines which fields need to be selected from the database, including those required by custom Go functions, to ensure correct processing.

---

## Installation & Setup

### Prerequisites

*   **Go**: Version `1.24.4` or higher.
*   **Database Driver**: For the provided SQLite implementation, you will need the `github.com/mattn/go-sqlite3` driver.

### Installation Steps

To add `querydsl` to your Go project, run:

```bash
go get github.com/asaidimu/querydsl
```

### Configuration

The core of `querydsl` relies on a `QueryExecutor` implementation. For SQLite, you instantiate `SqliteExecutor` with a standard `*sql.DB` connection:

```go
package main

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3" // Import the SQLite driver
	"github.com/asaidimu/querydsl/pkg/sqlite"
)

func main() {
	db, err := sql.Open("sqlite3", "file:./test.db?cache=shared")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Optionally, ping the database to ensure connection is live
	if err = db.Ping(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	executor := sqlite.NewSqliteExecutor(db)
	log.Println("SqliteExecutor initialized successfully.")

	// You can now use the executor to run queries
	// See Usage Documentation for examples.
}
```

### Verification

You can verify the installation and functionality by running the project's tests:

```bash
cd /path/to/your/go/pkg/mod/github.com/asaidimu/querydsl@vX.Y.Z # or your project root if vendored
make test
```

This will execute unit and integration tests against an in-memory SQLite database.

---

## Usage Documentation

`querydsl` operates by taking a `QueryDSL` struct, which declaratively describes the desired query, and executing it against a specified database table.

### Defining a QueryDSL

The `querydsl.QueryDSL` struct is your primary interface for building queries.

```go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
	querydsl "github.com/asaidimu/querydsl/pkg/core"
	"github.com/asaidimu/querydsl/pkg/sqlite"
)

func initDB() *sql.DB {
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	createTableSQL := `
	CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		first_name TEXT,
		last_name TEXT,
		age INTEGER,
		access_level TEXT,
		is_active BOOLEAN,
		balance REAL
	);
	INSERT INTO users (id, first_name, last_name, age, access_level, is_active, balance) VALUES
	(1, 'Alice', 'Smith', 25, 'standard', TRUE, 100.50),
	(2, 'Bob', 'Johnson', 16, 'standard', TRUE, 50.25),
	(3, 'Charlie', 'Brown', 30, 'premium', TRUE, 1200.75);
	`
	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatalf("Failed to create table and insert data: %v", err)
	}
	return db
}

// Helper for pointer to int
func IntPtr(i int) *int {
	return &i
}

func main() {
	db := initDB()
	defer db.Close()

	executor := sqlite.NewSqliteExecutor(db)
	ctx := context.Background()

	// --- 1. Basic Query: Filter, Project, Sort, Paginate ---
	fmt.Println("\n--- Basic Query: Active users over 18, showing name & age, sorted by age (desc), first 2 records ---")
	basicDSL := &querydsl.QueryDSL{
		Filters: &querydsl.QueryFilter{
			Group: &querydsl.FilterGroup{
				Operator: querydsl.LogicalOperatorAnd,
				Conditions: []querydsl.QueryFilter{
					{
						Condition: &querydsl.FilterCondition{
							Field:    "age",
							Operator: querydsl.ComparisonOperatorGt,
							Value:    18,
						},
					},
					{
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
				{Name: "last_name"},
				{Name: "age"},
			},
		},
		Sort: []querydsl.SortConfiguration{
			{Field: "age", Direction: querydsl.SortDirectionDesc},
		},
		Pagination: &querydsl.PaginationOptions{
			Type:  "offset",
			Limit: 2,
			Offset: IntPtr(0),
		},
	}

	result, err := executor.Execute(ctx, "users", basicDSL)
	if err != nil {
		log.Fatalf("Basic query failed: %v", err)
	}
	fmt.Printf("Results: %+v\n", result.Data)
	// Expected output (approx):
	// Results: [map[age:30 first_name:Charlie last_name:Brown] map[age:25 first_name:Alice last_name:Smith]]
}
```

### Basic Query Execution

Once you have an `Executor` instance, you can run queries:

```go
// (Pre-requisites: db setup and executor initialization as above)

// Example: Select all users with age greater than 20, ordered by age ascending.
dsl := &querydsl.QueryDSL{
	Filters: &querydsl.QueryFilter{
		Condition: &querydsl.FilterCondition{
			Field:    "age",
			Operator: querydsl.ComparisonOperatorGt,
			Value:    20,
		},
	},
	Sort: []querydsl.SortConfiguration{
		{Field: "age", Direction: querydsl.SortDirectionAsc},
	},
	Projection: &querydsl.ProjectionConfiguration{
		Include: []querydsl.ProjectionField{
			{Name: "id"},
			{Name: "first_name"},
			{Name: "age"},
		},
	},
}

ctx := context.Background()
result, err := executor.Execute(ctx, "users", dsl)
if err != nil {
	log.Fatalf("Error executing query: %v", err)
}

fmt.Println("Users over 20:")
for _, row := range result.Data.([]querydsl.Row) {
	fmt.Printf("  ID: %v, Name: %v, Age: %v\n", row["id"], row["first_name"], row["age"])
}
// Example Output:
// Users over 20:
//   ID: 1, Name: Alice, Age: 25
//   ID: 3, Name: Charlie, Age: 30
```

### Registering Custom Go Functions

`querydsl` allows you to extend its capabilities by registering Go functions that can perform custom computations or filtering directly on the `querydsl.Row` data after it's fetched from the database.

*   `GoComputeFunction`: Used to add new, derived fields to your query results.
*   `GoFilterFunction`: Used to apply custom, complex filtering logic that might be difficult or impossible to express in standard SQL.

```go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
	querydsl "github.com/asaidimu/querydsl/pkg/core"
	"github.com/asaidimu/querydsl/pkg/sqlite"
)

// (initDB and IntPtr functions as defined above)

// A custom GoComputeFunction to combine first and last names.
func goComputeFullName(row querydsl.Row) (any, error) {
	firstName, ok1 := row["first_name"].(string)
	lastName, ok2 := row["last_name"].(string)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("missing first_name or last_name for full_name computation")
	}
	return fmt.Sprintf("%s %s", firstName, lastName), nil
}

// A custom GoFilterFunction to check if a user is considered an adult (age >= 18).
func goFilterIsAdult(row querydsl.Row) (bool, error) {
	age, ok := row["age"].(int64)
	if !ok {
		return false, fmt.Errorf("age not found or not int64 for is_adult filter")
	}
	return age >= 18, nil
}

func main() {
	db := initDB()
	defer db.Close()
	executor := sqlite.NewSqliteExecutor(db)

	// Register your custom Go functions with the executor
	executor.RegisterComputeFunction("full_name_calc", goComputeFullName)
	executor.RegisterFilterFunction("is_adult_check", goFilterIsAdult)

	log.Println("Custom Go functions registered.")

	// Now you can use these functions in your QueryDSL
	// See "Using Go Functions in Queries"
}
```

**Important Note**: When using `GoComputeFunction` or `GoFilterFunction`, ensure that all fields required by your Go function (e.g., `first_name`, `last_name`, `age` in the examples above) are either explicitly `Include`d in your `ProjectionConfiguration` or not `Exclude`d (if not using `Include`). The `SqliteExecutor` attempts to infer these dependencies, but explicit inclusion is best practice.

### Using Go Functions in Queries

After registration, custom Go functions can be referenced in your `QueryDSL`:

```go
// (Pre-requisites: db setup, executor initialization, and Go functions registered)
// From the main function example above

	ctx := context.Background()

	// --- 2. Query with Go Computed Field ---
	fmt.Println("\n--- Query with Go Computed Field: Add a 'full_name' field ---")
	computedDSL := &querydsl.QueryDSL{
		Projection: &querydsl.ProjectionConfiguration{
			Include: []querydsl.ProjectionField{
				{Name: "id"},
				{Name: "first_name"},
				{Name: "last_name"},
			},
			Computed: []querydsl.ProjectionComputedItem{
				{
					ComputedFieldExpression: &querydsl.ComputedFieldExpression{
						Type:       "computed",
						Expression: querydsl.FunctionCall{Function: "full_name_calc"}, // Reference the registered function name
						Alias:      "full_name", // The name of the new field in the result
					},
				},
			},
		},
		Sort: []querydsl.SortConfiguration{
			{Field: "id", Direction: querydsl.SortDirectionAsc},
		},
	}

	computedResult, err := executor.Execute(ctx, "users", computedDSL)
	if err != nil {
		log.Fatalf("Computed field query failed: %v", err)
	}
	fmt.Println("Users with 'full_name' field:")
	for _, row := range computedResult.Data.([]querydsl.Row) {
		fmt.Printf("  ID: %v, Full Name: %v\n", row["id"], row["full_name"])
	}
	// Example Output:
	// Users with 'full_name' field:
	//   ID: 1, Full Name: Alice Smith
	//   ID: 2, Full Name: Bob Johnson
	//   ID: 3, Full Name: Charlie Brown

	// --- 3. Query with Go Filter Function ---
	fmt.Println("\n--- Query with Go Filter Function: Filter for adults using 'is_adult_check' ---")
	filterDSL := &querydsl.QueryDSL{
		Filters: &querydsl.QueryFilter{
			Condition: &querydsl.FilterCondition{
				Field:    "age", // This field is used by the 'is_adult_check' Go function
				Operator: "is_adult_check", // Reference the registered filter function name
				Value:    nil, // The Go function doesn't need a value here
			},
		},
		Projection: &querydsl.ProjectionConfiguration{
			Include: []querydsl.ProjectionField{
				{Name: "id"},
				{Name: "first_name"},
				{Name: "age"}, // 'age' must be included for the Go filter to access it
			},
		},
		Sort: []querydsl.SortConfiguration{
			{Field: "id", Direction: querydsl.SortDirectionAsc},
		},
	}

	filterResult, err := executor.Execute(ctx, "users", filterDSL)
	if err != nil {
		log.Fatalf("Go filter query failed: %v", err)
	}
	fmt.Println("Adult users (age >= 18):")
	for _, row := range filterResult.Data.([]querydsl.Row) {
		fmt.Printf("  ID: %v, Name: %v, Age: %v\n", row["id"], row["first_name"], row["age"])
	}
	// Example Output:
	// Adult users (age >= 18):
	//   ID: 1, Name: Alice, Age: 25
	//   ID: 3, Name: Charlie, Age: 30
}
```

### Advanced Usage Examples

`querydsl` supports nested logical operators and mixed SQL/Go conditions:

```go
// (Pre-requisites: db setup, executor initialization, and Go functions registered)
// Continuing from the main function example...

	// --- 4. Mixed Filters (DB-native and Go-based) with Logical Operators ---
	fmt.Println("\n--- Mixed Filters: Premium users OR (standard active adults) ---")
	mixedFilterDSL := &querydsl.QueryDSL{
		Filters: &querydsl.QueryFilter{
			Group: &querydsl.FilterGroup{
				Operator: querydsl.LogicalOperatorOr,
				Conditions: []querydsl.QueryFilter{
					{ // Condition 1: DB-native - premium access
						Condition: &querydsl.FilterCondition{
							Field:    "access_level",
							Operator: querydsl.ComparisonOperatorEq,
							Value:    "premium",
						},
					},
					{ // Condition 2: Group of conditions (DB-native AND Go-based)
						Group: &querydsl.FilterGroup{
							Operator: querydsl.LogicalOperatorAnd,
							Conditions: []querydsl.QueryFilter{
								{
									Condition: &querydsl.FilterCondition{
										Field:    "access_level",
										Operator: querydsl.ComparisonOperatorEq,
										Value:    "standard",
									},
								},
								{
									Condition: &querydsl.FilterCondition{
										Field:    "is_active",
										Operator: querydsl.ComparisonOperatorEq,
										Value:    true,
									},
								},
								{
									Condition: &querydsl.FilterCondition{
										Field:    "age",
										Operator: "is_adult_check", // Go filter
										Value:    nil,
									},
								},
							},
						},
					},
				},
			},
		},
		Projection: &querydsl.ProjectionConfiguration{
			Include: []querydsl.ProjectionField{
				{Name: "id"},
				{Name: "first_name"},
				{Name: "access_level"},
				{Name: "age"}, // Needed for 'is_adult_check'
				{Name: "is_active"},
			},
		},
		Sort: []querydsl.SortConfiguration{
			{Field: "id", Direction: querydsl.SortDirectionAsc},
		},
	}

	mixedFilterResult, err := executor.Execute(ctx, "users", mixedFilterDSL)
	if err != nil {
		log.Fatalf("Mixed filter query failed: %v", err)
	}
	fmt.Println("Users with premium access OR (standard, active, and adult):")
	for _, row := range mixedFilterResult.Data.([]querydsl.Row) {
		fmt.Printf("  ID: %v, Name: %v, Access: %v, Age: %v, Active: %v\n",
			row["id"], row["first_name"], row["access_level"], row["age"], row["is_active"])
	}
	// Expected Output:
	// Users with premium access OR (standard, active, and adult):
	//   ID: 1, Name: Alice, Access: standard, Age: 25, Active: true
	//   ID: 3, Name: Charlie, Access: premium, Age: 30, Active: true
```

---

## Project Architecture

`querydsl` is structured to provide a clear separation between the core DSL definition and specific database implementations.

### Core Components

*   **`pkg/core`**: This package defines the `QueryDSL` structure and all the fundamental types (like `FilterCondition`, `ProjectionConfiguration`, `SortConfiguration`, `PaginationOptions`, `LogicalOperator`, `ComparisonOperator`, `Row`, `GoComputeFunction`, `GoFilterFunction`). Crucially, it also defines the `QueryExecutor` interface, which all database-specific implementations must satisfy. This package is database-agnostic.
*   **`pkg/sqlite`**: This package provides a concrete implementation of the `QueryExecutor` interface specifically for SQLite databases.
    *   **`SqliteQuery`**: Responsible for translating the database-native parts of a `QueryDSL` into actual SQLite SQL strings and their corresponding parameters. It handles standard SQL operations like `SELECT`, `WHERE`, `ORDER BY`, `LIMIT`, and `OFFSET`, ensuring proper SQL injection protection. It explicitly skips custom operators meant for Go function processing.
    *   **`SqliteExecutor`**: This is the orchestrator. It receives the `QueryDSL`, first uses `SqliteQuery` to generate the SQL for the database-executable parts, executes that SQL, reads the results into `querydsl.Row` objects, and then applies any registered `GoFilterFunction`s (for custom filtering) and `GoComputeFunction`s (for computed fields) in memory. Finally, it applies the user-defined `ProjectionConfiguration` to shape the final output.

### Data Flow

The execution flow for a `QueryDSL` request is as follows:

1.  **Request Initiation**: A client constructs a `querydsl.QueryDSL` object and calls `executor.Execute(ctx, tableName, dsl)`.
2.  **Field Determination**: The `SqliteExecutor` analyzes the `QueryDSL` to identify *all* fields (from explicit projection, filters, and those required by registered Go functions) that need to be fetched from the database.
3.  **SQL Generation**: The `SqliteExecutor` passes a modified `QueryDSL` (containing all required fields for selection) to `SqliteQuery.Generate`. The `SqliteQuery` builds the `SELECT`, `WHERE` (for standard conditions only), `ORDER BY`, `LIMIT`, and `OFFSET` clauses.
4.  **Database Execution**: The generated SQL query is executed against the `*sql.DB` connection.
5.  **Row Materialization**: The raw `sql.Rows` result set is read and converted into a slice of `querydsl.Row` (Go `map[string]any`) objects.
6.  **Go Filter Application**: If any custom `GoFilterFunction`s were specified in the `QueryDSL`, the `SqliteExecutor` iterates through the `querydsl.Row` slice and applies these filters in memory, removing rows that do not pass.
7.  **Go Compute Application**: If any custom `GoComputeFunction`s were specified, the `SqliteExecutor` iterates through the filtered `querydsl.Row` slice and adds new, computed fields to each `Row` as defined by these functions.
8.  **Final Projection**: The `SqliteExecutor` applies the user's final `ProjectionConfiguration` (including or excluding fields) to shape the output `querydsl.Row` objects to exactly what was requested.
9.  **Result Return**: The processed `querydsl.Row` slice is wrapped in a `QueryResult` object and returned to the caller.

### Extension Points

The primary extension points of `querydsl` are:

*   **Custom Go Functions**: The `RegisterComputeFunction` and `RegisterFilterFunction` methods allow users to add powerful, custom logic that operates directly on the `querydsl.Row` data.
*   **Database Implementations**: The `QueryExecutor` interface provides a clear contract for building new database-specific implementations (e.g., for PostgreSQL, MySQL, etc.) by creating a new package under `pkg/` that satisfies this interface.

---

## Development & Contributing

Contributions to `querydsl` are welcome! Please follow these guidelines to ensure a smooth development process.

### Development Setup

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/asaidimu/querydsl.git
    cd querydsl
    ```
2.  **Download dependencies:**
    ```bash
    go mod tidy
    ```

### Scripts

The project uses a `Makefile` for common development tasks:

*   **`make build`**: Compiles the project.
    ```bash
    make build
    ```
*   **`make test`**: Runs all unit and integration tests.
    ```bash
    make test
    ```
*   **`make clean`**: Removes compiled binaries.
    ```bash
    make clean
    ```

### Testing

All tests are located in the `pkg/*/` directories, typically in `_test.go` files.
To run tests:

```bash
make test
# or with verbose output and race detection
go test -v -race ./...
```

New features and bug fixes should include corresponding tests to ensure correctness and prevent regressions.

### Contributing Guidelines

1.  **Fork the repository**.
2.  **Create a new branch** from `main` for your feature or bug fix: `git checkout -b feature/your-feature-name`.
3.  **Implement your changes**, adhering to Go coding standards (`gofmt` and `golint`).
4.  **Write tests** for your changes.
5.  **Ensure all tests pass** (`make test`).
6.  **Commit your changes** with a clear and concise commit message.
7.  **Push your branch** to your fork.
8.  **Open a Pull Request** to the `main` branch of the original repository.
    *   Provide a detailed description of your changes.
    *   Reference any relevant issues.

### Issue Reporting

Please report any bugs or suggest features by opening an issue on the GitHub Issues page:
[https://github.com/asaidimu/querydsl/issues](https://github.com/asaidimu/querydsl/issues)

---

## Additional Information

### Troubleshooting

*   **"unregistered Go compute function" / "unregistered Go filter function"**:
    Ensure you have called `executor.RegisterComputeFunction()` or `executor.RegisterFilterFunction()` for the specific function name/operator you are trying to use *before* executing the query.
*   **Missing Fields in Go Functions**:
    If your `GoComputeFunction` or `GoFilterFunction` panics or returns an error about a missing field (e.g., `row["some_field"]` is nil or wrong type), ensure that `some_field` is explicitly included in your `ProjectionConfiguration` or not excluded, so that it is fetched from the database and available to your Go function. The executor tries to infer, but explicit projection is safest.
*   **Database Connection Errors**:
    Verify your database connection string and credentials. Ensure the SQLite file exists or is correctly configured for in-memory operation.

### FAQ

*   **Why a hybrid approach (SQL + Go functions)?**
    This approach allows leveraging the database's optimized query processing for standard operations (filtering large datasets, complex joins) while providing the full flexibility of Go for complex business logic, custom aggregations, or specialized filtering that would be inefficient or impossible to implement in pure SQL. It enables powerful dynamic queries without compromising performance on core database tasks.
*   **When should I use a Go function versus a standard SQL operation?**
    *   **SQL Operations**: Use for simple comparisons (`=`, `>`, `IN`), basic string matching (`LIKE`), or when dealing with very large datasets where filtering at the database level is crucial for performance.
    *   **Go Functions**: Use for:
        *   Complex conditional logic involving multiple fields or external data.
        *   Custom data transformations (e.g., parsing specific string formats, applying custom algorithms).
        *   Accessing non-database resources as part of a filter or computation.
        *   Logic that is already implemented in Go and not easily translatable to SQL.

### Changelog / Roadmap

*   [**Changelog**](CHANGELOG.md): Refer to the `CHANGELOG.md` file for a history of changes.
*   **Roadmap**: Future enhancements may include:
    *   Support for `CASE` expressions in `Projection` (currently only `ComputedFieldExpression` is fully supported).
    *   Implementations for other popular databases (PostgreSQL, MySQL).
    *   Enhanced error reporting and type safety.
    *   Support for `Joins`, `Aggregations`, and `Window Functions` in the DSL and executor.
    *   More sophisticated dependency inference for Go functions.

### License

This project is licensed under the MIT License. See the [LICENSE.md](LICENSE.md) file for details.

### Acknowledgments

*   This project is inspired by various Query DSL implementations in different languages and frameworks.
*   Special thanks to the `github.com/mattn/go-sqlite3` project for providing a robust SQLite driver for Go.

---
_Copyright (c) 2025 Saidimu_
