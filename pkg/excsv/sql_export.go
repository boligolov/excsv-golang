package excsv

// EffectiveDialect resolves the dialect for a #$ line (exported for CLI).
func EffectiveDialect(stmt SQLStatement, headerSQLDialect string) string {
	return effectiveDialect(stmt, headerSQLDialect)
}
