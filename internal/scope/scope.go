package scope

// QueryScope represents a SQL condition that can be added to a query.
// It carries a raw SQL predicate and its arguments for safe parameter binding.
type QueryScope struct {
	// Condition is the SQL WHERE clause condition (e.g., "tenant_id = ?")
	Condition string
	// Args contains the parameter values for placeholders in Condition
	Args []interface{}
}
