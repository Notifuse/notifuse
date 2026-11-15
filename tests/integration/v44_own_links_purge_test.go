package integration

import (
	"context"
	"database/sql"
	"sort"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestV44MigrationRemovesTheInstanceOwnLinks runs the compiled V44 workspace update on a
// real workspace database: the clicked_links keys on the API endpoint's host and on the
// workspace's custom domain go, on any port, in any case, with one trailing dot; the other
// keys, an unreadable key and clicked_at stay; no email.link_clicked entry is written; a
// second run changes nothing.
func TestV44MigrationRemovesTheInstanceOwnLinks(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	ctx := context.Background()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	endpoint := "https://Links.Brand.example"
	workspace.Settings.CustomEndpointURL = &endpoint

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	mixed, err := factory.CreateMessageHistory(workspace.ID, testutil.WithMessageTemplate(template.ID))
	require.NoError(t, err)
	onlyOwn, err := factory.CreateMessageHistory(workspace.ID, testutil.WithMessageTemplate(template.ID))
	require.NoError(t, err)
	untouched, err := factory.CreateMessageHistory(workspace.ID, testutil.WithMessageTemplate(template.ID))
	require.NoError(t, err)

	link := `{"count": 1, "verified": true}`
	seed := func(id, links string) {
		t.Helper()
		_, err := db.ExecContext(ctx, `UPDATE message_history SET clicked_links = $2::jsonb, clicked_at = '2026-01-02T03:04:05Z' WHERE id = $1`, id, links)
		require.NoError(t, err)
	}
	seed(mixed.ID, `{
		"https://api.notifuse.test/unsubscribe-oneclick?email=a@b.c": `+link+`,
		"https://API.NOTIFUSE.TEST:8443/notification-center": `+link+`,
		"https://api.notifuse.test./x": `+link+`,
		"http://links.brand.example/blog/post": `+link+`,
		"HTTPS://user@links.brand.example/u": `+link+`,
		"https://shop.example.com/sale": `+link+`,
		"https://api.notifuse.test.evil/x": `+link+`,
		"https:/x": `+link+`
	}`)
	seed(onlyOwn.ID, `{"https://links.brand.example/notification-center": `+link+`}`)
	seed(untouched.ID, `{"https://shop.example.com/a": `+link+`}`)

	countEntries := func() int {
		var n int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM contact_timeline WHERE kind = 'email.link_clicked'`).Scan(&n))
		return n
	}
	entriesBefore := countEntries()
	var untouchedUpdatedAt string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT updated_at::text FROM message_history WHERE id = $1`, untouched.ID).Scan(&untouchedUpdatedAt))

	cfg := &config.Config{APIEndpoint: "https://api.notifuse.test"}
	run := func() {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		if err := (&migrations.V44Migration{}).UpdateWorkspace(ctx, cfg, workspace, tx); err != nil {
			_ = tx.Rollback()
			require.NoError(t, err)
		}
		require.NoError(t, tx.Commit())
	}
	links := func(id string) (map[string]bool, sql.NullString) {
		t.Helper()
		var keys []string
		rows, err := db.QueryContext(ctx, `SELECT jsonb_object_keys(clicked_links) FROM message_history WHERE id = $1`, id)
		require.NoError(t, err)
		defer rows.Close()
		for rows.Next() {
			var k string
			require.NoError(t, rows.Scan(&k))
			keys = append(keys, k)
		}
		require.NoError(t, rows.Err())
		got := map[string]bool{}
		for _, k := range keys {
			got[k] = true
		}
		var clickedAt sql.NullString
		require.NoError(t, db.QueryRowContext(ctx, `SELECT clicked_at::text FROM message_history WHERE id = $1`, id).Scan(&clickedAt))
		return got, clickedAt
	}

	run()

	got, clickedAt := links(mixed.ID)
	assert.Equal(t, map[string]bool{
		"https://shop.example.com/sale":    true,
		"https://api.notifuse.test.evil/x": true,
		"https:/x":                         true,
	}, got, "the own keys go, the others stay")
	assert.True(t, clickedAt.Valid, "clicked_at stays")

	got, clickedAt = links(onlyOwn.ID)
	assert.Empty(t, got)
	assert.True(t, clickedAt.Valid)
	var onlyOwnLinks string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT clicked_links::text FROM message_history WHERE id = $1`, onlyOwn.ID).Scan(&onlyOwnLinks))
	assert.Equal(t, "{}", onlyOwnLinks)

	got, _ = links(untouched.ID)
	assert.Equal(t, map[string]bool{"https://shop.example.com/a": true}, got)
	var updatedAt string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT updated_at::text FROM message_history WHERE id = $1`, untouched.ID).Scan(&updatedAt))
	assert.Equal(t, untouchedUpdatedAt, updatedAt, "a row without own links is not written")

	assert.Equal(t, entriesBefore, countEntries(), "the purge writes no email.link_clicked entry")

	// A second run finds nothing to remove.
	var before string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT string_agg(id || clicked_links::text || updated_at::text, ',' ORDER BY id) FROM message_history`).Scan(&before))
	run()
	var after string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT string_agg(id || clicked_links::text || updated_at::text, ',' ORDER BY id) FROM message_history`).Scan(&after))
	assert.Equal(t, before, after)
}

// TestV44MigrationPurgeEdgeCases pins what the purge leaves alone and what it writes: a key
// on another domain that only a misread host would match, also with standard_conforming_strings
// off, an IPv6 key, two trailing dots, clicked_links values that are not objects, and the
// updated_at of a purged row.
func TestV44MigrationPurgeEdgeCases(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	ctx := context.Background()
	link := `{"count": 1, "verified": true}`

	for _, conforming := range []bool{true, false} {
		name := "standard_conforming_strings on"
		if !conforming {
			name = "standard_conforming_strings off"
		}
		t.Run(name, func(t *testing.T) {
			workspace, err := factory.CreateWorkspace()
			require.NoError(t, err)
			db, err := factory.GetWorkspaceDB(workspace.ID)
			require.NoError(t, err)
			template, err := factory.CreateTemplate(workspace.ID)
			require.NoError(t, err)

			seed := func(links string) string {
				t.Helper()
				m, err := factory.CreateMessageHistory(workspace.ID, testutil.WithMessageTemplate(template.ID))
				require.NoError(t, err)
				_, err = db.ExecContext(ctx, `UPDATE message_history SET clicked_links = $2::jsonb, updated_at = '2026-01-02T03:04:05Z' WHERE id = $1`, m.ID, links)
				require.NoError(t, err)
				return m.ID
			}
			mixed := seed(`{
				"https://brand.co/unsubscribe?email=a@b.c": ` + link + `,
				"https://brand.com/x": ` + link + `,
				"https://brand.c/x": ` + link + `,
				"https://brand.co../x": ` + link + `,
				"https://[2001:db8::1]:8443/notification-center": ` + link + `,
				"https://[2001:db8::2]/x": ` + link + `
			}`)
			notObjects := map[string]string{}
			for _, v := range []string{`["https://brand.co/x"]`, `"https://brand.co/x"`, `null`, `42`} {
				notObjects[seed(v)] = v
			}

			var mixedUpdatedAt string
			require.NoError(t, db.QueryRowContext(ctx, `SELECT updated_at::text FROM message_history WHERE id = $1`, mixed).Scan(&mixedUpdatedAt))

			cfg := &config.Config{APIEndpoint: "https://brand.co"}
			endpoint := "https://[2001:db8::1]:8443"
			workspace.Settings.CustomEndpointURL = &endpoint

			tx, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			if !conforming {
				_, err = tx.ExecContext(ctx, `SET LOCAL standard_conforming_strings = off`)
				require.NoError(t, err)
			}
			if err := (&migrations.V44Migration{}).UpdateWorkspace(ctx, cfg, workspace, tx); err != nil {
				_ = tx.Rollback()
				require.NoError(t, err)
			}
			require.NoError(t, tx.Commit())

			var keys []string
			rows, err := db.QueryContext(ctx, `SELECT jsonb_object_keys(clicked_links) FROM message_history WHERE id = $1`, mixed)
			require.NoError(t, err)
			for rows.Next() {
				var k string
				require.NoError(t, rows.Scan(&k))
				keys = append(keys, k)
			}
			require.NoError(t, rows.Err())
			_ = rows.Close()
			sort.Strings(keys)
			assert.Equal(t, []string{
				"https://[2001:db8::2]/x",
				"https://brand.c/x",
				"https://brand.co../x",
				"https://brand.com/x",
			}, keys, "only the keys on the own hosts go")

			var updatedAt string
			require.NoError(t, db.QueryRowContext(ctx, `SELECT updated_at::text FROM message_history WHERE id = $1`, mixed).Scan(&updatedAt))
			assert.Equal(t, mixedUpdatedAt, updatedAt, "a purged row keeps its updated_at")

			for id, want := range notObjects {
				var got sql.NullString
				require.NoError(t, db.QueryRowContext(ctx, `SELECT clicked_links::text FROM message_history WHERE id = $1`, id).Scan(&got))
				assert.Equal(t, want, got.String, "a value that is not an object is left as it is")
			}
		})
	}
}
