package schema

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebAnalyticsTableDefinitions(t *testing.T) {
	defs := WebAnalyticsTableDefinitions()

	t.Run("idempotent DDL for three partitioned parents and their indexes", func(t *testing.T) {
		joined := strings.Join(defs, "\n")
		for _, table := range WebAnalyticsTableNames {
			assert.Contains(t, joined, "CREATE TABLE IF NOT EXISTS "+table+" (")
		}
		for _, stmt := range defs {
			assert.Contains(t, stmt, "IF NOT EXISTS", "every statement must be idempotent")
		}
		assert.Equal(t, 3, strings.Count(joined, "PARTITION BY RANGE (session_date)"))
		assert.Contains(t, joined, "USING BRIN (created_at)")
		assert.Contains(t, joined, "USING BRIN (entered_at)")
		assert.Contains(t, joined, "USING BRIN (goal_at)")
		assert.Equal(t, 3, strings.Count(joined, "WHERE contact_email IS NOT NULL"), "partial identity index on each table")
	})

	t.Run("primary keys embed the partition key and the writer", func(t *testing.T) {
		joined := strings.Join(defs, "\n")
		assert.Contains(t, joined, "PRIMARY KEY (session_date, id)")
		// tab_id makes each browser tab a disjoint writer under one shared
		// session_id. Without it two tabs both number their pages from 1 and
		// overwrite each other's rows, arbitrated only by a beat_seq race
		// between two independent per-tab counters.
		assert.Contains(t, joined, "PRIMARY KEY (session_date, session_id, tab_id, page_number)")
		assert.Contains(t, joined, "PRIMARY KEY (session_date, session_id, tab_id, goal_name, client_ts_ms)")
	})

	t.Run("tab_id is a defaulted BIGINT on both child tables", func(t *testing.T) {
		// BIGINT rather than a UUID: web_pages is the highest-volume partitioned
		// table and tab_id only has to be unique among one session's tabs, so 16
		// bytes per row plus a wider PK index would be paid for nothing.
		// DEFAULT 0 is the migration property — a payload from an SDK that does
		// not send tab_id lands on writer 0 and behaves exactly as it does today.
		for _, stmt := range defs {
			if strings.Contains(stmt, "CREATE TABLE IF NOT EXISTS web_pages (") ||
				strings.Contains(stmt, "CREATE TABLE IF NOT EXISTS web_goals (") {
				assert.Contains(t, stmt, "tab_id BIGINT NOT NULL DEFAULT 0")
			}
		}
	})

	t.Run("attribution snapshot present on sessions and goals", func(t *testing.T) {
		var sessions, goals string
		for _, stmt := range defs {
			if strings.Contains(stmt, "CREATE TABLE IF NOT EXISTS web_sessions") {
				sessions = stmt
			}
			if strings.Contains(stmt, "CREATE TABLE IF NOT EXISTS web_goals") {
				goals = stmt
			}
		}
		require.NotEmpty(t, sessions)
		require.NotEmpty(t, goals)
		for _, col := range []string{"utm_source", "channel_group", "custom_10", "referrer_domain", "country", "beat_seq", "contact_email"} {
			assert.Contains(t, sessions, col)
			assert.Contains(t, goals, col)
		}
		// contact_email now lives in the shared attribution block, so goal reports
		// can name the converting contact without joining web_sessions.
		assert.Contains(t, sessions, "contact_email")
		assert.Contains(t, goals, "contact_email")
		assert.Contains(t, goals, "properties JSONB")
	})
}

func TestWebAnalyticsPartitionName(t *testing.T) {
	assert.Equal(t, "web_sessions_y2026m08", WebAnalyticsPartitionName("web_sessions", time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)))
	assert.Equal(t, "web_goals_y2025m01", WebAnalyticsPartitionName("web_goals", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, "web_pages_y2026m12", WebAnalyticsPartitionName("web_pages", time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)))
}

func TestParseWebAnalyticsPartitionName(t *testing.T) {
	t.Run("round-trips generated names", func(t *testing.T) {
		month := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
		for _, table := range WebAnalyticsTableNames {
			name := WebAnalyticsPartitionName(table, month)
			gotTable, gotMonth, ok := ParseWebAnalyticsPartitionName(name)
			require.True(t, ok, name)
			assert.Equal(t, table, gotTable)
			assert.Equal(t, month, gotMonth)
		}
	})

	t.Run("rejects foreign and malicious names", func(t *testing.T) {
		for _, name := range []string{
			"contacts",
			"web_sessions",                    // parent, not a partition
			"web_sessions_y2026m13",           // impossible month
			"web_clicks_y2026m01",             // unknown parent
			"web_sessions_y2026m08; DROP x--", // injection attempt
			"WEB_SESSIONS_Y2026M08",
			"",
		} {
			_, _, ok := ParseWebAnalyticsPartitionName(name)
			assert.False(t, ok, name)
		}
	})
}

func TestWebAnalyticsPartitionDDL(t *testing.T) {
	t.Run("mid-year month", func(t *testing.T) {
		ddl := WebAnalyticsPartitionDDL("web_sessions", time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC))
		assert.Equal(t,
			`CREATE TABLE IF NOT EXISTS web_sessions_y2026m08 PARTITION OF web_sessions FOR VALUES FROM ('2026-08-01') TO ('2026-09-01') WITH (fillfactor = 85)`,
			ddl)
	})

	t.Run("december rolls into january of next year", func(t *testing.T) {
		ddl := WebAnalyticsPartitionDDL("web_goals", time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC))
		assert.Contains(t, ddl, "FROM ('2026-12-01') TO ('2027-01-01')")
		assert.Contains(t, ddl, "web_goals_y2026m12")
	})
}
