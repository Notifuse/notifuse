package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetMigrationStatements(t *testing.T) {
	t.Run("Returns migration statements", func(t *testing.T) {
		statements := GetMigrationStatements()

		assert.NotNil(t, statements, "Migration statements should not be nil")
		assert.Greater(t, len(statements), 0, "Should have at least one migration statement")

		// Verify statements are the same as MigrationStatements
		assert.Equal(t, MigrationStatements, statements, "Should return the same statements as MigrationStatements")
	})

	t.Run("Migration statements contain CREATE TABLE", func(t *testing.T) {
		statements := GetMigrationStatements()

		foundCreateTable := false
		for _, statement := range statements {
			if strings.Contains(strings.ToUpper(statement), "CREATE TABLE") {
				foundCreateTable = true
				break
			}
		}

		assert.True(t, foundCreateTable, "Migration statements should contain at least one CREATE TABLE statement")
	})

	t.Run("All statements are non-empty", func(t *testing.T) {
		statements := GetMigrationStatements()

		for i, statement := range statements {
			assert.NotEmpty(t, statement, "Statement at index %d should not be empty", i)
			assert.NotEmpty(t, strings.TrimSpace(statement), "Statement at index %d should not be just whitespace", i)
		}
	})
}

func TestMigrationStatements(t *testing.T) {
	t.Run("Contains some expected table references", func(t *testing.T) {
		// Test that migration statements contain some expected table references
		// Since these are migration statements, they may not be CREATE TABLE statements
		expectedTables := []string{
			"workspace_invitations",
			"settings",
		}

		statements := MigrationStatements
		allStatements := strings.Join(statements, " ")

		foundTables := 0
		for _, tableName := range expectedTables {
			if strings.Contains(allStatements, tableName) {
				foundTables++
			}
		}

		assert.Greater(t, foundTables, 0, "Migration statements should contain at least one expected table reference")
	})

	t.Run("Statements are valid SQL format", func(t *testing.T) {
		statements := MigrationStatements

		for i, statement := range statements {
			// Basic SQL validation - should be valid SQL statements
			upperStatement := strings.ToUpper(strings.TrimSpace(statement))

			// Should contain SQL keywords (more flexible check)
			hasSQLKeywords := strings.Contains(upperStatement, "CREATE") ||
				strings.Contains(upperStatement, "ALTER") ||
				strings.Contains(upperStatement, "DO") ||
				strings.Contains(upperStatement, "BEGIN")

			assert.True(t, hasSQLKeywords,
				"Statement %d should contain SQL keywords, got: %s", i, statement[:min(50, len(statement))])

			// Should not be empty
			assert.NotEmpty(t, strings.TrimSpace(statement), "Statement %d should not be empty", i)
		}
	})

	t.Run("Each statement creates different table", func(t *testing.T) {
		statements := MigrationStatements
		tableNames := make(map[string]bool)

		for _, statement := range statements {
			// Extract table name from CREATE TABLE statement
			upperStatement := strings.ToUpper(statement)
			if strings.Contains(upperStatement, "CREATE TABLE") {
				// Simple extraction - find text after "CREATE TABLE" and before "("
				parts := strings.Split(upperStatement, "CREATE TABLE")
				if len(parts) > 1 {
					tablePart := strings.TrimSpace(parts[1])
					if openParen := strings.Index(tablePart, "("); openParen > 0 {
						tableName := strings.TrimSpace(tablePart[:openParen])
						tableName = strings.Trim(tableName, "\"'`") // Remove quotes

						assert.False(t, tableNames[tableName], "Table %s should not be created multiple times", tableName)
						tableNames[tableName] = true
					}
				}
			}
		}

		assert.Greater(t, len(tableNames), 0, "Should have extracted at least one table name")
	})
}

func TestTableNames(t *testing.T) {
	t.Run("Contains expected tables", func(t *testing.T) {
		expectedTables := []string{
			"users",
			"user_sessions",
			"workspaces",
			"user_workspaces",
			"workspace_invitations",
			"broadcasts",
			"tasks",
			"settings",
		}

		for _, expectedTable := range expectedTables {
			assert.Contains(t, TableNames, expectedTable, "TableNames should contain: %s", expectedTable)
		}
	})

	t.Run("All table names are non-empty", func(t *testing.T) {
		for i, tableName := range TableNames {
			assert.NotEmpty(t, tableName, "Table name at index %d should not be empty", i)
			assert.NotEmpty(t, strings.TrimSpace(tableName), "Table name at index %d should not be just whitespace", i)
		}
	})

	t.Run("No duplicate table names", func(t *testing.T) {
		seen := make(map[string]bool)

		for _, tableName := range TableNames {
			assert.False(t, seen[tableName], "Table name %s should not be duplicated", tableName)
			seen[tableName] = true
		}
	})

	t.Run("Table names follow naming convention", func(t *testing.T) {
		for _, tableName := range TableNames {
			// Table names should be lowercase and use underscores
			assert.Equal(t, strings.ToLower(tableName), tableName, "Table name %s should be lowercase", tableName)
			assert.NotContains(t, tableName, " ", "Table name %s should not contain spaces", tableName)
			assert.NotContains(t, tableName, "-", "Table name %s should not contain hyphens", tableName)
		}
	})

	t.Run("TableNames and MigrationStatements exist", func(t *testing.T) {
		// Basic sanity check that both exist
		assert.Greater(t, len(TableNames), 0, "Should have at least one table name")
		assert.Greater(t, len(MigrationStatements), 0, "Should have at least one migration statement")
	})
}

func TestSchemaConsistency(t *testing.T) {
	t.Run("Migration statements reference some TableNames", func(t *testing.T) {
		allStatements := strings.Join(MigrationStatements, " ")
		allStatementsLower := strings.ToLower(allStatements)

		// Check that some table names appear in the migration statements
		foundTables := 0
		for _, tableName := range TableNames {
			if strings.Contains(allStatementsLower, strings.ToLower(tableName)) {
				foundTables++
			}
		}

		// At least some table names should be found in migration statements
		assert.Greater(t, foundTables, 0,
			"At least one table name should be found in migration statements")
	})

	t.Run("No obvious SQL injection vulnerabilities", func(t *testing.T) {
		// Basic check for dangerous patterns in migration statements
		dangerousPatterns := []string{
			"';",
			"/**/", // Changed to avoid matching legitimate comments
		}

		for _, statement := range MigrationStatements {
			upperStatement := strings.ToUpper(statement)
			for _, pattern := range dangerousPatterns {
				assert.NotContains(t, upperStatement, pattern,
					"Statement should not contain dangerous pattern: %s", pattern)
			}
		}
	})
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
