package security

import (
	"strings"
)

// writeKeywords are SQL keywords that indicate a write/mutating query.
var writeKeywords = []string{
	"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "TRUNCATE",
	"CREATE", "GRANT", "REVOKE", "MERGE",
}

// IsWriteQuery returns true if the SQL appears to be a write (mutating) statement.
// Read-only prefixes (SELECT, SHOW, EXPLAIN, DESCRIBE) return false.
// WITH (CTE) queries are scanned for write keywords in the body.
// ContainsSemicolon returns true if the SQL contains a semicolon, indicating
// stacked queries. Stacked queries allow write operations to be hidden after
// a SELECT (e.g. "SELECT 1; DROP TABLE x").
func ContainsSemicolon(sql string) bool {
	return strings.Contains(sql, ";")
}

func IsWriteQuery(sql string) bool {
	upper := strings.ToUpper(strings.TrimSpace(sql))

	// Stacked queries are always treated as writes.
	if strings.Contains(upper, ";") {
		return true
	}

	// Read-only statements.
	for _, prefix := range []string{"SELECT", "SHOW", "EXPLAIN", "DESCRIBE"} {
		if strings.HasPrefix(upper, prefix) {
			return false
		}
	}

	// CTE: WITH ... <body> — scan body for write keywords.
	if strings.HasPrefix(upper, "WITH") {
		for _, kw := range writeKeywords {
			if strings.Contains(upper, kw) {
				return true
			}
		}
		return false
	}

	// Anything else that starts with a write keyword is a write.
	for _, kw := range writeKeywords {
		if strings.HasPrefix(upper, kw) {
			return true
		}
	}

	return true // Unknown — err on the side of caution.
}
