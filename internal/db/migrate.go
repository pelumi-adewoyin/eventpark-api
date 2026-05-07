package db

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunMigrations applies any SQL files in migrationsFS that have not yet been
// recorded in the schema_migrations table. Files are applied in lexicographic
// order (001_..., 002_..., etc.) inside individual transactions so a failure
// rolls back only the offending migration.
//
// Bootstrap safety: if schema_migrations has no entries but the database
// already contains tables from a prior manual setup (pre-migration-runner),
// we detect which migrations are already baked in and mark them as applied
// without re-running them. This prevents "type already exists" errors on the
// first deploy of the runner against a live database.
func RunMigrations(pool *pgxpool.Pool, migrationsFS fs.FS) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 1. Ensure the tracking table exists
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// 2. Load already-applied filenames
	rows, err := pool.Query(ctx, `SELECT filename FROM schema_migrations ORDER BY filename`)
	if err != nil {
		return fmt.Errorf("query schema_migrations: %w", err)
	}
	applied := map[string]bool{}
	for rows.Next() {
		var f string
		_ = rows.Scan(&f)
		applied[f] = true
	}
	rows.Close()

	// 3. Read and sort migration files
	entries, err := fs.ReadDir(migrationsFS, ".")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	// 4. Bootstrap: if tracking table is empty, detect pre-existing schema and
	//    mark already-applied migrations so we don't try to re-run them.
	if len(applied) == 0 {
		if err := seedExistingMigrations(ctx, pool, files); err != nil {
			return fmt.Errorf("seed existing migrations: %w", err)
		}
		// Re-load applied set after seeding
		rows2, _ := pool.Query(ctx, `SELECT filename FROM schema_migrations ORDER BY filename`)
		if rows2 != nil {
			for rows2.Next() {
				var f string
				_ = rows2.Scan(&f)
				applied[f] = true
			}
			rows2.Close()
		}
	}

	// 5. Apply each pending migration
	for _, filename := range files {
		if applied[filename] {
			log.Printf("migration already applied: %s", filename)
			continue
		}

		content, err := fs.ReadFile(migrationsFS, filename)
		if err != nil {
			return fmt.Errorf("read %s: %w", filename, err)
		}

		log.Printf("applying migration: %s", filename)

		// Run inside a transaction
		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", filename, err)
		}

		if _, err := tx.Exec(ctx, string(content)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("execute %s: %w", filename, err)
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (filename) VALUES ($1)`, filename,
		); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record %s: %w", filename, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s: %w", filename, err)
		}

		log.Printf("migration applied OK: %s", filename)
	}

	log.Printf("all migrations up to date (%d files checked)", len(files))
	return nil
}

// seedExistingMigrations detects which migrations are already baked into the
// database (from a manual setup before the migration runner existed) and
// records them in schema_migrations so they are not re-executed.
//
// Detection is done by checking for sentinel tables/types that each migration
// uniquely introduces. This is a one-time bootstrap; once schema_migrations
// has entries this function is never called again.
func seedExistingMigrations(ctx context.Context, pool *pgxpool.Pool, files []string) error {
	// tableExists checks whether a table is present in the public schema.
	tableExists := func(table string) bool {
		var exists bool
		_ = pool.QueryRow(ctx,
			`SELECT EXISTS(
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)`, table,
		).Scan(&exists)
		return exists
	}

	// Sentinel table for each migration file (first unique table it creates).
	sentinels := map[string]string{
		"001_init.sql":             "users",
		"002_phase2_gaps.sql":      "event_budget_lines",
		"003_corporate_phase4.sql": "departments",
		"004_budgets.sql":          "personal_budgets",
	}

	for _, filename := range files {
		sentinel, ok := sentinels[filename]
		if !ok {
			continue // no sentinel defined — will be applied normally
		}
		if tableExists(sentinel) {
			log.Printf("bootstrap: %s already applied (table %q exists) — marking as done", filename, sentinel)
			_, _ = pool.Exec(ctx,
				`INSERT INTO schema_migrations (filename) VALUES ($1) ON CONFLICT DO NOTHING`,
				filename,
			)
		}
	}
	return nil
}
