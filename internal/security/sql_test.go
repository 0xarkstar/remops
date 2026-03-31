package security

import "testing"

func TestIsWriteQuery(t *testing.T) {
	tests := []struct {
		sql   string
		write bool
	}{
		// Read-only
		{"SELECT * FROM users", false},
		{"select id from t", false},
		{"SHOW TABLES", false},
		{"show databases", false},
		{"EXPLAIN SELECT 1", false},
		{"DESCRIBE users", false},
		{"  SELECT 1", false},

		// Write statements
		{"INSERT INTO t VALUES (1)", true},
		{"UPDATE t SET x=1", true},
		{"DELETE FROM t", true},
		{"DROP TABLE t", true},
		{"ALTER TABLE t ADD COLUMN x INT", true},
		{"TRUNCATE TABLE t", true},
		{"CREATE TABLE t (id INT)", true},
		{"GRANT SELECT ON t TO user1", true},
		{"REVOKE SELECT ON t FROM user1", true},
		{"MERGE INTO t USING s ON t.id=s.id", true},

		// CTE — read-only body
		{"WITH cte AS (SELECT 1) SELECT * FROM cte", false},

		// CTE — write body
		{"WITH cte AS (SELECT 1) DELETE FROM t WHERE id IN (SELECT * FROM cte)", true},
		{"WITH cte AS (SELECT 1) INSERT INTO t SELECT * FROM cte", true},

		// Stacked queries — semicolons always treated as write
		{"SELECT 1; DROP TABLE x", true},
		{"select 1; delete from t", true},
		{"SHOW TABLES; INSERT INTO t VALUES (1)", true},

		// Case insensitive
		{"insert into t values (1)", true},
		{"alter table t drop column x", true},
		{"truncate table t", true},
	}

	for _, tt := range tests {
		got := IsWriteQuery(tt.sql)
		if got != tt.write {
			t.Errorf("IsWriteQuery(%q) = %v, want %v", tt.sql, got, tt.write)
		}
	}
}
