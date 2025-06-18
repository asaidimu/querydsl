package sqlite

import (
	"fmt"
	"strings"
	querydsl "github.com/asaidimu/querydsl/pkg/core"
)

type SqliteQuery struct{}

func NewSqliteQuery() *SqliteQuery {
	return &SqliteQuery{}
}

func quoteIdentifier(s string) string {
	escapedS := strings.ReplaceAll(s, `"`, `""`)
	return `"` + escapedS + `"`
}

// Generate implements the QueryGenerator interface for SQLite.
func (s *SqliteQuery) Generate(tableName string, dsl *querydsl.QueryDSL) (string, []any, error) {
	if dsl == nil {
		return "", nil, fmt.Errorf("QueryDSL cannot be nil")
	}
	if tableName == "" {
		return "", nil, fmt.Errorf("table name cannot be empty")
	}

	// ALWAYS quote the table name to prevent injection
	quotedTableName := quoteIdentifier(tableName)

	var selectFields []string
	var whereClauses []string
	var orderByClauses []string
	var queryParams []any
	limit := -1
	offset := 0

	// --- 1. Process Projection (SELECT clause) for database-native fields ---
	if dsl.Projection != nil && len(dsl.Projection.Include) > 0 {
		for _, field := range dsl.Projection.Include {
			if field.Name != "" && field.Nested == nil {
				// ALWAYS quote column names in SELECT
				selectFields = append(selectFields, quoteIdentifier(field.Name))
			}
		}
	}
	if len(selectFields) == 0 {
		selectFields = append(selectFields, "*") // Default to select all for executor to process
	}

	// --- 2. Process Filters (WHERE clause) for database-native conditions ---
	if dsl.Filters != nil {
		whereSql, err := s.buildWhereClause(dsl.Filters, &queryParams)
		if err != nil {
			return "", nil, fmt.Errorf("error building WHERE clause: %w", err)
		}
		if whereSql != "" {
			whereClauses = append(whereClauses, whereSql)
		}
	}

	// --- 3. Process Sort (ORDER BY) ---
	if len(dsl.Sort) > 0 {
		for _, sortCfg := range dsl.Sort {
			// ALWAYS quote column names in ORDER BY
			orderByClauses = append(orderByClauses, fmt.Sprintf("%s %s", quoteIdentifier(sortCfg.Field), strings.ToUpper(string(sortCfg.Direction))))
		}
	}

	// --- 4. Process Pagination (LIMIT, OFFSET) ---
	if dsl.Pagination != nil && dsl.Pagination.Type == "offset" {
		limit = dsl.Pagination.Limit
		if dsl.Pagination.Offset != nil {
			offset = *dsl.Pagination.Offset
		}
	}

	// --- Assemble final SQL string ---
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("SELECT %s FROM %s", strings.Join(selectFields, ", "), quotedTableName))

	if len(whereClauses) > 0 {
		sb.WriteString(" WHERE " + strings.Join(whereClauses, " AND "))
	}

	if len(orderByClauses) > 0 {
		sb.WriteString(" ORDER BY " + strings.Join(orderByClauses, ", "))
	}

	if limit != -1 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", limit))
	}
	if offset > 0 {
		sb.WriteString(fmt.Sprintf(" OFFSET %d", offset))
	}

	return sb.String(), queryParams, nil
}

// buildWhereClause recursively builds the WHERE clause from QueryFilter.
func (s *SqliteQuery) buildWhereClause(filter *querydsl.QueryFilter, queryParams *[]any) (string, error) {
	if filter.Condition != nil {
		if filter.Condition.Operator.IsStandard() {
			return s.buildCondition(filter.Condition, queryParams)
		}
		return "", nil // Skip custom operator conditions in SQL
	}
	if filter.Group != nil {
		var groupClauses []string
		for _, cond := range filter.Group.Conditions {
			clause, err := s.buildWhereClause(&cond, queryParams)
			if err != nil {
				return "", err
			}
			if clause != "" {
				groupClauses = append(groupClauses, clause)
			}
		}

		if len(groupClauses) == 0 {
			return "", nil
		}

		op := strings.ToUpper(string(filter.Group.Operator))
		switch op {
		case "NOT":
			if len(groupClauses) != 1 {
				return "", fmt.Errorf("NOT operator requires exactly one condition/group for SQL translation")
			}
			return fmt.Sprintf("NOT (%s)", groupClauses[0]), nil
		case "AND", "OR":
			return fmt.Sprintf("(%s)", strings.Join(groupClauses, " "+op+" ")), nil
		default:
			return "", fmt.Errorf("unsupported logical operator for direct SQL translation: %s", filter.Group.Operator)
		}
	}
	return "", fmt.Errorf("empty or invalid filter structure")
}

// buildCondition translates a single FilterCondition into SQL.
func (s *SqliteQuery) buildCondition(cond *querydsl.FilterCondition, queryParams *[]any) (string, error) {
	paramPlaceholder := "?"

	// ALWAYS quote the field name to prevent injection
	quotedField := quoteIdentifier(cond.Field)

	switch cond.Operator {
	case querydsl.ComparisonOperatorEq:
		*queryParams = append(*queryParams, cond.Value)
		return fmt.Sprintf("%s = %s", quotedField, paramPlaceholder), nil
	case querydsl.ComparisonOperatorNeq:
		*queryParams = append(*queryParams, cond.Value)
		return fmt.Sprintf("%s != %s", quotedField, paramPlaceholder), nil
	case querydsl.ComparisonOperatorLt:
		*queryParams = append(*queryParams, cond.Value)
		return fmt.Sprintf("%s < %s", quotedField, paramPlaceholder), nil
	case querydsl.ComparisonOperatorLte:
		*queryParams = append(*queryParams, cond.Value)
		return fmt.Sprintf("%s <= %s", quotedField, paramPlaceholder), nil
	case querydsl.ComparisonOperatorGt:
		*queryParams = append(*queryParams, cond.Value)
		return fmt.Sprintf("%s > %s", quotedField, paramPlaceholder), nil
	case querydsl.ComparisonOperatorGte:
		*queryParams = append(*queryParams, cond.Value)
		return fmt.Sprintf("%s >= %s", quotedField, paramPlaceholder), nil
	case querydsl.ComparisonOperatorIn:
		if vals, ok := cond.Value.([]any); ok && len(vals) > 0 {
			placeholders := make([]string, len(vals))
			for i := range vals {
				placeholders[i] = "?"
				*queryParams = append(*queryParams, vals[i])
			}
			return fmt.Sprintf("%s IN (%s)", quotedField, strings.Join(placeholders, ",")), nil
		}
		return "", fmt.Errorf("IN operator requires a non-empty array value")
	case querydsl.ComparisonOperatorNin:
		if vals, ok := cond.Value.([]any); ok && len(vals) > 0 {
			placeholders := make([]string, len(vals))
			for i := range vals {
				placeholders[i] = "?"
				*queryParams = append(*queryParams, vals[i])
			}
			return fmt.Sprintf("%s NOT IN (%s)", quotedField, strings.Join(placeholders, ",")), nil
		}
		return "", fmt.Errorf("NIN operator requires a non-empty array value")
	case querydsl.ComparisonOperatorContains:
		*queryParams = append(*queryParams, "%"+fmt.Sprintf("%v", cond.Value)+"%")
		return fmt.Sprintf("%s LIKE %s", quotedField, paramPlaceholder), nil
	case querydsl.ComparisonOperatorNContains:
		*queryParams = append(*queryParams, "%"+fmt.Sprintf("%v", cond.Value)+"%")
		return fmt.Sprintf("%s NOT LIKE %s", quotedField, paramPlaceholder), nil
	case querydsl.ComparisonOperatorStartsWith:
		*queryParams = append(*queryParams, fmt.Sprintf("%v", cond.Value)+"%")
		return fmt.Sprintf("%s LIKE %s", quotedField, paramPlaceholder), nil
	case querydsl.ComparisonOperatorEndsWith:
		*queryParams = append(*queryParams, "%"+fmt.Sprintf("%v", cond.Value))
		return fmt.Sprintf("%s LIKE %s", quotedField, paramPlaceholder), nil
	case querydsl.ComparisonOperatorExists:
		return fmt.Sprintf("%s IS NOT NULL", quotedField), nil
	case querydsl.ComparisonOperatorNExists:
		return fmt.Sprintf("%s IS NULL", quotedField), nil
	default:
		return "", fmt.Errorf("unsupported comparison operator for direct SQL: %s", cond.Operator)
	}
}
