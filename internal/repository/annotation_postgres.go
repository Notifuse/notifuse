package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/Notifuse/notifuse/internal/domain"
)

// annotationColumns is the projection shared by List and Get, in the order
// scanAnnotation reads them.
var annotationColumns = []string{
	"id",
	"annotated_at",
	"timezone",
	"title",
	"description",
	"color",
	"source",
	"source_id",
	"created_at",
	"updated_at",
}

type annotationRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewAnnotationRepository returns the workspace-database persistence for
// annotations. The table lives in each workspace database, so every method
// resolves its connection through the workspace repository.
func NewAnnotationRepository(workspaceRepo domain.WorkspaceRepository) domain.AnnotationRepository {
	return &annotationRepository{
		workspaceRepo: workspaceRepo,
	}
}

func (r *annotationRepository) List(ctx context.Context, workspaceID string, filter domain.AnnotationFilter) ([]*domain.Annotation, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	query := psql.Select(annotationColumns...).From("annotations")

	if filter.Start != nil {
		query = query.Where(sq.GtOrEq{"annotated_at": *filter.Start})
	}
	if filter.End != nil {
		query = query.Where(sq.LtOrEq{"annotated_at": *filter.End})
	}
	if len(filter.Sources) > 0 {
		query = query.Where(sq.Eq{"source": filter.Sources})
	}

	// An unset limit means "the page the console asks for by default", not "every
	// annotation ever written": the range filter is optional, so an unbounded
	// query is one request away.
	limit := filter.Limit
	if limit <= 0 {
		limit = domain.AnnotationDefaultListLimit
	}
	query = query.OrderBy("annotated_at DESC").Limit(uint64(limit))

	sqlStr, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build annotations query: %w", err)
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list annotations: %w", err)
	}
	defer rows.Close()

	// Non-nil so the list endpoint serialises [] rather than null.
	annotations := make([]*domain.Annotation, 0)
	for rows.Next() {
		annotation, err := scanAnnotation(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan annotation: %w", err)
		}
		annotations = append(annotations, annotation)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating annotations: %w", err)
	}

	return annotations, nil
}

func (r *annotationRepository) Get(ctx context.Context, workspaceID, id string) (*domain.Annotation, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	sqlStr, args, err := psql.Select(annotationColumns...).
		From("annotations").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build annotation query: %w", err)
	}

	annotation, err := scanAnnotation(db.QueryRowContext(ctx, sqlStr, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &domain.ErrNotFound{Entity: "annotation", ID: id}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get annotation: %w", err)
	}

	return annotation, nil
}

func (r *annotationRepository) Create(ctx context.Context, workspaceID string, annotation *domain.Annotation) error {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	stampAnnotationTimes(annotation)

	query := `
		INSERT INTO annotations (
			id, annotated_at, timezone, title, description,
			color, source, source_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err = db.ExecContext(ctx, query,
		annotation.ID,
		annotation.AnnotatedAt,
		annotation.Timezone,
		annotation.Title,
		annotation.Description,
		annotation.Color,
		annotation.Source,
		annotation.SourceID,
		annotation.CreatedAt,
		annotation.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create annotation: %w", err)
	}

	return nil
}

// CreateFromSource inserts an automatic annotation and reports whether the row
// was new. It is the only write that may collide: a task retry, a redeploy
// mid-run or two dispatchers racing all try to annotate the same broadcast.
//
// The ON CONFLICT target repeats the index predicate — WHERE source_id IS NOT
// NULL — because idx_annotations_source is a *partial* unique index and
// PostgreSQL will not infer a partial index as an arbiter without it. Dropping
// the predicate raises 42P10 ("no unique or exclusion constraint matching the
// ON CONFLICT specification") at runtime, not at compile time. Do not replace
// it with a target-less ON CONFLICT DO NOTHING either: that would also swallow
// a genuine primary-key collision, hiding an id generator gone wrong.
func (r *annotationRepository) CreateFromSource(ctx context.Context, workspaceID string, annotation *domain.Annotation) (bool, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	stampAnnotationTimes(annotation)

	query := `
		INSERT INTO annotations (
			id, annotated_at, timezone, title, description,
			color, source, source_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (source, source_id) WHERE source_id IS NOT NULL DO NOTHING
	`

	result, err := db.ExecContext(ctx, query,
		annotation.ID,
		annotation.AnnotatedAt,
		annotation.Timezone,
		annotation.Title,
		annotation.Description,
		annotation.Color,
		annotation.Source,
		annotation.SourceID,
		annotation.CreatedAt,
		annotation.UpdatedAt,
	)
	if err != nil {
		return false, fmt.Errorf("failed to create annotation from source: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected > 0, nil
}

// Update writes only the operator-editable fields. source and source_id are
// deliberately absent: an edit must not be able to turn a manual row into a
// broadcast one, nor steal another broadcast's idempotency slot.
func (r *annotationRepository) Update(ctx context.Context, workspaceID string, annotation *domain.Annotation) error {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	annotation.UpdatedAt = time.Now().UTC()

	query := `
		UPDATE annotations SET
			annotated_at = $1,
			timezone = $2,
			title = $3,
			description = $4,
			color = $5,
			updated_at = $6
		WHERE id = $7
	`

	result, err := db.ExecContext(ctx, query,
		annotation.AnnotatedAt,
		annotation.Timezone,
		annotation.Title,
		annotation.Description,
		annotation.Color,
		annotation.UpdatedAt,
		annotation.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update annotation: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &domain.ErrNotFound{Entity: "annotation", ID: annotation.ID}
	}

	return nil
}

func (r *annotationRepository) Delete(ctx context.Context, workspaceID, id string) error {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	result, err := db.ExecContext(ctx, `DELETE FROM annotations WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete annotation: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	// Deleting nothing is a miss, not a silent success: the console needs a 404
	// to tell the operator the row is already gone.
	if rowsAffected == 0 {
		return &domain.ErrNotFound{Entity: "annotation", ID: id}
	}

	return nil
}

// stampAnnotationTimes fills the audit timestamps the caller left unset, so a
// service that only cares about the annotated instant still writes sane rows,
// while a backfill or a test can pin them explicitly.
func stampAnnotationTimes(annotation *domain.Annotation) {
	now := time.Now().UTC()
	if annotation.CreatedAt.IsZero() {
		annotation.CreatedAt = now
	}
	if annotation.UpdatedAt.IsZero() {
		annotation.UpdatedAt = now
	}
}

// annotationScanner is satisfied by both *sql.Row and *sql.Rows, so Get and List
// share one column ordering.
type annotationScanner interface {
	Scan(dest ...interface{}) error
}

func scanAnnotation(scanner annotationScanner) (*domain.Annotation, error) {
	var annotation domain.Annotation
	var sourceID sql.NullString

	if err := scanner.Scan(
		&annotation.ID,
		&annotation.AnnotatedAt,
		&annotation.Timezone,
		&annotation.Title,
		&annotation.Description,
		&annotation.Color,
		&annotation.Source,
		&sourceID,
		&annotation.CreatedAt,
		&annotation.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if sourceID.Valid {
		annotation.SourceID = &sourceID.String
	}

	return &annotation, nil
}
