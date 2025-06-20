package core // Assuming it's in the core package based on the import path

// QueryGenerator defines the interface for generating database-specific queries
// from a generic QueryDSL object.
type QueryGenerator interface {
    // Generate creates a SQL query string and a slice of query parameters
    // for a given table name and QueryDSL object.
    Generate(tableName string, dsl *QueryDSL) (string, []any, error)
}
