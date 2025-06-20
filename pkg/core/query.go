package core // Assuming it's in the core package based on the import path

// QueryGenerator defines the interface for generating database-specific queries
// from a generic QueryDSL object.
type QueryGenerator interface {
    // GenerateSelectSql creates a SQL query string and a slice of query parameters
    // for a given table name and QueryDSL object.
    GenerateSelectSQL(tableName string, dsl *QueryDSL) (string, []any, error)

    // GenerateUpdateSQL creates a complete SQL UPDATE query string and its corresponding
    // parameters from a map of updates and a QueryFilter for the WHERE clause.
	GenerateUpdateSQL(tableName string, updates map[string]any, filters *QueryFilter) (string, []any, error)
}
