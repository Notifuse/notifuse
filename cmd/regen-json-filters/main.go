// Command regen-json-filters brings one workspace database already at 44.0 up to the way 44.0
// compiles a value read inside a custom JSON column, and the way its defensive casts test the
// conversion. v44 does it for a database migrated to 44 after this change; one migrated before it
// (a preview or demo instance that ran an earlier 44.0 build) never runs v44 again.
//
// Deploy the fixed code first: an instance still running the old code compiles the old form again
// whenever an automation is activated or a segment saved. Then, for each workspace database:
//
//	go run ./cmd/regen-json-filters -dsn 'postgres://user:pass@host:5432/notifuse_ws_xxx?sslmode=disable'
//
// In one transaction, under a 5-second lock_timeout, it recompiles every segment's query, storing
// and marking for a rebuild those that changed (the recompute check rebuilds them within minutes),
// then replaces the function body of every live automation's trigger. It prints what it changed
// and what it left as it was, with why. A rerun changes nothing.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/Notifuse/notifuse/internal/migrations"
	_ "github.com/lib/pq"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, func(dsn string) (*sql.DB, error) {
		return sql.Open("postgres", dsn)
	}))
}

// run parses args, opens the database with open and returns the exit code: 0 done, 1 failed (the
// transaction is rolled back), 2 a usage error.
func run(args []string, out, errOut io.Writer, open func(dsn string) (*sql.DB, error)) int {
	flags := flag.NewFlagSet("regen-json-filters", flag.ContinueOnError)
	flags.SetOutput(errOut)
	dsn := flags.String("dsn", "", "connection string of one workspace database (notifuse_ws_...)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *dsn == "" {
		fmt.Fprintln(errOut, "regen-json-filters: -dsn is required: the connection string of one workspace database")
		flags.Usage()
		return 2
	}

	db, err := open(*dsn)
	if err != nil {
		fmt.Fprintf(errOut, "regen-json-filters: cannot open the database: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := regenerate(context.Background(), db, out); err != nil {
		fmt.Fprintf(errOut, "regen-json-filters: %v (nothing was changed)\n", err)
		return 1
	}
	return 0
}

func regenerate(ctx context.Context, db *sql.DB, out io.Writer) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot begin a transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		return fmt.Errorf("cannot set the lock timeout: %w", err)
	}
	// The bodies the current generator builds call the five-argument automation_enroll_contact,
	// which v44 installs. On a database below 44 they would install without complaint (a function
	// body's callees are not checked) and then abort every write to contact_timeline.
	var at44 bool
	if err := tx.QueryRowContext(ctx,
		`SELECT to_regprocedure('automation_enroll_contact(varchar,varchar,varchar,varchar,jsonb)') IS NOT NULL`).Scan(&at44); err != nil {
		return fmt.Errorf("cannot check the database version: %w", err)
	}
	if !at44 {
		return fmt.Errorf("this database is not at 44.0 yet: start Notifuse 44.0 on it, which migrates it, instead of running this command")
	}
	segments, err := migrations.RegenerateSegmentSQL(ctx, tx)
	if err != nil {
		return fmt.Errorf("segments: %w", err)
	}
	automations, err := migrations.RegenerateAutomationTriggerBodies(ctx, tx)
	if err != nil {
		return fmt.Errorf("automations: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit: %w", err)
	}

	fmt.Fprintf(out, "Segments recompiled and marked for a rebuild: %d\n", segments.Recompiled)
	printSkipped(out, "Segments left with their query", segments.Skipped)
	fmt.Fprintf(out, "Live automation triggers replaced: %d\n", automations.Replaced)
	printSkipped(out, "Automations left with their trigger", automations.Skipped)
	return nil
}

func printSkipped(out io.Writer, title string, skipped map[string]string) {
	if len(skipped) == 0 {
		return
	}
	ids := make([]string, 0, len(skipped))
	for id := range skipped {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	fmt.Fprintf(out, "%s: %d\n", title, len(ids))
	for _, id := range ids {
		fmt.Fprintf(out, "  %s: %s\n", id, skipped[id])
	}
}
