package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupAnnotationTest wires the repository to a sqlmock connection and records
// every statement it executes. The recording matters because the interesting
// assertions here are about SQL *text* — the ON CONFLICT arbiter predicate, and
// the columns Update deliberately leaves out — and Go's regexp has no negative
// lookahead to express the latter as an ExpectExec pattern.
func setupAnnotationTest(t *testing.T, seen *[]string) (*mocks.MockWorkspaceRepository, *annotationRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(
		func(expectedSQL, actualSQL string) error {
			*seen = append(*seen, actualSQL)
			return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
		})))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := NewAnnotationRepository(mockWorkspaceRepo)

	return mockWorkspaceRepo, repo.(*annotationRepository), mock, db
}

func annotationRows() *sqlmock.Rows {
	return sqlmock.NewRows(annotationColumns)
}

// normalizeSQL collapses the whitespace of a heredoc query so a test can assert
// on a clause without reproducing the source file's indentation.
func normalizeSQL(query string) string {
	return strings.Join(strings.Fields(query), " ")
}

func testAnnotation() *domain.Annotation {
	return &domain.Annotation{
		ID:          "ann123",
		AnnotatedAt: time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC),
		Timezone:    "Asia/Tokyo",
		Title:       "Product launch",
		Description: "v2 shipped",
		Color:       domain.AnnotationDefaultColor,
		Source:      domain.AnnotationSourceManual,
	}
}

func TestAnnotationRepository_List_NoFilter(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)

	mock.ExpectQuery("SELECT").WillReturnRows(annotationRows())

	annotations, err := repo.List(ctx, "ws1", domain.AnnotationFilter{})
	require.NoError(t, err)
	// Non-nil so the list endpoint serialises [] rather than null.
	require.NotNil(t, annotations)
	assert.Empty(t, annotations)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, executed, 1)
	assert.NotContains(t, executed[0], "WHERE", "an unset filter must emit no predicate at all")
	assert.Contains(t, executed[0], "FROM annotations")
}

func TestAnnotationRepository_List_DateRange(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 31, 23, 59, 59, 0, time.UTC)

	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectQuery("SELECT").
		WithArgs(start, end).
		WillReturnRows(annotationRows())

	_, err := repo.List(ctx, "ws1", domain.AnnotationFilter{Start: &start, End: &end})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, executed, 1)
	assert.Contains(t, executed[0], "annotated_at >= $1")
	assert.Contains(t, executed[0], "annotated_at <= $2")
}

func TestAnnotationRepository_List_DateRange_StartOnly(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectQuery("SELECT").
		WithArgs(start).
		WillReturnRows(annotationRows())

	_, err := repo.List(ctx, "ws1", domain.AnnotationFilter{Start: &start})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, executed, 1)
	assert.Contains(t, executed[0], "annotated_at >= $1")
	assert.NotContains(t, executed[0], "annotated_at <=")
}

func TestAnnotationRepository_List_SourceFilter(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectQuery("SELECT").
		WithArgs(domain.AnnotationSourceManual, domain.AnnotationSourceBroadcast).
		WillReturnRows(annotationRows())

	_, err := repo.List(ctx, "ws1", domain.AnnotationFilter{
		Sources: []string{domain.AnnotationSourceManual, domain.AnnotationSourceBroadcast},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	// A slice through sq.Eq becomes an IN list, not an equality against an array.
	require.Len(t, executed, 1)
	assert.Contains(t, executed[0], "source IN ($1,$2)")
}

func TestAnnotationRepository_List_DefaultsLimitWhenUnset(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil).Times(2)
	mock.ExpectQuery("SELECT").WillReturnRows(annotationRows())
	mock.ExpectQuery("SELECT").WillReturnRows(annotationRows())

	_, err := repo.List(ctx, "ws1", domain.AnnotationFilter{})
	require.NoError(t, err)

	_, err = repo.List(ctx, "ws1", domain.AnnotationFilter{Limit: 7})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Len(t, executed, 2)
	// The literal is deliberate: writing AnnotationDefaultListLimit here would
	// assert the constant against itself and survive any change to it.
	assert.Contains(t, executed[0], "LIMIT 100")
	assert.Contains(t, executed[1], "LIMIT 7", "an explicit limit is honoured verbatim")
}

func TestAnnotationRepository_List_OrdersByAnnotatedAtDesc(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectQuery("SELECT").WillReturnRows(annotationRows())

	_, err := repo.List(ctx, "ws1", domain.AnnotationFilter{})
	require.NoError(t, err)

	require.Len(t, executed, 1)
	assert.Contains(t, executed[0], "ORDER BY annotated_at DESC")
}

func TestAnnotationRepository_List_ScansRows(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	annotatedAt := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)

	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectQuery("SELECT").WillReturnRows(annotationRows().
		AddRow("ann1", annotatedAt, "Asia/Tokyo", "Launch", "notes", "#3b82f6",
			domain.AnnotationSourceManual, nil, createdAt, createdAt).
		AddRow("ann2", annotatedAt, "UTC", "Campaign", "", "#7763f1",
			domain.AnnotationSourceBroadcast, "bcast42", createdAt, createdAt))

	annotations, err := repo.List(ctx, "ws1", domain.AnnotationFilter{})
	require.NoError(t, err)
	require.Len(t, annotations, 2)

	// A manual row has no source_id at all; the NULL must not become "".
	assert.Nil(t, annotations[0].SourceID)
	assert.Equal(t, "Asia/Tokyo", annotations[0].Timezone)

	require.NotNil(t, annotations[1].SourceID)
	assert.Equal(t, "bcast42", *annotations[1].SourceID)
	assert.True(t, annotations[1].IsSystem())
}

func TestAnnotationRepository_List_QueryError(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectQuery("SELECT").WillReturnError(errors.New("relation does not exist"))

	_, err := repo.List(ctx, "ws1", domain.AnnotationFilter{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list annotations")
}

func TestAnnotationRepository_Get_Success(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	annotatedAt := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)

	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectQuery("SELECT").
		WithArgs("ann1").
		WillReturnRows(annotationRows().AddRow("ann1", annotatedAt, "UTC", "Launch", "", "#3b82f6",
			domain.AnnotationSourceManual, nil, annotatedAt, annotatedAt))

	annotation, err := repo.Get(ctx, "ws1", "ann1")
	require.NoError(t, err)
	assert.Equal(t, "ann1", annotation.ID)
	assert.Equal(t, "Launch", annotation.Title)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAnnotationRepository_Get_NotFound(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectQuery("SELECT").WithArgs("missing").WillReturnError(sql.ErrNoRows)

	annotation, err := repo.Get(ctx, "ws1", "missing")
	assert.Nil(t, annotation)

	var notFound *domain.ErrNotFound
	require.ErrorAs(t, err, &notFound, "the handler maps *domain.ErrNotFound to 404")
	assert.Equal(t, "annotation", notFound.Entity)
	assert.Equal(t, "missing", notFound.ID)
}

func TestAnnotationRepository_Create_StampsTimestamps(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	annotation := testAnnotation()

	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectExec("INSERT INTO annotations").
		WithArgs(
			annotation.ID,
			annotation.AnnotatedAt,
			annotation.Timezone,
			annotation.Title,
			annotation.Description,
			annotation.Color,
			annotation.Source,
			nil,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	require.NoError(t, repo.Create(ctx, "ws1", annotation))
	require.NoError(t, mock.ExpectationsWereMet())

	assert.False(t, annotation.CreatedAt.IsZero(), "a caller that left the audit stamps unset gets them filled")
	assert.False(t, annotation.UpdatedAt.IsZero())
	require.Len(t, executed, 1)
	assert.NotContains(t, executed[0], "ON CONFLICT", "the manual path must surface a duplicate id, not swallow it")
}

func TestAnnotationRepository_Create_Error(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectExec("INSERT INTO annotations").WillReturnError(errors.New("duplicate key"))

	err := repo.Create(ctx, "ws1", testAnnotation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create annotation")
}

// TestAnnotationRepository_CreateFromSource_ConflictTargetMatchesPartialIndex
// pins the arbiter predicate. idx_annotations_source is a *partial* unique index
// (WHERE source_id IS NOT NULL), and PostgreSQL refuses to infer a partial index
// as an ON CONFLICT arbiter unless the predicate is repeated in the statement —
// the plain form raises 42P10 at runtime.
//
// This is a text pin only: sqlmock never parses SQL against a schema, so it
// cannot reproduce 42P10 no matter what this statement says. The real guards are
// the integration test on the broadcast annotation and the schema parity
// spot-check, both of which execute the insert against a live PostgreSQL.
func TestAnnotationRepository_CreateFromSource_ConflictTargetMatchesPartialIndex(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	sourceID := "broadcast42"
	annotation := testAnnotation()
	annotation.Source = domain.AnnotationSourceBroadcast
	annotation.SourceID = &sourceID

	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectExec("INSERT INTO annotations").WillReturnResult(sqlmock.NewResult(1, 1))

	created, err := repo.CreateFromSource(ctx, "ws1", annotation)
	require.NoError(t, err)
	assert.True(t, created)

	require.Len(t, executed, 1)
	assert.Contains(t, normalizeSQL(executed[0]),
		"ON CONFLICT (source, source_id) WHERE source_id IS NOT NULL DO NOTHING")
}

func TestAnnotationRepository_CreateFromSource_Conflict(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	sourceID := "broadcast42"
	annotation := testAnnotation()
	annotation.Source = domain.AnnotationSourceBroadcast
	annotation.SourceID = &sourceID

	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectExec("INSERT INTO annotations").WillReturnResult(sqlmock.NewResult(0, 0))

	created, err := repo.CreateFromSource(ctx, "ws1", annotation)
	// A second event for the same broadcast is the expected outcome of a task
	// retry, not a failure: the caller learns nothing was written and moves on.
	require.NoError(t, err)
	assert.False(t, created)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAnnotationRepository_CreateFromSource_Error(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectExec("INSERT INTO annotations").WillReturnError(errors.New("connection reset"))

	created, err := repo.CreateFromSource(ctx, "ws1", testAnnotation())
	require.Error(t, err)
	assert.False(t, created)
	assert.Contains(t, err.Error(), "failed to create annotation from source")
}

func TestAnnotationRepository_Update_Success(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	annotation := testAnnotation()

	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectExec("UPDATE annotations").
		WithArgs(
			annotation.AnnotatedAt,
			annotation.Timezone,
			annotation.Title,
			annotation.Description,
			annotation.Color,
			sqlmock.AnyArg(), // updated_at
			annotation.ID,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.Update(ctx, "ws1", annotation))
	require.NoError(t, mock.ExpectationsWereMet())
	assert.False(t, annotation.UpdatedAt.IsZero())
}

// TestAnnotationRepository_Update_DoesNotTouchSource pins that an edit cannot
// change where a row came from. Letting source_id through would let an operator
// steal a broadcast's idempotency slot, after which that broadcast's own
// annotation would be silently dropped by the ON CONFLICT above.
func TestAnnotationRepository_Update_DoesNotTouchSource(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	sourceID := "broadcast42"
	annotation := testAnnotation()
	annotation.Source = domain.AnnotationSourceBroadcast
	annotation.SourceID = &sourceID

	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectExec("UPDATE annotations").WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.Update(ctx, "ws1", annotation))

	require.Len(t, executed, 1)
	statement := normalizeSQL(executed[0])
	assert.NotContains(t, statement, "source =")
	assert.NotContains(t, statement, "source_id =")
	// The columns that *are* written, so a future rewrite cannot pass this test by
	// dropping the SET clause entirely.
	for _, column := range []string{"annotated_at =", "timezone =", "title =", "description =", "color =", "updated_at ="} {
		assert.Contains(t, statement, column)
	}
}

func TestAnnotationRepository_Update_NotFound(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectExec("UPDATE annotations").WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.Update(ctx, "ws1", testAnnotation())

	var notFound *domain.ErrNotFound
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, "ann123", notFound.ID)
}

func TestAnnotationRepository_Update_Error(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectExec("UPDATE annotations").WillReturnError(errors.New("deadlock detected"))

	err := repo.Update(ctx, "ws1", testAnnotation())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update annotation")
}

func TestAnnotationRepository_Delete_Success(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectExec("DELETE FROM annotations").
		WithArgs("ann123").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.Delete(ctx, "ws1", "ann123"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAnnotationRepository_Delete_NotFound(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectExec("DELETE FROM annotations").
		WithArgs("missing").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.Delete(ctx, "ws1", "missing")

	var notFound *domain.ErrNotFound
	require.ErrorAs(t, err, &notFound, "deleting nothing is a miss, not a silent success")
	assert.Equal(t, "annotation", notFound.Entity)
	assert.Equal(t, "missing", notFound.ID)
}

func TestAnnotationRepository_Delete_Error(t *testing.T) {
	var executed []string
	mockWorkspaceRepo, repo, mock, db := setupAnnotationTest(t, &executed)

	ctx := context.Background()
	mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(db, nil)
	mock.ExpectExec("DELETE FROM annotations").WillReturnError(errors.New("permission denied"))

	err := repo.Delete(ctx, "ws1", "ann123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete annotation")
}

// TestAnnotationRepository_ConnectionError covers every method: a workspace
// whose database cannot be reached must fail before any statement is built,
// never with a nil-connection panic.
func TestAnnotationRepository_ConnectionError(t *testing.T) {
	connErr := errors.New("workspace database unavailable")

	calls := map[string]func(repo *annotationRepository, ctx context.Context) error{
		"List": func(repo *annotationRepository, ctx context.Context) error {
			_, err := repo.List(ctx, "ws1", domain.AnnotationFilter{})
			return err
		},
		"Get": func(repo *annotationRepository, ctx context.Context) error {
			_, err := repo.Get(ctx, "ws1", "ann123")
			return err
		},
		"Create": func(repo *annotationRepository, ctx context.Context) error {
			return repo.Create(ctx, "ws1", testAnnotation())
		},
		"CreateFromSource": func(repo *annotationRepository, ctx context.Context) error {
			_, err := repo.CreateFromSource(ctx, "ws1", testAnnotation())
			return err
		},
		"Update": func(repo *annotationRepository, ctx context.Context) error {
			return repo.Update(ctx, "ws1", testAnnotation())
		},
		"Delete": func(repo *annotationRepository, ctx context.Context) error {
			return repo.Delete(ctx, "ws1", "ann123")
		},
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			var executed []string
			mockWorkspaceRepo, repo, _, _ := setupAnnotationTest(t, &executed)

			ctx := context.Background()
			mockWorkspaceRepo.EXPECT().GetConnection(ctx, "ws1").Return(nil, connErr)

			err := call(repo, ctx)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to get workspace connection")
			assert.ErrorIs(t, err, connErr)
			assert.Empty(t, executed, "no statement may reach the database")
		})
	}
}
