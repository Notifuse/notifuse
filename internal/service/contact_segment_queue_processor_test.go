package service

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewContactSegmentQueueProcessor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	assert.NotNil(t, processor)
	assert.NotNil(t, processor.queueRepo)
	assert.NotNil(t, processor.segmentRepo)
	assert.NotNil(t, processor.contactRepo)
	assert.NotNil(t, processor.workspaceRepo)
	assert.NotNil(t, processor.queryBuilder)
	assert.NotNil(t, processor.logger)
	assert.Equal(t, 100, processor.batchSize)
}

func TestContactSegmentQueueProcessor_ProcessQueue_GetConnectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(nil, errors.New("connection failed"))

	count, err := processor.ProcessQueue(ctx, "workspace1")
	assert.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "failed to get workspace connection")
}

// TestContactSegmentQueueProcessor_ProcessQueue_CancelledContext verifies the
// entry gate: a caller whose context is already cancelled must not claim new
// work (the claim itself deliberately ignores cancellation once started), so
// ProcessQueue returns before even acquiring a connection.
func TestContactSegmentQueueProcessor_ProcessQueue_CancelledContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// No GetConnection expectation: the gate must short-circuit first.
	count, err := processor.ProcessQueue(ctx, "workspace1")
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, count)
}

// TestContactSegmentQueueProcessor_ProcessQueue_ClaimError verifies that a
// failure of the atomic claim (DELETE ... RETURNING inside its short
// transaction) surfaces as an error, rolls back, and processes nothing.
func TestContactSegmentQueueProcessor_ProcessQueue_ClaimError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	db, mock, err := sqlmock.New()
	var count int
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	// The claim runs in a short transaction; a query failure rolls it back so
	// the rows survive for the next pass.
	mock.ExpectBegin()
	mock.ExpectQuery("DELETE FROM contact_segment_queue").
		WillReturnError(errors.New("claim failed"))
	mock.ExpectRollback()

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	count, err = processor.ProcessQueue(ctx, "workspace1")
	assert.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "failed to claim pending emails")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactSegmentQueueProcessor_ProcessQueue_NoPendingContacts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	db, mock, err := sqlmock.New()
	var count int
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Claim returns no rows: the transaction still commits, nothing to process.
	mock.ExpectBegin()
	rows := sqlmock.NewRows([]string{"email"})
	mock.ExpectQuery("DELETE FROM contact_segment_queue").WillReturnRows(rows)
	mock.ExpectCommit()

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	count, err = processor.ProcessQueue(ctx, "workspace1")
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestContactSegmentQueueProcessor_ProcessQueue_GetSegmentsError verifies that a
// GetSegments failure re-enqueues the already-claimed contacts (so they are not
// lost, since the claim deleted them) before surfacing the error.
func TestContactSegmentQueueProcessor_ProcessQueue_GetSegmentsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	db, mock, err := sqlmock.New()
	var count int
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Claim one contact.
	mock.ExpectBegin()
	rows := sqlmock.NewRows([]string{"email"}).AddRow("test@test.com")
	mock.ExpectQuery("DELETE FROM contact_segment_queue").WillReturnRows(rows)
	mock.ExpectCommit()
	// GetSegments fails -> the claimed contact is re-enqueued.
	mock.ExpectExec("INSERT INTO contact_segment_queue").
		WillReturnResult(sqlmock.NewResult(0, 1))

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	mockSegmentRepo.EXPECT().
		GetSegments(ctx, "workspace1", false).
		Return(nil, errors.New("db error"))

	count, err = processor.ProcessQueue(ctx, "workspace1")
	assert.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "failed to get segments")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestContactSegmentQueueProcessor_ProcessQueue_NoActiveSegments verifies that
// when there are no active segments, the claimed rows (already deleted by the
// claim) are simply reported as processed — no separate delete is needed.
func TestContactSegmentQueueProcessor_ProcessQueue_NoActiveSegments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	db, mock, err := sqlmock.New()
	var count int
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	rows := sqlmock.NewRows([]string{"email"}).AddRow("test@test.com")
	mock.ExpectQuery("DELETE FROM contact_segment_queue").WillReturnRows(rows)
	mock.ExpectCommit()

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	mockSegmentRepo.EXPECT().
		GetSegments(ctx, "workspace1", false).
		Return([]*domain.Segment{}, nil)

	count, err = processor.ProcessQueue(ctx, "workspace1")
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactSegmentQueueProcessor_ProcessQueue_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	db, mock, err := sqlmock.New()
	var count int
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Setup segment
	sql := "SELECT email FROM contacts WHERE email LIKE $1"
	segment := &domain.Segment{
		ID:            "segment1",
		Name:          "Test Segment",
		Status:        string(domain.SegmentStatusActive),
		Version:       1,
		GeneratedSQL:  &sql,
		GeneratedArgs: domain.JSONArray{"%test%"},
	}

	// Claim one contact.
	mock.ExpectBegin()
	emailRows := sqlmock.NewRows([]string{"email"}).AddRow("test@test.com")
	mock.ExpectQuery("DELETE FROM contact_segment_queue").WillReturnRows(emailRows)
	mock.ExpectCommit()

	// Segment evaluation query (contact matches).
	segmentRows := sqlmock.NewRows([]string{"segment_id"}).AddRow("segment1")
	mock.ExpectQuery("SELECT 'segment1' as segment_id").WillReturnRows(segmentRows)

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	mockSegmentRepo.EXPECT().
		GetSegments(ctx, "workspace1", false).
		Return([]*domain.Segment{segment}, nil)

	mockSegmentRepo.EXPECT().
		AddContactToSegment(ctx, "workspace1", "test@test.com", "segment1", int64(1)).
		Return(nil)

	count, err = processor.ProcessQueue(ctx, "workspace1")
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactSegmentQueueProcessor_ProcessQueue_RemoveFromSegment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	db, mock, err := sqlmock.New()
	var count int
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Setup segment
	sql := "SELECT email FROM contacts WHERE email LIKE $1"
	segment := &domain.Segment{
		ID:            "segment1",
		Name:          "Test Segment",
		Status:        string(domain.SegmentStatusActive),
		Version:       1,
		GeneratedSQL:  &sql,
		GeneratedArgs: domain.JSONArray{"%test%"},
	}

	// Claim one contact.
	mock.ExpectBegin()
	emailRows := sqlmock.NewRows([]string{"email"}).AddRow("test@test.com")
	mock.ExpectQuery("DELETE FROM contact_segment_queue").WillReturnRows(emailRows)
	mock.ExpectCommit()

	// Segment evaluation query - no rows returned means contact doesn't match.
	segmentRows := sqlmock.NewRows([]string{"segment_id"})
	mock.ExpectQuery("SELECT 'segment1' as segment_id").WillReturnRows(segmentRows)

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	mockSegmentRepo.EXPECT().
		GetSegments(ctx, "workspace1", false).
		Return([]*domain.Segment{segment}, nil)

	mockSegmentRepo.EXPECT().
		RemoveContactFromSegment(ctx, "workspace1", "test@test.com", "segment1").
		Return(nil)

	count, err = processor.ProcessQueue(ctx, "workspace1")
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestContactSegmentQueueProcessor_ProcessQueue_ProcessContactError verifies that
// a per-contact evaluation failure does not fail the batch: the contact is
// re-enqueued for retry (claimBatch already removed it) and the processed count
// reflects the failure.
func TestContactSegmentQueueProcessor_ProcessQueue_ProcessContactError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	db, mock, err := sqlmock.New()
	var count int
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Setup segment
	sql := "SELECT email FROM contacts WHERE email LIKE $1"
	segment := &domain.Segment{
		ID:            "segment1",
		Name:          "Test Segment",
		Status:        string(domain.SegmentStatusActive),
		Version:       1,
		GeneratedSQL:  &sql,
		GeneratedArgs: domain.JSONArray{"%test%"},
	}

	// Claim one contact.
	mock.ExpectBegin()
	emailRows := sqlmock.NewRows([]string{"email"}).AddRow("test@test.com")
	mock.ExpectQuery("DELETE FROM contact_segment_queue").WillReturnRows(emailRows)
	mock.ExpectCommit()

	// Segment evaluation query fails -> processContact returns an error.
	mock.ExpectQuery("SELECT 'segment1' as segment_id").
		WillReturnError(errors.New("eval failed"))

	// The failed contact is re-enqueued.
	mock.ExpectExec("INSERT INTO contact_segment_queue").
		WillReturnResult(sqlmock.NewResult(0, 1))

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	mockSegmentRepo.EXPECT().
		GetSegments(ctx, "workspace1", false).
		Return([]*domain.Segment{segment}, nil)

	count, err = processor.ProcessQueue(ctx, "workspace1")
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestContactSegmentQueueProcessor_ProcessQueue_ProcessContactPanic_RecoveredAndRequeued
// verifies the per-contact panic containment: a panic evaluating one contact is
// recovered (this is a permanent recurring task — crashing would re-claim the
// same contact after the debounce and crash again, forever), the contact takes
// the failed-contact requeue path, and ProcessQueue returns normally.
func TestContactSegmentQueueProcessor_ProcessQueue_ProcessContactPanic_RecoveredAndRequeued(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Non-empty GeneratedSQL is required so processContact reaches the
	// membership write (an empty one returns before any segment call).
	sql := "SELECT email FROM contacts WHERE email LIKE $1"
	segment := &domain.Segment{
		ID:            "segment1",
		Name:          "Test Segment",
		Status:        string(domain.SegmentStatusActive),
		Version:       1,
		GeneratedSQL:  &sql,
		GeneratedArgs: domain.JSONArray{"%test%"},
	}

	// Claim one contact.
	mock.ExpectBegin()
	emailRows := sqlmock.NewRows([]string{"email"}).AddRow("test@test.com")
	mock.ExpectQuery("DELETE FROM contact_segment_queue").WillReturnRows(emailRows)
	mock.ExpectCommit()

	// Eval matches segment1 so the panicking AddContactToSegment is reached.
	segmentRows := sqlmock.NewRows([]string{"segment_id"}).AddRow("segment1")
	mock.ExpectQuery("SELECT 'segment1' as segment_id").WillReturnRows(segmentRows)

	// The panicked contact is re-enqueued via the failed-contact path.
	mock.ExpectExec("INSERT INTO contact_segment_queue").
		WillReturnResult(sqlmock.NewResult(0, 1))

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	mockSegmentRepo.EXPECT().
		GetSegments(ctx, "workspace1", false).
		Return([]*domain.Segment{segment}, nil)

	mockSegmentRepo.EXPECT().
		AddContactToSegment(ctx, "workspace1", "test@test.com", "segment1", int64(1)).
		Do(func(context.Context, string, string, string, int64) {
			panic("boom")
		}).
		Return(nil)

	var count int
	assert.NotPanics(t, func() {
		count, err = processor.ProcessQueue(ctx, "workspace1")
	})
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestContactSegmentQueueProcessor_ProcessQueue_CancelledMidBatch_RequeuesRemainder
// verifies the cancellation path INSIDE the batch: when the caller's context is
// cancelled after the claim but before per-contact work, every unprocessed
// contact must be re-enqueued (the claim already deleted the rows) and no
// contact may be evaluated. The requeue itself must succeed despite the
// cancelled parent: it runs on a detached context — with a non-detached one,
// database/sql would reject the Exec before it reached the driver, so the
// requeue expectation below also pins the detachment.
func TestContactSegmentQueueProcessor_ProcessQueue_CancelledMidBatch_RequeuesRemainder(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	sql := "SELECT email FROM contacts WHERE email LIKE $1"
	segment := &domain.Segment{
		ID:            "segment1",
		Name:          "Test Segment",
		Status:        string(domain.SegmentStatusActive),
		Version:       1,
		GeneratedSQL:  &sql,
		GeneratedArgs: domain.JSONArray{"%test%"},
	}

	// Claim two contacts.
	mock.ExpectBegin()
	emailRows := sqlmock.NewRows([]string{"email"}).
		AddRow("a@test.com").
		AddRow("b@test.com")
	mock.ExpectQuery("DELETE FROM contact_segment_queue").WillReturnRows(emailRows)
	mock.ExpectCommit()

	// No eval query is expected between commit and requeue: after the
	// cancellation below, the loop must not process any contact.

	// The whole claimed batch is re-enqueued.
	mock.ExpectExec("INSERT INTO contact_segment_queue").
		WithArgs("a@test.com", "b@test.com").
		WillReturnResult(sqlmock.NewResult(0, 2))

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	// Cancel between the claim and the per-contact loop: GetSegments runs after
	// claimBatch commits, so cancelling here lands exactly in that window.
	mockSegmentRepo.EXPECT().
		GetSegments(ctx, "workspace1", false).
		Do(func(context.Context, string, bool) { cancel() }).
		Return([]*domain.Segment{segment}, nil)

	count, err := processor.ProcessQueue(ctx, "workspace1")
	assert.NoError(t, err)
	assert.Equal(t, 0, count, "no contact may be counted processed after cancellation")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContactSegmentQueueProcessor_ProcessContact_NoSegments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()
	db, _, _ := sqlmock.New()
	defer func() { _ = db.Close() }()

	err := processor.processContact(ctx, "workspace1", db, "test@test.com", []*domain.Segment{})
	assert.NoError(t, err)
}

func TestContactSegmentQueueProcessor_ProcessContact_NoGeneratedSQL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()
	db, _, _ := sqlmock.New()
	defer func() { _ = db.Close() }()

	segment := &domain.Segment{
		ID:           "segment1",
		Name:         "Test Segment",
		Status:       string(domain.SegmentStatusActive),
		Version:      1,
		GeneratedSQL: nil, // No SQL
	}

	err := processor.processContact(ctx, "workspace1", db, "test@test.com", []*domain.Segment{segment})
	assert.NoError(t, err)
}

func TestContactSegmentQueueProcessor_RebindPlaceholders(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	tests := []struct {
		name     string
		input    string
		offset   int
		expected string
	}{
		{
			name:     "rebind from 1",
			input:    "SELECT * FROM contacts WHERE field = $1 AND other = $2",
			offset:   1,
			expected: "SELECT * FROM contacts WHERE field = $1 AND other = $2",
		},
		{
			name:     "rebind from 5",
			input:    "SELECT * FROM contacts WHERE field = $1 AND other = $2",
			offset:   5,
			expected: "SELECT * FROM contacts WHERE field = $5 AND other = $6",
		},
		{
			name:     "no placeholders",
			input:    "SELECT * FROM contacts",
			offset:   10,
			expected: "SELECT * FROM contacts",
		},
		{
			// Offset 1 is a no-op shift, so a placeholder keeps its own number. This case
			// previously expected "$1": the function renumbered placeholders by the order they
			// appeared instead of shifting each by its own value.
			name:     "multiple digit placeholder",
			input:    "WHERE field = $10",
			offset:   1,
			expected: "WHERE field = $10",
		},
		{
			name:     "multiple digit placeholder with a real shift",
			input:    "WHERE a = $9 AND b = $10",
			offset:   3,
			expected: "WHERE a = $11 AND b = $12",
		},
		{
			// The query builder binds a JSONB key once and references it in two comparisons
			// (multi-value contains, NULL-inclusive relative dates). Renumbering by occurrence
			// turned the second reference into a new placeholder, so the query asked for one
			// more argument than was supplied and failed to bind — killing segment evaluation
			// for the contact, since every segment is checked in a single UNION.
			name:     "repeated placeholder keeps pointing at the same argument",
			input:    "WHERE (c->$2->>'n' ILIKE $3 OR c->$2->>'n' ILIKE $4) AND kind = $1",
			offset:   5,
			expected: "WHERE (c->$6->>'n' ILIKE $7 OR c->$6->>'n' ILIKE $8) AND kind = $5",
		},
		{
			name:     "placeholders out of ascending order",
			input:    "WHERE b = $2 AND a = $1",
			offset:   1,
			expected: "WHERE b = $2 AND a = $1",
		},
		{
			name:     "dollar sign that is not a placeholder is left alone",
			input:    "WHERE price = 'US$ 5' AND a = $1",
			offset:   2,
			expected: "WHERE price = 'US$ 5' AND a = $2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.rebindPlaceholders(tt.input, tt.offset)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContactSegmentQueueProcessor_GetQueueSize(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mockQueueRepo.EXPECT().
			GetQueueSize(ctx, "workspace1").
			Return(42, nil)

		size, err := processor.GetQueueSize(ctx, "workspace1")
		assert.NoError(t, err)
		assert.Equal(t, 42, size)
	})

	t.Run("error", func(t *testing.T) {
		mockQueueRepo.EXPECT().
			GetQueueSize(ctx, "workspace1").
			Return(0, errors.New("db error"))

		size, err := processor.GetQueueSize(ctx, "workspace1")
		assert.Error(t, err)
		assert.Equal(t, 0, size)
	})
}

// TestContactSegmentQueueProcessor_ClaimBatch_Error verifies claimBatch surfaces
// a query error and rolls the claim transaction back (the rows survive).
func TestContactSegmentQueueProcessor_ClaimBatch_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery("DELETE FROM contact_segment_queue").
		WillReturnError(errors.New("query error"))
	mock.ExpectRollback()

	emails, err := processor.claimBatch(ctx, db, 100)
	assert.Error(t, err)
	assert.Nil(t, emails)
	assert.Contains(t, err.Error(), "failed to claim pending emails")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestContactSegmentQueueProcessor_ClaimBatch_CommitError verifies that rows are
// NOT reported claimed when the commit fails — the database kept them, so
// reporting them claimed would double-process nothing but dropping them would
// be wrong.
func TestContactSegmentQueueProcessor_ClaimBatch_CommitError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	rows := sqlmock.NewRows([]string{"email"}).AddRow("test@test.com")
	mock.ExpectQuery("DELETE FROM contact_segment_queue").WillReturnRows(rows)
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))

	emails, err := processor.claimBatch(ctx, db, 100)
	assert.Error(t, err)
	assert.Nil(t, emails)
	assert.Contains(t, err.Error(), "failed to claim pending emails")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestContactSegmentQueueProcessor_ClaimBatch_DetachedFromCancel verifies the
// claim is detached from the caller's cancellation. This is a genuine
// regression test for the pre-flight path: database/sql rejects an
// already-cancelled context before reaching the driver, so without the
// WithoutCancel detachment this fails deterministically. (The mid-scan
// cancel-vs-commit race itself is not reproducible under sqlmock; the short
// transaction covers it by construction.)
func TestContactSegmentQueueProcessor_ClaimBatch_DetachedFromCancel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before the claim starts

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	rows := sqlmock.NewRows([]string{"email"}).AddRow("test@test.com")
	mock.ExpectQuery("DELETE FROM contact_segment_queue").WillReturnRows(rows)
	mock.ExpectCommit()

	emails, err := processor.claimBatch(ctx, db, 100)
	assert.NoError(t, err)
	assert.Equal(t, []string{"test@test.com"}, emails)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestContactSegmentQueueProcessor_RequeueBatch_EmptyList verifies requeueBatch is
// a no-op (and issues no query) for an empty list.
func TestContactSegmentQueueProcessor_RequeueBatch_EmptyList(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	// No expectations: an empty list must not touch the database.
	err = processor.requeueBatch(ctx, db, []string{})
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestContactSegmentQueueProcessor_RequeueBatch_Success verifies requeueBatch
// re-inserts all emails in one ON CONFLICT upsert.
func TestContactSegmentQueueProcessor_RequeueBatch_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectExec("INSERT INTO contact_segment_queue").
		WithArgs("a@test.com", "b@test.com").
		WillReturnResult(sqlmock.NewResult(0, 2))

	err = processor.requeueBatch(ctx, db, []string{"a@test.com", "b@test.com"})
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestContactSegmentQueueProcessor_RequeueBatch_Dedup verifies duplicate emails
// are collapsed before the multi-row upsert: a repeated key in one
// ON CONFLICT (email) DO UPDATE statement is a Postgres error ("cannot affect
// row a second time") that would fail the whole re-enqueue. The statement is
// pinned to exactly two value tuples.
func TestContactSegmentQueueProcessor_RequeueBatch_Dedup(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockContactSegmentQueueRepository(ctrl)
	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewContactSegmentQueueProcessor(
		mockQueueRepo,
		mockSegmentRepo,
		mockContactRepo,
		mockWorkspaceRepo,
		mockLogger,
	)

	ctx := context.Background()

	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Exactly two tuples: the duplicate "a@test.com" must be collapsed.
	mock.ExpectExec(`INSERT INTO contact_segment_queue[\s\S]*\(\$1, NOW\(\)\), \(\$2, NOW\(\)\)`).
		WithArgs("a@test.com", "b@test.com").
		WillReturnResult(sqlmock.NewResult(0, 2))

	err = processor.requeueBatch(ctx, db, []string{"a@test.com", "a@test.com", "b@test.com"})
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
