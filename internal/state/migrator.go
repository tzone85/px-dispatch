// Package state provides the migration system for initializing and upgrading
// the SQLite projection database schema.
package state

import (
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/tzone85/px-dispatch/migrations"
)

// migrationFile pairs a version number with its filename for sorting.
type migrationFile struct {
	version  int
	filename string
}

// RunMigrations applies all unapplied SQL migrations to the given database.
// It creates a schema_migrations tracking table, reads embedded .sql files,
// and executes them in version order. Returns the count of newly applied
// migrations. This is forward-only; rollbacks are not supported.
func RunMigrations(db *sql.DB) (int, error) {
	if err := ensureMigrationsTable(db); err != nil {
		return 0, err
	}

	applied, err := loadAppliedVersions(db)
	if err != nil {
		return 0, fmt.Errorf("load applied versions: %w", err)
	}

	sorted, err := loadMigrationFiles()
	if err != nil {
		return 0, fmt.Errorf("load migration files: %w", err)
	}

	count := 0
	for _, mf := range sorted {
		if applied[mf.version] {
			continue
		}

		if err := applyMigration(db, mf); err != nil {
			return count, err
		}
		count++
	}

	return count, nil
}

// ensureMigrationsTable creates the schema_migrations tracking table if it
// does not already exist.
func ensureMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}
	return nil
}

// loadAppliedVersions returns a set of version numbers that have already
// been applied to the database.
func loadAppliedVersions(db *sql.DB) (map[int]bool, error) {
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("query schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate versions: %w", err)
	}

	return applied, nil
}

// loadMigrationFiles reads embedded SQL files and returns them sorted by
// version number extracted from the filename prefix (e.g., "001_init.sql" -> 1).
func loadMigrationFiles() ([]migrationFile, error) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var files []migrationFile
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}

		version, err := extractVersion(name)
		if err != nil {
			return nil, fmt.Errorf("parse version from %s: %w", name, err)
		}

		files = append(files, migrationFile{
			version:  version,
			filename: name,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].version < files[j].version
	})

	return files, nil
}

// applyMigration reads and executes a single migration file within a
// transaction, then records the version in schema_migrations.
func applyMigration(db *sql.DB, mf migrationFile) error {
	data, err := fs.ReadFile(migrations.FS, mf.filename)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", mf.filename, err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx for migration %s: %w", mf.filename, err)
	}

	if _, err := tx.Exec(string(data)); err != nil {
		tx.Rollback()
		return fmt.Errorf("execute migration %s: %w", mf.filename, err)
	}

	if _, err := tx.Exec(
		"INSERT INTO schema_migrations (version) VALUES (?)",
		mf.version,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("record migration %s: %w", mf.filename, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", mf.filename, err)
	}

	return nil
}

// extractVersion parses the numeric prefix from a migration filename.
// For example, "001_init.sql" returns 1.
func extractVersion(filename string) (int, error) {
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid migration filename: %s", filename)
	}

	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("non-numeric prefix in %s: %w", filename, err)
	}

	return version, nil
}
