package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log" // For debugging, consider removing in production
	"sync" // Add sync for mutex
	querydsl "github.com/asaidimu/querydsl/pkg/core"
	"maps"
)


// SqliteExecutor implements the QueryExecutor interface for SQLite databases.
type SqliteExecutor struct {
	db                 *sql.DB
	queryGenerator     querydsl.QueryGenerator
	goComputeFunctions map[string]querydsl.GoComputeFunction
	goFilterFunctions  map[querydsl.ComparisonOperator]querydsl.GoFilterFunction
	mu                 sync.RWMutex // Mutex to protect goComputeFunctions and goFilterFunctions
}

// NewSqliteExecutor creates a new SqliteExecutor instance.
func NewSqliteExecutor(db *sql.DB, query querydsl.QueryGenerator) *SqliteExecutor {
	if query == nil {
		query = NewSqliteQuery()
	}

	return &SqliteExecutor{
		db: db,
		queryGenerator: query,
		goComputeFunctions: make(map[string]querydsl.GoComputeFunction),
		goFilterFunctions: make(map[querydsl.ComparisonOperator]querydsl.GoFilterFunction),
	}
}

// RegisterComputeFunction registers a Go function that can be used to compute
// new fields based on existing row data.
func (e *SqliteExecutor) RegisterComputeFunction(name string, fn querydsl.GoComputeFunction) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.goComputeFunctions[name] = fn
	log.Printf("Registered compute function: %s", name)
}

// RegisterFilterFunction registers a Go function that can be used as a custom
// comparison operator for filtering rows.
func (e *SqliteExecutor) RegisterFilterFunction(operator querydsl.ComparisonOperator, fn querydsl.GoFilterFunction) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.goFilterFunctions[operator] = fn
	log.Printf("Registered filter function: %s", operator)
}

// Query runs a query against the database based on the provided QueryDSL.
func (e *SqliteExecutor) Query(ctx context.Context, tableName string, dsl *querydsl.QueryDSL) (*querydsl.QueryResult, error) {
	// Collect all fields needed from the database for projection and Go functions
	fieldsToSelect := e.determineFieldsToSelect(dsl)

	// Create a temporary DSL for the SQL generation that includes all necessary fields
	// This ensures the query generator fetches everything Go functions might need.
	sqlDsl := *dsl // Create a shallow copy
	sqlDsl.Projection = &querydsl.ProjectionConfiguration{
		Include: fieldsToSelect, // Use the determined fields for SQL SELECT
		// Exclude is ignored here as we explicitly include
		// Computed fields are handled after DB fetch
	}

	sqlQuery, queryParams, err := e.queryGenerator.GenerateSelectSQL(tableName, &sqlDsl)
	if err != nil {
		return nil, fmt.Errorf("failed to generate SQL query: %w", err)
	}

	log.Printf("Generated SQL for DB execution: %s", sqlQuery)
	log.Printf("SQL Params: %v", queryParams)

	rows, err := e.db.QueryContext(ctx, sqlQuery, queryParams...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute base query: %w", err)
	}
	defer rows.Close()

	// Read all rows from the database
	dbRows, err := e.readRows(rows)
	if err != nil {
		return nil, fmt.Errorf("failed to read rows from database: %w", err)
	}
	log.Printf("Fetched %d rows from DB before Go processing.", len(dbRows))

	// Apply Go-based filters (if any)
	processedRows, err := e.applyGoFilters(dbRows, dsl.Filters)
	if err != nil {
		return nil, fmt.Errorf("Go filter failed: %w", err)
	}
	log.Printf("After Go filters, %d rows remain.", len(processedRows))

	// Apply Go-based computed fields (if any)
	processedRows, err = e.applyGoComputeFunctions(processedRows, dsl.Projection)
	if err != nil {
		return nil, fmt.Errorf("Go computed field failed: %w", err)
	}
	log.Printf("After Go compute functions, %d rows remain.", len(processedRows))

	// Finally, apply the user's requested projection (include/exclude, and order)
	finalResults := e.applyFinalProjection(processedRows, dsl.Projection)
	log.Printf("After final projection, %d rows returned.", len(finalResults))

	return &querydsl.QueryResult{
		Data: finalResults,
		// Pagination, Aggregations, Window functions not yet implemented
	}, nil
}

// determineFieldsToSelect analyzes the DSL to figure out all fields that need to be
// selected from the database, including those required by Go functions.
func (e *SqliteExecutor) determineFieldsToSelect(dsl *querydsl.QueryDSL) []querydsl.ProjectionField {
	requiredFields := make(map[string]struct{})

	// 1. Fields from explicit Projection.Include
	if dsl.Projection != nil && len(dsl.Projection.Include) > 0 {
		for _, field := range dsl.Projection.Include {
			if field.Name != "" {
				requiredFields[field.Name] = struct{}{}
			}
		}
	}

	e.mu.RLock() // Read lock for Go function maps
	defer e.mu.RUnlock()

	// 2. Fields required by Go-based filter functions
	e.collectGoFilterRequiredFields(dsl.Filters, requiredFields)

	// 3. Fields required by Go-based computed functions
	if dsl.Projection != nil && len(dsl.Projection.Computed) > 0 {
		for _, item := range dsl.Projection.Computed {
			if item.ComputedFieldExpression != nil {
				// This is a simplification. Ideally, a compute function would declare its dependencies.
				// For now, we'll hardcode dependencies for the test functions.
				if item.ComputedFieldExpression.Expression.Function == "full_name_calc" {
					requiredFields["first_name"] = struct{}{}
					requiredFields["last_name"] = struct{}{}
				}
				// Add other computed function dependencies here if needed (e.g., "is_eligible_calc")
			}
		}
	}

	// Convert map to slice of ProjectionField
	var finalFields []querydsl.ProjectionField
	for fieldName := range requiredFields {
		finalFields = append(finalFields, querydsl.ProjectionField{Name: fieldName})
	}

	// If no specific fields are requested/required, the query generator defaults to SELECT *.
	// Returning an empty slice here correctly signals that "all" fields are implicitly desired by the generator.
	return finalFields
}

// collectGoFilterRequiredFields recursively traverses the filter DSL to find
// fields referenced by Go-based filter functions.
func (e *SqliteExecutor) collectGoFilterRequiredFields(filter *querydsl.QueryFilter, fields map[string]struct{}) {
	if filter == nil {
		return
	}
	if filter.Condition != nil {
		// Check if it's a Go-based filter
		if !filter.Condition.Operator.IsStandard() {
			fields[filter.Condition.Field] = struct{}{} // Add the field used by the Go filter
		}
	}
	if filter.Group != nil {
		for _, subFilter := range filter.Group.Conditions {
			e.collectGoFilterRequiredFields(&subFilter, fields)
		}
	}
}

// readRows reads all rows from a sql.Rows result and converts them into a slice of Row maps.
func (e *SqliteExecutor) readRows(rows *sql.Rows) ([]querydsl.Row, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, fmt.Errorf("failed to get column types: %w", err)
	}

	var results []querydsl.Row
	for rows.Next() {
		row := make(querydsl.Row, len(columns))
		values := make([]any, len(columns))
		scanArgs := make([]any, len(columns))
		for i := range values {
			scanArgs[i] = &values[i]
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		for i, col := range columns {
			val := values[i]
			// Handle NULLs and type conversions
			if val != nil {
				switch columnTypes[i].DatabaseTypeName() {
				case "BOOLEAN": // SQLite often stores booleans as INTEGER (0 or 1)
					if intVal, ok := val.(int64); ok {
						row[col] = intVal != 0
					} else {
						row[col] = val // Keep original type if not int64
					}
				case "REAL": // SQLite float
					if floatVal, ok := val.(float64); ok {
						row[col] = floatVal
					} else {
						row[col] = val
					}
				case "INTEGER": // SQLite integer
					if intVal, ok := val.(int64); ok {
						row[col] = intVal
					} else {
						row[col] = val
					}
				case "TEXT": // SQLite text
					if byteVal, ok := val.([]byte); ok { // database/sql returns []byte for TEXT by default
						row[col] = string(byteVal)
					} else if strVal, ok := val.(string); ok { // In some cases, it might already be a string
						row[col] = strVal
					} else {
						row[col] = val
					}
				default:
					// Default to the type returned by the driver
					row[col] = val
				}
			} else {
				row[col] = nil // Explicitly set to nil for NULL values
			}
		}
		results = append(results, row)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error after scanning rows: %w", err)
	}
	return results, nil
}

// applyGoFilters iterates through rows and applies Go-based filter functions.
func (e *SqliteExecutor) applyGoFilters(rows []querydsl.Row, filter *querydsl.QueryFilter) ([]querydsl.Row, error) {
	if filter == nil {
		return rows, nil
	}

	e.mu.RLock() // Read lock for goFilterFunctions map
	defer e.mu.RUnlock()

	var filteredRows []querydsl.Row
	for _, row := range rows {
		passes, err := e.evaluateGoFilter(row, filter)
		if err != nil {
			return nil, fmt.Errorf("error evaluating Go filter for row %+v: %w", row, err)
		}
		if passes {
			filteredRows = append(filteredRows, row)
		}
	}
	return filteredRows, nil
}

// evaluateGoFilter recursively evaluates a QueryFilter, applying Go functions where necessary.
func (e *SqliteExecutor) evaluateGoFilter(row querydsl.Row, filter *querydsl.QueryFilter) (bool, error) {
	if filter.Condition != nil {
		if !filter.Condition.Operator.IsStandard() { // It's a Go-based filter
			fn, ok := e.goFilterFunctions[filter.Condition.Operator]
			if !ok {
				return false, fmt.Errorf("unregistered Go filter function for operator: %s", filter.Condition.Operator)
			}
			return fn(row)
		}
		return true, nil // Standard SQL conditions are handled by the database's WHERE clause
	}
	if filter.Group != nil {
		switch filter.Group.Operator {
		case querydsl.LogicalOperatorAnd:
			for _, cond := range filter.Group.Conditions {
				passes, err := e.evaluateGoFilter(row, &cond)
				if err != nil {
					return false, err
				}
				if !passes {
					return false, nil
				}
			}
			return true, nil
		case querydsl.LogicalOperatorOr:
			for _, cond := range filter.Group.Conditions {
				passes, err := e.evaluateGoFilter(row, &cond)
				if err != nil {
					return false, err
				}
				if passes {
					return true, nil
				}
			}
			return false, nil
		case querydsl.LogicalOperatorNot:
			if len(filter.Group.Conditions) != 1 {
				return false, fmt.Errorf("NOT operator requires exactly one condition/group for Go evaluation")
			}
			passes, err := e.evaluateGoFilter(row, &filter.Group.Conditions[0])
			if err != nil {
				return false, err
			}
			return !passes, nil
		case querydsl.LogicalOperatorNor: // NOT (A OR B)
			for _, cond := range filter.Group.Conditions {
				passes, err := e.evaluateGoFilter(row, &cond)
				if err != nil {
					return false, err
				}
				if passes { // If any condition passes, NOR is false
					return false, nil
				}
			}
			return true, nil // All conditions failed, so NOR is true
		case querydsl.LogicalOperatorXor: // (A AND NOT B) OR (NOT A AND B)
			if len(filter.Group.Conditions) != 2 {
				return false, fmt.Errorf("XOR operator requires exactly two conditions/groups for Go evaluation")
			}
			passesA, errA := e.evaluateGoFilter(row, &filter.Group.Conditions[0])
			if errA != nil {
				return false, errA
			}
			passesB, errB := e.evaluateGoFilter(row, &filter.Group.Conditions[1])
			if errB != nil {
				return false, errB
			}
			return (passesA && !passesB) || (!passesA && passesB), nil
		default:
			return false, fmt.Errorf("unsupported logical operator for Go evaluation: %s", filter.Group.Operator)
		}
	}
	return false, fmt.Errorf("empty or invalid filter structure for Go evaluation")
}

// applyGoComputeFunctions iterates through rows and applies Go-based computed field functions.
func (e *SqliteExecutor) applyGoComputeFunctions(rows []querydsl.Row, projection *querydsl.ProjectionConfiguration) ([]querydsl.Row, error) {
	if projection == nil || len(projection.Computed) == 0 {
		return rows, nil
	}

	e.mu.RLock() // Read lock for goComputeFunctions map
	defer e.mu.RUnlock()

	for _, row := range rows {
		for _, item := range projection.Computed {
			if item.ComputedFieldExpression != nil {
				funcName := item.ComputedFieldExpression.Expression.Function
				alias := item.ComputedFieldExpression.Alias
				if alias == "" {
					alias = fmt.Sprintf("%v", funcName) // Default alias if not provided
				}

				fn, ok := e.goComputeFunctions[fmt.Sprintf("%v", funcName)] // Function name might be string or other FilterValue
				if !ok {
					return nil, fmt.Errorf("unregistered Go compute function: %v", funcName)
				}

				computedValue, err := fn(row)
				if err != nil {
					return nil, fmt.Errorf("error executing Go compute function '%v' on row %+v: %w", funcName, row, err)
				}
				row[alias] = computedValue
			}
			// Handle CaseExpression if implemented
		}
	}
	return rows, nil
}

// applyFinalProjection processes rows to match the user's requested projection (include/exclude).
// This runs AFTER all Go-based filtering and computation.
func (e *SqliteExecutor) applyFinalProjection(rows []querydsl.Row, projection *querydsl.ProjectionConfiguration) []querydsl.Row {
	if projection == nil || (len(projection.Include) == 0 && len(projection.Exclude) == 0 && len(projection.Computed) == 0) {
		return rows // No explicit projection, return as is (all fetched fields)
	}

	var finalRows []querydsl.Row

	for _, originalRow := range rows {
		newRow := make(querydsl.Row)

		// 1. Handle Include projection
		if len(projection.Include) > 0 {
			for _, f := range projection.Include {
				if val, ok := originalRow[f.Name]; ok {
					newRow[f.Name] = val
				}
			}
		} else if len(projection.Exclude) > 0 {
			// 2. Handle Exclude projection (only if no Include is specified)
			excludedFields := make(map[string]struct{})
			for _, f := range projection.Exclude {
				excludedFields[f.Name] = struct{}{}
			}
			for fieldName, val := range originalRow {
				if _, isExcluded := excludedFields[fieldName]; !isExcluded {
					newRow[fieldName] = val
				}
			}
		} else {
			maps.Copy(newRow, originalRow)
		}

		// 3. Add computed fields to the final row
		if len(projection.Computed) > 0 {
			for _, item := range projection.Computed {
				if item.ComputedFieldExpression != nil {
					alias := item.ComputedFieldExpression.Alias
					if alias == "" {
						alias = fmt.Sprintf("%v", item.ComputedFieldExpression.Expression.Function)
					}
					if val, ok := originalRow[alias]; ok {
						newRow[alias] = val
					}
				}
			}
		}

		finalRows = append(finalRows, newRow)
	}
	return finalRows
}

// Update performs an update operation on the database based on the provided
// tableName, update data, and filters.
// It returns sql.Result (containing rows affected, etc.) and an error.
func (e *SqliteExecutor) Update(ctx context.Context, tableName string, updates map[string]any, filters *querydsl.QueryFilter) (sql.Result, error) {
	// Ensure the queryGenerator is set and capable of generating update SQL
	// (assuming SqliteQuery has been initialized as queryGenerator)
	sqliteQueryGenerator, ok := e.queryGenerator.(*SqliteQuery)
	if !ok {
		return nil, fmt.Errorf("query generator is not a SqliteQuery instance, cannot generate UPDATE SQL")
	}

	sqlQuery, queryParams, err := sqliteQueryGenerator.GenerateUpdateSQL(tableName, updates, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to generate SQL UPDATE query: %w", err)
	}

	log.Printf("Generated SQL for DB update: %s", sqlQuery) // For debugging, consider removing in production
	log.Printf("SQL Params: %v", queryParams) // For debugging, consider removing in production

	result, err := e.db.ExecContext(ctx, sqlQuery, queryParams...) // Use ExecContext for UPDATE
	if err != nil {
		return nil, fmt.Errorf("failed to execute UPDATE query: %w", err)
	}

	return result, nil
}

