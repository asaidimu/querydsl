package core

// Add a helper function to core or as a method on ComparisonOperator
// to distinguish standard vs. custom operators.
// For this example, let's just make a simple map for demonstration.
var standardComparisonOperators = map[ComparisonOperator]struct{}{
	ComparisonOperatorEq:         {},
	ComparisonOperatorNeq:        {},
	ComparisonOperatorLt:         {},
	ComparisonOperatorLte:        {},
	ComparisonOperatorGt:         {},
	ComparisonOperatorGte:        {},
	ComparisonOperatorIn:         {},
	ComparisonOperatorNin:        {},
	ComparisonOperatorContains:   {},
	ComparisonOperatorNContains:  {},
	ComparisonOperatorStartsWith: {},
	ComparisonOperatorEndsWith:   {},
	ComparisonOperatorExists:     {},
	ComparisonOperatorNExists:    {},
}

func (c ComparisonOperator) IsStandard() bool {
	_, ok := standardComparisonOperators[c]
	return ok
}

// GetStandardComparisonOperators returns a map of all standard comparison operators.
// This might be useful for external checks or initializations.
func GetStandardComparisonOperators() map[ComparisonOperator]struct{} {
	return standardComparisonOperators
}
