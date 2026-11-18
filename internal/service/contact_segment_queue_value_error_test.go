package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const queueEmail = "jane@test.com"

// queueHarness runs ProcessQueue over one claimed contact and two segments: segDate compares
// custom_json_1.renewed_at as a date, segPlain matches by email. Its mocks are strict: a write a
// test does not expect fails it, and so does a query sqlmock was not told about.
type queueHarness struct {
	processor   *ContactSegmentQueueProcessor
	segmentRepo *mocks.MockSegmentRepository
	mock        sqlmock.Sqlmock
	segDate     *domain.Segment
	segPlain    *domain.Segment
}

func newQueueHarness(t *testing.T) *queueHarness {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	segmentRepo := mocks.NewMockSegmentRepository(ctrl)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Debug(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	dateSQL := `SELECT email FROM contacts WHERE (((custom_json_1['renewed_at'] #>> '{}'))::timestamptz > $1)`
	plainSQL := "SELECT email FROM contacts WHERE email LIKE $1"
	h := &queueHarness{
		processor: NewContactSegmentQueueProcessor(mocks.NewMockContactSegmentQueueRepository(ctrl), segmentRepo,
			mocks.NewMockContactRepository(ctrl), workspaceRepo, log),
		segmentRepo: segmentRepo,
		mock:        mock,
		segDate: &domain.Segment{ID: "segdate", Status: string(domain.SegmentStatusActive), Version: 3,
			Tree: renewedAfterTree(), GeneratedSQL: &dateSQL, GeneratedArgs: domain.JSONArray{"2026-01-01T00:00:00Z"}},
		segPlain: &domain.Segment{ID: "segplain", Status: string(domain.SegmentStatusActive), Version: 5,
			Tree: createValidSegmentTree(), GeneratedSQL: &plainSQL, GeneratedArgs: domain.JSONArray{"%test%"}},
	}

	mock.ExpectBegin()
	mock.ExpectQuery("DELETE FROM contact_segment_queue").WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow(queueEmail))
	mock.ExpectCommit()
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace1").Return(db, nil)
	segmentRepo.EXPECT().GetSegments(gomock.Any(), "workspace1", false).DoAndReturn(
		func(context.Context, string, bool) ([]*domain.Segment, error) {
			return []*domain.Segment{h.segDate, h.segPlain}, nil
		})
	return h
}

// expectCombined makes the combined statement fail with err, from the query or, with afterRows,
// once a first row was returned.
func (h *queueHarness) expectCombined(err error, afterRows bool) {
	q := h.mock.ExpectQuery("SELECT 'segdate' as segment_id")
	if afterRows {
		q.WillReturnRows(sqlmock.NewRows([]string{"segment_id"}).AddRow("segplain").RowError(0, err))
		return
	}
	q.WillReturnError(err)
}

func (h *queueHarness) expectAlone(segment *domain.Segment) *sqlmock.ExpectedQuery {
	return h.mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS ("+*segment.GeneratedSQL+" AND email = $2)")).
		WithArgs(segment.GeneratedArgs[0], queueEmail)
}

func (h *queueHarness) expectDiagnosis(culprit bool) {
	rows := sqlmock.NewRows([]string{"email", "v"})
	if culprit {
		rows.AddRow(queueEmail, "2026-02-30")
	}
	h.mock.ExpectQuery(regexp.QuoteMeta(`SELECT email, `+renewedText+` FROM contacts WHERE `+renewedText+` IS NOT NULL AND NOT pg_input_is_valid(`+renewedText+`, 'timestamptz') AND `+renewedText+` = $1 AND email = ANY($2) LIMIT 1`)).
		WithArgs("2026-02-30", `{"`+queueEmail+`"}`).WillReturnRows(rows)
}

func queueValueError() *domain.SegmentLastError {
	return &domain.SegmentLastError{
		Kind: domain.SegmentLastErrorKindValueConversion, Column: "custom_json_1", Path: []string{"renewed_at"},
		Type: domain.SegmentValueTypeDate, Email: queueEmail, Value: "2026-02-30", Cause: outOfRange.Message,
		Source: domain.SegmentLastErrorSourceContactQueue,
	}
}

func TestContactSegmentQueueProcessor_DataErrorFallsBackToEachSegment(t *testing.T) {
	for _, afterRows := range []bool{false, true} {
		name := "from the query"
		if afterRows {
			name = "from the rows"
		}
		t.Run(name, func(t *testing.T) {
			h := newQueueHarness(t)
			ctx := context.Background()
			h.expectCombined(outOfRange, afterRows)
			h.expectAlone(h.segDate).WillReturnError(outOfRange)
			h.expectDiagnosis(true)
			h.expectAlone(h.segPlain).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

			// The segment that raised gets no add and no remove, and its error is recorded at the
			// snapshot's version; the other segment gets the contact. No INSERT into the queue.
			h.segmentRepo.EXPECT().SetSegmentLastError(ctx, "workspace1", "segdate", int64(3), queueValueError()).Return(true, nil)
			h.segmentRepo.EXPECT().AddContactToSegment(ctx, "workspace1", queueEmail, "segplain", int64(5)).Return(nil)

			count, err := h.processor.ProcessQueue(ctx, "workspace1")
			require.NoError(t, err)
			assert.Equal(t, 1, count, "the contact leaves the queue")
			require.NoError(t, h.mock.ExpectationsWereMet())
			assert.Equal(t, queueValueError(), h.segDate.LastError, "the batch's snapshot follows the write")
		})
	}
}

func TestContactSegmentQueueProcessor_DataErrorWithAnErrorAlreadyRecorded(t *testing.T) {
	h := newQueueHarness(t)
	ctx := context.Background()
	h.segDate.LastError = &domain.SegmentLastError{Kind: domain.SegmentLastErrorKindValueConversion,
		Email: "first@test.com", Source: domain.SegmentLastErrorSourceContactQueue}
	h.expectCombined(outOfRange, false)
	// No diagnosis query and no SetSegmentLastError: the first error recorded stays.
	h.expectAlone(h.segDate).WillReturnError(outOfRange)
	h.expectAlone(h.segPlain).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	h.segmentRepo.EXPECT().RemoveContactFromSegment(ctx, "workspace1", queueEmail, "segplain").Return(nil)

	count, err := h.processor.ProcessQueue(ctx, "workspace1")
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	require.NoError(t, h.mock.ExpectationsWereMet())
}

func TestContactSegmentQueueProcessor_DataErrorWithoutCulpritStillRecorded(t *testing.T) {
	h := newQueueHarness(t)
	ctx := context.Background()
	h.expectCombined(outOfRange, false)
	h.expectAlone(h.segDate).WillReturnError(outOfRange)
	h.expectDiagnosis(false)
	h.mock.ExpectQuery(regexp.QuoteMeta(`AND email = ANY($1) LIMIT 1`)).WillReturnRows(sqlmock.NewRows([]string{"email", "v"}))
	h.expectAlone(h.segPlain).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	h.segmentRepo.EXPECT().SetSegmentLastError(ctx, "workspace1", "segdate", int64(3), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ int64, lastErr *domain.SegmentLastError) (bool, error) {
			assert.Equal(t, domain.SegmentLastErrorKindValueConversion, lastErr.Kind)
			// The query raised on this contact: naming it lets the queue clear the error once its
			// value converts, even though the diagnosis did not find the value
			assert.Equal(t, queueEmail, lastErr.Email)
			assert.Equal(t, "", lastErr.Value)
			assert.Equal(t, "custom_json_1", lastErr.Column)
			return true, nil
		})
	h.segmentRepo.EXPECT().AddContactToSegment(ctx, "workspace1", queueEmail, "segplain", int64(5)).Return(nil)

	_, err := h.processor.ProcessQueue(ctx, "workspace1")
	require.NoError(t, err)
}

func TestContactSegmentQueueProcessor_DataErrorAndAWriteErrorRequeues(t *testing.T) {
	h := newQueueHarness(t)
	ctx := context.Background()
	h.expectCombined(outOfRange, false)
	h.expectAlone(h.segDate).WillReturnError(outOfRange)
	h.expectDiagnosis(true)
	h.expectAlone(h.segPlain).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	h.segmentRepo.EXPECT().SetSegmentLastError(ctx, "workspace1", "segdate", int64(3), gomock.Any()).Return(true, nil)
	h.segmentRepo.EXPECT().AddContactToSegment(ctx, "workspace1", queueEmail, "segplain", int64(5)).Return(errors.New("insert failed"))
	h.mock.ExpectExec("INSERT INTO contact_segment_queue").WithArgs(queueEmail).WillReturnResult(sqlmock.NewResult(0, 1))

	count, err := h.processor.ProcessQueue(ctx, "workspace1")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	require.NoError(t, h.mock.ExpectationsWereMet())
}

func TestContactSegmentQueueProcessor_RecordingTheErrorFailsRequeues(t *testing.T) {
	h := newQueueHarness(t)
	ctx := context.Background()
	h.expectCombined(outOfRange, false)
	h.expectAlone(h.segDate).WillReturnError(outOfRange)
	h.expectDiagnosis(true)
	h.expectAlone(h.segPlain).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	h.segmentRepo.EXPECT().SetSegmentLastError(ctx, "workspace1", "segdate", int64(3), gomock.Any()).Return(false, errors.New("db down"))
	h.segmentRepo.EXPECT().AddContactToSegment(ctx, "workspace1", queueEmail, "segplain", int64(5)).Return(nil)
	h.mock.ExpectExec("INSERT INTO contact_segment_queue").WithArgs(queueEmail).WillReturnResult(sqlmock.NewResult(0, 1))

	count, err := h.processor.ProcessQueue(ctx, "workspace1")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	require.NoError(t, h.mock.ExpectationsWereMet())
}

func TestContactSegmentQueueProcessor_OtherErrorInTheSegmentPassRequeues(t *testing.T) {
	h := newQueueHarness(t)
	ctx := context.Background()
	h.expectCombined(outOfRange, false)
	h.expectAlone(h.segDate).WillReturnError(outOfRange)
	h.expectDiagnosis(true)
	h.expectAlone(h.segPlain).WillReturnError(errors.New("connection reset"))
	h.segmentRepo.EXPECT().SetSegmentLastError(ctx, "workspace1", "segdate", int64(3), gomock.Any()).Return(true, nil)
	h.mock.ExpectExec("INSERT INTO contact_segment_queue").WithArgs(queueEmail).WillReturnResult(sqlmock.NewResult(0, 1))

	count, err := h.processor.ProcessQueue(ctx, "workspace1")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	require.NoError(t, h.mock.ExpectationsWereMet())
}

func TestContactSegmentQueueProcessor_OtherCombinedErrorRequeuesAsBefore(t *testing.T) {
	h := newQueueHarness(t)
	ctx := context.Background()
	// 22021 is class 22 but says nothing about a contact's value: no per-segment pass.
	h.expectCombined(fmt.Errorf("wrapped: %w", &pq.Error{Code: "22021", Message: "invalid byte sequence"}), false)
	h.mock.ExpectExec("INSERT INTO contact_segment_queue").WithArgs(queueEmail).WillReturnResult(sqlmock.NewResult(0, 1))

	count, err := h.processor.ProcessQueue(ctx, "workspace1")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	require.NoError(t, h.mock.ExpectationsWereMet())
}

func TestContactSegmentQueueProcessor_ClearsTheErrorNamingTheContact(t *testing.T) {
	t.Run("after the combined statement", func(t *testing.T) {
		h := newQueueHarness(t)
		ctx := context.Background()
		h.segDate.LastError = queueValueError()
		h.mock.ExpectQuery("SELECT 'segdate' as segment_id").WillReturnRows(sqlmock.NewRows([]string{"segment_id"}).AddRow("segdate"))
		h.segmentRepo.EXPECT().AddContactToSegment(ctx, "workspace1", queueEmail, "segdate", int64(3)).Return(nil)
		h.segmentRepo.EXPECT().RemoveContactFromSegment(ctx, "workspace1", queueEmail, "segplain").Return(nil)
		h.segmentRepo.EXPECT().ClearSegmentLastErrorForEmail(ctx, "workspace1", "segdate", int64(3), queueEmail).Return(true, nil)

		count, err := h.processor.ProcessQueue(ctx, "workspace1")
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		assert.Nil(t, h.segDate.LastError, "the batch's snapshot follows the clear")
	})

	t.Run("after the segment answered alone", func(t *testing.T) {
		h := newQueueHarness(t)
		ctx := context.Background()
		h.segPlain.LastError = &domain.SegmentLastError{Kind: domain.SegmentLastErrorKindValueConversion,
			Email: queueEmail, Source: domain.SegmentLastErrorSourceContactQueue}
		h.segDate.LastError = &domain.SegmentLastError{Kind: domain.SegmentLastErrorKindValueConversion,
			Email: "other@test.com", Source: domain.SegmentLastErrorSourceContactQueue}
		h.expectCombined(outOfRange, false)
		h.expectAlone(h.segDate).WillReturnError(outOfRange)
		h.expectAlone(h.segPlain).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		h.segmentRepo.EXPECT().AddContactToSegment(ctx, "workspace1", queueEmail, "segplain", int64(5)).Return(nil)
		h.segmentRepo.EXPECT().ClearSegmentLastErrorForEmail(ctx, "workspace1", "segplain", int64(5), queueEmail).Return(true, nil)

		_, err := h.processor.ProcessQueue(ctx, "workspace1")
		require.NoError(t, err)
	})
}

// A healthy pass issues no statement for an error that names another contact or comes from a
// build: the strict mock fails on a ClearSegmentLastErrorForEmail.
func TestContactSegmentQueueProcessor_HealthyPassLeavesOtherErrorsAlone(t *testing.T) {
	h := newQueueHarness(t)
	ctx := context.Background()
	h.segDate.LastError = &domain.SegmentLastError{Kind: domain.SegmentLastErrorKindValueConversion,
		Email: queueEmail, Source: domain.SegmentLastErrorSourceBuild}
	h.segPlain.LastError = &domain.SegmentLastError{Kind: domain.SegmentLastErrorKindValueConversion,
		Email: "other@test.com", Source: domain.SegmentLastErrorSourceContactQueue}
	h.mock.ExpectQuery("SELECT 'segdate' as segment_id").WillReturnRows(sqlmock.NewRows([]string{"segment_id"}).AddRow("segplain"))
	h.segmentRepo.EXPECT().RemoveContactFromSegment(ctx, "workspace1", queueEmail, "segdate").Return(nil)
	h.segmentRepo.EXPECT().AddContactToSegment(ctx, "workspace1", queueEmail, "segplain", int64(5)).Return(nil)

	count, err := h.processor.ProcessQueue(ctx, "workspace1")
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	require.NoError(t, h.mock.ExpectationsWereMet())
}
