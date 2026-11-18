package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lastErrorMatcher matches a *domain.SegmentLastError on its kind and source.
type lastErrorMatcher struct {
	kind, source string
}

func (m lastErrorMatcher) Matches(x interface{}) bool {
	lastErr, ok := x.(*domain.SegmentLastError)
	return ok && lastErr != nil && lastErr.Kind == m.kind && lastErr.Source == m.source && lastErr.Cause != ""
}

func (m lastErrorMatcher) String() string {
	return fmt.Sprintf("a %s last error from the %s", m.kind, m.source)
}

// isOtherBuildError matches the error a build records when it fails for good on anything but a
// value it cannot convert.
func isOtherBuildError() gomock.Matcher {
	return lastErrorMatcher{kind: domain.SegmentLastErrorKindOther, source: domain.SegmentLastErrorSourceBuild}
}

// segmentWithJSONDate is a segment comparing custom_json_1.renewed_at as a date, building.
func segmentWithJSONDate(version int64) *domain.Segment {
	sqlText := `SELECT email FROM contacts WHERE (((custom_json_1['renewed_at'] #>> '{}'))::timestamptz > $1)`
	due := time.Now().Add(-time.Hour)
	return &domain.Segment{
		ID: "segment1", Name: "Renewed", Status: string(domain.SegmentStatusBuilding), Version: version,
		Timezone: "UTC", Tree: renewedAfterTree(), GeneratedSQL: &sqlText,
		GeneratedArgs:  domain.JSONArray{"2026-01-01T00:00:00Z"},
		RecomputeAfter: &due,
	}
}

const batchDiagnosisQuery = `SELECT email, ` + renewedText + ` FROM contacts WHERE ` + renewedText + ` IS NOT NULL AND NOT pg_input_is_valid(` + renewedText + `, 'timestamptz') AND ` + renewedText + ` = $1 AND email = ANY($2) LIMIT 1`

// expectFailingBatch makes the batch query raise outOfRange, from the query itself or, with
// afterRows, once a first row was returned; then expects the diagnosis within the batch.
func (h *buildProcessorHarness) expectFailingBatch(t *testing.T, afterRows bool, batch []string, culprit string) sqlmock.Sqlmock {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	batchQuery := *segmentWithJSONDate(1).GeneratedSQL + " AND email = ANY($2)"
	if afterRows {
		mock.ExpectQuery(batchQuery).WillReturnRows(
			sqlmock.NewRows([]string{"email"}).AddRow(batch[0]).AddRow(batch[1]).RowError(1, outOfRange))
	} else {
		mock.ExpectQuery(batchQuery).WillReturnError(outOfRange)
	}
	diagnosis := sqlmock.NewRows([]string{"email", "v"})
	if culprit != "" {
		diagnosis.AddRow(culprit, "2026-02-30")
	}
	mock.ExpectQuery(batchDiagnosisQuery).WithArgs("2026-02-30", fmt.Sprintf(`{"%s","%s"}`, batch[0], batch[1])).
		WillReturnRows(diagnosis)
	if culprit == "" {
		mock.ExpectQuery(`SELECT email, ` + renewedText + ` FROM contacts WHERE ` + renewedText + ` IS NOT NULL AND NOT pg_input_is_valid(` + renewedText + `, 'timestamptz') AND email = ANY($1) LIMIT 1`).
			WillReturnRows(sqlmock.NewRows([]string{"email", "v"}))
	}
	h.wsRepo.EXPECT().GetConnection(gomock.Any(), "workspace1").Return(db, nil).Times(2)
	return mock
}

func TestSegmentBuildProcessor_Process_DataErrorEndsTheBuild(t *testing.T) {
	for _, afterRows := range []bool{false, true} {
		name := "from the query"
		if afterRows {
			name = "from the rows"
		}
		t.Run(name, func(t *testing.T) {
			h := newBuildProcessorHarness(t)
			ctx := context.Background()
			// Retries left: a data error does not use them.
			task := buildTask(1, false)
			segment := segmentWithJSONDate(1)
			batch := []string{"a@test.com", "jane@test.com"}

			h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil).AnyTimes()
			h.contactRepo.EXPECT().Count(ctx, "workspace1").Return(2, nil)
			h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "", 100).Return(batch, nil)
			mock := h.expectFailingBatch(t, afterRows, batch, "jane@test.com")

			h.segmentRepo.EXPECT().ReleaseSegmentWithError(gomock.Any(), "workspace1", "segment1", int64(1), gomock.Any()).
				DoAndReturn(func(_ context.Context, _, _ string, _ int64, lastErr *domain.SegmentLastError) (bool, error) {
					assert.Equal(t, &domain.SegmentLastError{
						Kind: domain.SegmentLastErrorKindValueConversion, Column: "custom_json_1", Path: []string{"renewed_at"},
						Type: domain.SegmentValueTypeDate, Email: "jane@test.com", Value: "2026-02-30",
						Cause: outOfRange.Message, Source: domain.SegmentLastErrorSourceBuild,
					}, lastErr)
					return true, nil
				})
			// The daily rebuild moves to the next day, not to every tick.
			h.segmentRepo.EXPECT().UpdateRecomputeAfter(gomock.Any(), "workspace1", "segment1", gomock.Any()).
				DoAndReturn(func(_ context.Context, _, _ string, at *time.Time) error {
					require.NotNil(t, at)
					assert.True(t, at.After(time.Now()))
					return nil
				})

			completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
			require.NoError(t, err)
			assert.True(t, completed, "retrying cannot help")
			assert.Equal(t, "stopped: a contact's custom_json_1.renewed_at is not a valid date; see the segment", task.State.Message)
			assert.NotContains(t, task.State.Message, "jane@test.com", "tasks are readable without contacts read")
			assert.NotContains(t, task.State.Message, "2026-02-30")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestSegmentBuildProcessor_Process_DataErrorWithoutCandidateIsKindOther(t *testing.T) {
	h := newBuildProcessorHarness(t)
	ctx := context.Background()
	task := buildTask(1, false)
	segment := createTestSegmentWithSQL("segment1", "S", 1, string(domain.SegmentStatusBuilding))

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("SELECT email FROM contacts").WillReturnError(&pq.Error{Code: "22P02", Message: `invalid input syntax for type numeric: "x"`})

	h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil).AnyTimes()
	h.contactRepo.EXPECT().Count(ctx, "workspace1").Return(1, nil)
	h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "", 100).Return([]string{"a@test.com"}, nil)
	h.wsRepo.EXPECT().GetConnection(gomock.Any(), "workspace1").Return(db, nil).Times(2)
	h.segmentRepo.EXPECT().ReleaseSegmentWithError(gomock.Any(), "workspace1", "segment1", int64(1), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ int64, lastErr *domain.SegmentLastError) (bool, error) {
			assert.Equal(t, domain.NewOtherSegmentLastError(domain.SegmentLastErrorSourceBuild, `invalid input syntax for type numeric: "x"`), lastErr)
			return true, nil
		})

	completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
	require.NoError(t, err)
	assert.True(t, completed)
	assert.Equal(t, "stopped: a contact value could not be converted; see the segment", task.State.Message)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSegmentBuildProcessor_Process_DataErrorOnAMovedSegmentReschedulesNothing(t *testing.T) {
	h := newBuildProcessorHarness(t)
	ctx := context.Background()
	task := buildTask(1, false)
	segment := segmentWithJSONDate(1)
	batch := []string{"a@test.com", "jane@test.com"}

	h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil).AnyTimes()
	h.contactRepo.EXPECT().Count(ctx, "workspace1").Return(2, nil)
	h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "", 100).Return(batch, nil)
	h.expectFailingBatch(t, false, batch, "jane@test.com")
	// The segment moved on since: nothing written, and no UpdateRecomputeAfter (the mock is strict).
	h.segmentRepo.EXPECT().ReleaseSegmentWithError(gomock.Any(), "workspace1", "segment1", int64(1), gomock.Any()).Return(false, nil)

	completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
	require.NoError(t, err)
	assert.True(t, completed)
}

func TestSegmentBuildProcessor_Process_DataErrorWriteFailsIsRetried(t *testing.T) {
	h := newBuildProcessorHarness(t)
	ctx := context.Background()
	task := buildTask(1, false)
	segment := segmentWithJSONDate(1)
	batch := []string{"a@test.com", "jane@test.com"}

	h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil).AnyTimes()
	h.contactRepo.EXPECT().Count(ctx, "workspace1").Return(2, nil)
	h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "", 100).Return(batch, nil)
	h.expectFailingBatch(t, false, batch, "jane@test.com")
	h.segmentRepo.EXPECT().ReleaseSegmentWithError(gomock.Any(), "workspace1", "segment1", int64(1), gomock.Any()).
		Return(false, errors.New("db down"))

	completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
	require.Error(t, err)
	assert.False(t, completed, "the segment is still building: the task is retried")
}

// The task's context runs out while the value is looked for: the error is still recorded, on a
// context of its own.
func TestSegmentBuildProcessor_Process_DataErrorRecordedAfterTheDeadline(t *testing.T) {
	h := newBuildProcessorHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	task := buildTask(1, false)
	segment := segmentWithJSONDate(1)
	batch := []string{"a@test.com", "jane@test.com"}

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery(*segment.GeneratedSQL + " AND email = ANY($2)").WillReturnError(outOfRange)
	mock.ExpectQuery(batchDiagnosisQuery).WithArgs("2026-02-30", `{"a@test.com","jane@test.com"}`).
		WillDelayFor(300 * time.Millisecond).
		WillReturnRows(sqlmock.NewRows([]string{"email", "v"}).AddRow("jane@test.com", "2026-02-30"))

	h.segmentRepo.EXPECT().GetSegmentByID(gomock.Any(), "workspace1", "segment1").Return(segment, nil).AnyTimes()
	h.contactRepo.EXPECT().Count(gomock.Any(), "workspace1").Return(2, nil)
	h.contactRepo.EXPECT().GetBatchForSegmentAfter(gomock.Any(), "workspace1", "", 100).Return(batch, nil)
	h.wsRepo.EXPECT().GetConnection(gomock.Any(), "workspace1").Return(db, nil).Times(2)
	h.segmentRepo.EXPECT().ReleaseSegmentWithError(gomock.Any(), "workspace1", "segment1", int64(1), gomock.Any()).
		DoAndReturn(func(c context.Context, _, _ string, _ int64, lastErr *domain.SegmentLastError) (bool, error) {
			require.Error(t, ctx.Err(), "the task's context is over")
			assert.NoError(t, c.Err(), "the write runs on a live context")
			assert.Equal(t, "jane@test.com", lastErr.Email, "the diagnosis ran past the task's deadline too")
			return true, nil
		})
	h.segmentRepo.EXPECT().UpdateRecomputeAfter(gomock.Any(), "workspace1", "segment1", gomock.Any()).
		DoAndReturn(func(c context.Context, _, _ string, _ *time.Time) error {
			assert.NoError(t, c.Err())
			return nil
		})

	completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
	require.NoError(t, err)
	assert.True(t, completed)
}

// A build that fails for good on anything else records that error, as kind other.
func TestSegmentBuildProcessor_Process_FinalFailureRecordsTheError(t *testing.T) {
	h := newBuildProcessorHarness(t)
	ctx := context.Background()
	task := buildTask(1, true)
	segment := createTestSegmentWithSQL("segment1", "S", 1, string(domain.SegmentStatusBuilding))

	h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil).AnyTimes()
	h.contactRepo.EXPECT().Count(ctx, "workspace1").Return(1, nil)
	h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "", 100).Return(nil, errors.New("db down"))
	h.segmentRepo.EXPECT().ReleaseSegmentWithError(gomock.Any(), "workspace1", "segment1", int64(1), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ int64, lastErr *domain.SegmentLastError) (bool, error) {
			require.NotNil(t, lastErr)
			assert.Equal(t, domain.SegmentLastErrorKindOther, lastErr.Kind)
			assert.Equal(t, domain.SegmentLastErrorSourceBuild, lastErr.Source)
			assert.Contains(t, lastErr.Cause, "db down")
			return true, nil
		})

	_, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
	require.Error(t, err)
}

// An error from a membership write that happens to carry a conversion code is a write, not a
// query on a contact's value: it is retried as before.
func TestSegmentBuildProcessor_Process_ConversionCodeFromAWriteIsRetried(t *testing.T) {
	h := newBuildProcessorHarness(t)
	ctx := context.Background()
	task := buildTask(1, false)
	segment := createTestSegmentWithSQL("segment1", "S", 1, string(domain.SegmentStatusBuilding))

	h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil).AnyTimes()
	h.contactRepo.EXPECT().Count(ctx, "workspace1").Return(1, nil)
	h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "", 100).Return([]string{"a@test.com"}, nil)
	h.expectQuery(t, "a@test.com")
	h.segmentRepo.EXPECT().AddContactToSegment(ctx, "workspace1", "a@test.com", "segment1", int64(1)).
		Return(fmt.Errorf("failed to add contact to segment: %w", &pq.Error{Code: "22P02"}))

	completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
	require.Error(t, err)
	assert.False(t, completed)
}
