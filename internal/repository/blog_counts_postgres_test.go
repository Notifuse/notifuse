package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
)

// utcInstant matches a time.Time argument in UTC, optionally equal to a given instant.
type utcInstant struct{ want time.Time }

func (m utcInstant) Match(v driver.Value) bool {
	got, ok := v.(time.Time)
	if !ok || got.Location() != time.UTC {
		return false
	}
	if m.want.IsZero() {
		return time.Since(got) < time.Minute
	}
	return got.Equal(m.want)
}

func newBlogPostRepoWithMock(t *testing.T) (domain.BlogPostRepository, sqlmock.Sqlmock, context.Context) {
	t.Helper()
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace123").Return(db, nil).AnyTimes()
	ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
	return NewBlogPostRepository(workspaceRepo), sqlMock, ctx
}

var blogPostListColumns = []string{"id", "category_id", "slug", "settings", "published_at", "created_at", "updated_at", "deleted_at"}

func TestBlogPostRepository_ListPosts_Scheduled(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

	t.Run("future publication dates, soonest first", func(t *testing.T) {
		repo, sqlMock, ctx := newBlogPostRepoWithMock(t)
		sqlMock.ExpectQuery(`(?s)SELECT COUNT\(\*\).*` + regexp.QuoteMeta(`WHERE deleted_at IS NULL AND published_at IS NOT NULL AND published_at > $1`)).
			WithArgs(utcInstant{now}).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		sqlMock.ExpectQuery(`(?s)SELECT id, category_id.*`+regexp.QuoteMeta(`published_at > $1`)+`.*`+regexp.QuoteMeta(`ORDER BY published_at ASC, id ASC`)+`\s+LIMIT \$2 OFFSET \$3`).
			WithArgs(utcInstant{now}, 10, 20).
			WillReturnRows(sqlmock.NewRows(blogPostListColumns))

		_, err := repo.ListPosts(ctx, domain.ListBlogPostsRequest{Status: domain.BlogPostStatusScheduled, Limit: 10, Offset: 20, Now: now})
		require.NoError(t, err)
		require.NoError(t, sqlMock.ExpectationsWereMet())
	})

	t.Run("a zero instant falls back to the current UTC time", func(t *testing.T) {
		repo, sqlMock, ctx := newBlogPostRepoWithMock(t)
		sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*)`)).WithArgs(utcInstant{}).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT id, category_id`)).WithArgs(utcInstant{}, 10, 0).
			WillReturnRows(sqlmock.NewRows(blogPostListColumns))

		_, err := repo.ListPosts(ctx, domain.ListBlogPostsRequest{Status: domain.BlogPostStatusScheduled, Limit: 10})
		require.NoError(t, err)
		require.NoError(t, sqlMock.ExpectationsWereMet())
	})
}

func TestBlogPostRepository_ListPosts_Search(t *testing.T) {
	repo, sqlMock, ctx := newBlogPostRepoWithMock(t)
	condition := regexp.QuoteMeta(`category_id = $1 AND settings->>'title' ILIKE $2`)
	sqlMock.ExpectQuery(`(?s)SELECT COUNT\(\*\).*`+condition).
		WithArgs("cat-1", `%50\% off\_now\\%`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	sqlMock.ExpectQuery(`(?s)SELECT id, category_id.*`+condition+`.*LIMIT \$3 OFFSET \$4`).
		WithArgs("cat-1", `%50\% off\_now\\%`, 10, 0).
		WillReturnRows(sqlmock.NewRows(blogPostListColumns))

	result, err := repo.ListPosts(ctx, domain.ListBlogPostsRequest{CategoryID: "cat-1", Status: domain.BlogPostStatusAll, Search: `50% off_now\`, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalCount, "total_count follows the search")
	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestBlogPostRepository_CountPosts(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	query := `(?s)SELECT category_id, COUNT\(\*\), COUNT\(\*\), ` +
		regexp.QuoteMeta(`COUNT(*) FILTER (WHERE published_at IS NOT NULL), COUNT(*) FILTER (WHERE published_at > $1), COUNT(*) FILTER (WHERE published_at IS NULL)`) +
		`.*` + regexp.QuoteMeta(`FROM blog_posts WHERE deleted_at IS NULL GROUP BY category_id`)
	countColumns := []string{"category_id", "total", "matched", "published", "scheduled", "draft"}
	rows := func() *sqlmock.Rows {
		// category, total, matching the search, published (scheduled included), scheduled, draft
		return sqlmock.NewRows(countColumns).
			AddRow("cat-1", 5, 5, 3, 1, 2).
			AddRow("cat-2", 2, 2, 2, 0, 0).
			AddRow(nil, 1, 1, 0, 0, 1)
	}

	t.Run("without category: totals over every post", func(t *testing.T) {
		repo, sqlMock, ctx := newBlogPostRepoWithMock(t)
		sqlMock.ExpectQuery(query).WithArgs(utcInstant{now}).WillReturnRows(rows())

		counts, err := repo.CountPosts(ctx, "workspace123", "", "", now)
		require.NoError(t, err)
		assert.Equal(t, &domain.BlogPostCounts{
			AllTotal: 8, Total: 8, Published: 5, Scheduled: 1, Draft: 3,
			ByCategory: map[string]int{"cat-1": 5, "cat-2": 2},
		}, counts)
		require.NoError(t, sqlMock.ExpectationsWereMet())
	})

	t.Run("with a category: its totals, all_total and by_category over every post", func(t *testing.T) {
		repo, sqlMock, ctx := newBlogPostRepoWithMock(t)
		sqlMock.ExpectQuery(query).WithArgs(utcInstant{now}).WillReturnRows(rows())

		counts, err := repo.CountPosts(ctx, "workspace123", "cat-1", "", now)
		require.NoError(t, err)
		assert.Equal(t, &domain.BlogPostCounts{
			AllTotal: 8, Total: 5, Published: 3, Scheduled: 1, Draft: 2,
			ByCategory: map[string]int{"cat-1": 5, "cat-2": 2},
		}, counts)
	})

	t.Run("an empty blog gives zeros and an empty by_category", func(t *testing.T) {
		repo, sqlMock, ctx := newBlogPostRepoWithMock(t)
		sqlMock.ExpectQuery(query).WithArgs(utcInstant{now}).
			WillReturnRows(sqlmock.NewRows(countColumns))

		counts, err := repo.CountPosts(ctx, "workspace123", "cat-9", "", now)
		require.NoError(t, err)
		want := domain.NewBlogPostCounts()
		assert.Equal(t, &want, counts)
	})

	t.Run("a search narrows the counts by status, never all_total nor by_category", func(t *testing.T) {
		repo, sqlMock, ctx := newBlogPostRepoWithMock(t)
		match := regexp.QuoteMeta(`settings->>'title' ILIKE $2`)
		sqlMock.ExpectQuery(`(?s)SELECT category_id, COUNT\(\*\), `+
			regexp.QuoteMeta(`COUNT(*) FILTER (WHERE settings->>'title' ILIKE $2),`)+
			`.*FILTER \(WHERE `+match+` AND published_at IS NOT NULL\)`+
			`.*FILTER \(WHERE `+match+` AND published_at > \$1\)`+
			`.*FILTER \(WHERE `+match+` AND published_at IS NULL\)`).
			// The pattern is escaped as blogPosts.list escapes it.
			WithArgs(utcInstant{now}, `%50\% off%`).
			WillReturnRows(sqlmock.NewRows(countColumns).
				AddRow("cat-1", 5, 2, 1, 0, 1).
				AddRow("cat-2", 2, 0, 0, 0, 0))

		counts, err := repo.CountPosts(ctx, "workspace123", "", "50% off", now)
		require.NoError(t, err)
		assert.Equal(t, &domain.BlogPostCounts{
			AllTotal: 7, Total: 2, Published: 1, Scheduled: 0, Draft: 1,
			ByCategory: map[string]int{"cat-1": 5, "cat-2": 2},
		}, counts)
		require.NoError(t, sqlMock.ExpectationsWereMet())
	})

	t.Run("query error", func(t *testing.T) {
		repo, sqlMock, ctx := newBlogPostRepoWithMock(t)
		sqlMock.ExpectQuery(query).WillReturnError(errors.New("boom"))

		_, err := repo.CountPosts(ctx, "workspace123", "", "", now)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
	})
}
