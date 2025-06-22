package core

import (
	"context"
)

// Row represents a single record/row of data retrieved from the database.
// This is the input/output type for your pure Go functions.
type Row map[string]any

// GoComputeFunction is a pure Go function that computes a new value for a row.
// It takes a Row (representing the current data) and returns the computed value
// for a new field, and an error if computation fails.
type GoComputeFunction func(row Row) (any, error)

// GoFilterFunction is a pure Go function that performs custom filtering logic on a row.
// It takes a Row and returns true if the row passes the filter, false otherwise,
// and an error if evaluation fails.
type GoFilterFunction func(row Row) (bool, error)

// QueryExecutor defines the interface for executing queries against a database
// using a QueryDSL object, and applying Go-based logic post-retrieval.
type QueryExecutor interface {
	// RegisterComputeFunction registers a single GoComputeFunction
	// under a specific name. This name will be used in the QueryDSL's FunctionCall
	// or ComputedFieldExpression to reference this Go function.
	RegisterComputeFunction(name string, fn GoComputeFunction)

	// RegisterFilterFunction registers a single GoFilterFunction
	// under a specific comparison operator name. This name will be used
	// in the QueryDSL's FilterCondition to reference this Go function.
	RegisterFilterFunction(operator ComparisonOperator, fn GoFilterFunction)

	// RegisterComputeFunctions registers multiple GoComputeFunction functions
	// from a map. This is a convenient way to register a batch of functions.
	RegisterComputeFunctions(functionMap map[string]GoComputeFunction)

	// RegisterFilterFunctions registers multiple GoFilterFunction functions
	// from a map.
	RegisterFilterFunctions(functionMap map[ComparisonOperator]GoFilterFunction)

	// Update performs an update operation on the database based on the provided
	// update data and filters.
	// It returns the number of rows affected and an error.
	Update(ctx context.Context, updates map[string]any, filters QueryFilter) (int64, error)

	// Insert performs an insert operation and returns the inserted records as they exist in the database.
	// This implementation uses the `RETURNING` clause (requires SQLite 3.35+)
	// to atomically retrieve the inserted data, including all database-applied values
	// (auto-generated primary keys, defaults, timestamps, etc.).
	Insert(ctx context.Context, records []map[string]any) (*QueryResult, error)

	// Delete performs a delete operation with optional filters for safety.
	// By default, requires a WHERE clause to prevent accidental deletion of all records.
	// Set unsafeDelete to true to allow deletion without WHERE clause.
	// Returns the number of rows affected and an error.
	Delete(ctx context.Context, filters QueryFilter, unsafeDelete bool) (int64, error)

	// Query processes the QueryDSL, first by generating and running SQL
	// for database-executable parts, then by applying registered Go functions
	// for computations and custom filters.
	// It returns the final processed results and any associated metadata.
	Query(ctx context.Context, dsl *QueryDSL) (*QueryResult, error)
}
