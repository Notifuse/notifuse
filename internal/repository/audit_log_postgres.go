package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/lib/pq"

	"github.com/Notifuse/notifuse/internal/domain"
)

// auditLogRepository stores the audit log in the SYSTEM database. Rows are
// written once and read many times; the only deletes go through the SQL purge
// functions, because the guard trigger refuses every other one (see
// internal/database/schema/audit_logs_tables.go).
type auditLogRepository struct {
	db *sql.DB
}

// NewAuditLogRepository takes the system database, like the user and setting
// repositories do.
func NewAuditLogRepository(db *sql.DB) domain.AuditLogRepository {
	return &auditLogRepository{db: db}
}

// auditStreamPageSize is the keyset page an export walks with. Larger than
// the list cap because nobody renders it.
const auditStreamPageSize = 1000

var auditLogColumns = []string{
	"id", "occurred_at", "workspace_id", "action", "category", "outcome", "status_code",
	"actor_type", "actor_id", "actor_email", "actor_name", "actor_role", "auth_method",
	"target_type", "target_id", "target_name", "ip_address", "user_agent", "request_id",
	"changes", "metadata",
}

// Column widths, so an oversized value — a padded X-Forwarded-For header, an
// absurd target id — is cut to fit rather than failing the INSERT. A row with
// a truncated field beats no row: the write path is fail-open, and "make my
// audit entry fail to insert" must not be something a caller can arrange.
const (
	auditIDWidth      = 32
	auditActionWidth  = 100
	auditShortWidth   = 20
	auditCategoryW    = 50
	auditActorIDWidth = 64
	auditNameWidth    = 255
	auditIPWidth      = 45
	auditRequestWidth = 64
)

func clampRunes(value string, max int) string {
	if len(value) <= max {
		return value
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func (r *auditLogRepository) Insert(ctx context.Context, log *domain.AuditLog) error {
	changes, err := auditJSON(log.Changes)
	if err != nil {
		return fmt.Errorf("failed to encode audit changes: %w", err)
	}
	metadata, err := auditJSON(log.Metadata)
	if err != nil {
		return fmt.Errorf("failed to encode audit metadata: %w", err)
	}

	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	query := psql.Insert("audit_logs").Columns(auditLogColumns...).Values(
		clampRunes(log.ID, auditIDWidth),
		log.OccurredAt.UTC(),
		nullableString(clampRunes(log.WorkspaceID, auditIDWidth)),
		clampRunes(log.Action, auditActionWidth),
		clampRunes(log.Category, auditCategoryW),
		clampRunes(log.Outcome, auditShortWidth),
		nullableInt(log.StatusCode),
		clampRunes(log.ActorType, auditShortWidth),
		nullableString(clampRunes(log.ActorID, auditActorIDWidth)),
		nullableString(clampRunes(log.ActorEmail, auditNameWidth)),
		nullableString(clampRunes(log.ActorName, auditNameWidth)),
		nullableString(clampRunes(log.ActorRole, auditShortWidth)),
		nullableString(clampRunes(log.AuthMethod, auditShortWidth)),
		nullableString(clampRunes(log.TargetType, auditCategoryW)),
		nullableString(clampRunes(log.TargetID, auditNameWidth)),
		nullableString(clampRunes(log.TargetName, auditNameWidth)),
		nullableString(clampRunes(log.IPAddress, auditIPWidth)),
		nullableString(log.UserAgent),
		nullableString(clampRunes(log.RequestID, auditRequestWidth)),
		changes,
		metadata,
	)

	sqlStr, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build audit insert: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, sqlStr, args...); err != nil {
		return fmt.Errorf("failed to insert audit log: %w", err)
	}
	return nil
}

func (r *auditLogRepository) List(ctx context.Context, filter domain.AuditLogFilter) ([]*domain.AuditLog, string, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = domain.AuditLogDefaultListLimit
	}
	return r.page(ctx, filter, limit)
}

func (r *auditLogRepository) GetByID(ctx context.Context, id string) (*domain.AuditLog, error) {
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	sqlStr, args, err := psql.Select(auditLogColumns...).From("audit_logs").Where(sq.Eq{"id": id}).ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build audit query: %w", err)
	}
	log, err := scanAuditLog(r.db.QueryRowContext(ctx, sqlStr, args...))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", domain.ErrAuditLogNotFound, id)
		}
		return nil, fmt.Errorf("failed to get audit log: %w", err)
	}
	return log, nil
}

// Stream walks the selection newest first in keyset pages, so an export of a
// million rows never holds more than one page in memory. fn's error is
// returned as-is: the service uses a sentinel to stop at the export cap.
func (r *auditLogRepository) Stream(ctx context.Context, filter domain.AuditLogFilter, fn func(*domain.AuditLog) error) error {
	page := filter
	page.Cursor = ""
	for {
		logs, next, err := r.page(ctx, page, auditStreamPageSize)
		if err != nil {
			return err
		}
		for _, log := range logs {
			if err := fn(log); err != nil {
				return err
			}
		}
		if next == "" {
			return nil
		}
		page.Cursor = next
	}
}

func (r *auditLogRepository) Purge(ctx context.Context, workspaceID *string, before time.Time) (int64, error) {
	var scope sql.NullString
	if workspaceID != nil {
		scope = sql.NullString{String: *workspaceID, Valid: true}
	}
	var deleted int64
	if err := r.db.QueryRowContext(ctx, `SELECT audit_logs_purge($1, $2)`, scope, before.UTC()).Scan(&deleted); err != nil {
		return 0, fmt.Errorf("failed to purge audit logs: %w", err)
	}
	return deleted, nil
}

func (r *auditLogRepository) PurgeOrphans(ctx context.Context, keep []string, before time.Time) (int64, error) {
	var deleted int64
	if err := r.db.QueryRowContext(ctx, `SELECT audit_logs_purge_orphans($1, $2)`, pq.Array(keep), before.UTC()).Scan(&deleted); err != nil {
		return 0, fmt.Errorf("failed to purge orphaned audit logs: %w", err)
	}
	return deleted, nil
}

// page runs one keyset page of limit rows and returns the cursor of the next
// one, empty at the end. It fetches limit+1 to learn whether there is a next
// page without a second query.
func (r *auditLogRepository) page(ctx context.Context, filter domain.AuditLogFilter, limit int) ([]*domain.AuditLog, string, error) {
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	query := psql.Select(auditLogColumns...).From("audit_logs")

	// The scope is the baseline: a workspace sees its rows, the deployment view
	// sees everything unless it asks for one workspace.
	if filter.Scope == domain.AuditScopeDeployment {
		if filter.WorkspaceID != "" {
			query = query.Where(sq.Eq{"workspace_id": filter.WorkspaceID})
		}
	} else {
		query = query.Where(sq.Eq{"workspace_id": filter.WorkspaceID})
	}

	if len(filter.Actions) > 0 {
		query = query.Where(sq.Eq{"action": filter.Actions})
	}
	if len(filter.Categories) > 0 {
		query = query.Where(sq.Eq{"category": filter.Categories})
	}
	if filter.ActorID != "" {
		query = query.Where(sq.Eq{"actor_id": filter.ActorID})
	}
	if filter.ActorEmail != "" {
		query = query.Where(sq.ILike{"actor_email": "%" + filter.ActorEmail + "%"})
	}
	if filter.ActorType != "" {
		query = query.Where(sq.Eq{"actor_type": filter.ActorType})
	}
	if filter.Outcome != "" {
		query = query.Where(sq.Eq{"outcome": filter.Outcome})
	}
	if filter.TargetType != "" {
		query = query.Where(sq.Eq{"target_type": filter.TargetType})
	}
	if filter.TargetID != "" {
		query = query.Where(sq.Eq{"target_id": filter.TargetID})
	}
	if filter.IPAddress != "" {
		query = query.Where(sq.Eq{"ip_address": filter.IPAddress})
	}
	if filter.From != nil {
		query = query.Where(sq.GtOrEq{"occurred_at": filter.From.UTC()})
	}
	if filter.To != nil {
		query = query.Where(sq.LtOrEq{"occurred_at": filter.To.UTC()})
	}
	if filter.Cursor != "" {
		cursorAt, cursorID, err := domain.DecodeAuditCursor(filter.Cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		query = query.Where(sq.Or{
			sq.Lt{"occurred_at": cursorAt},
			sq.And{sq.Eq{"occurred_at": cursorAt}, sq.Lt{"id": cursorID}},
		})
	}

	query = query.OrderBy("occurred_at DESC", "id DESC").Limit(uint64(limit + 1))

	sqlStr, args, err := query.ToSql()
	if err != nil {
		return nil, "", fmt.Errorf("failed to build audit query: %w", err)
	}
	rows, err := r.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list audit logs: %w", err)
	}
	defer rows.Close()

	// Non-nil so the list endpoint serialises [] rather than null.
	logs := make([]*domain.AuditLog, 0, limit)
	for rows.Next() {
		log, err := scanAuditLog(rows)
		if err != nil {
			return nil, "", fmt.Errorf("failed to scan audit log: %w", err)
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("failed to read audit logs: %w", err)
	}

	nextCursor := ""
	if len(logs) > limit {
		logs = logs[:limit]
		last := logs[len(logs)-1]
		nextCursor = domain.EncodeAuditCursor(last.OccurredAt, last.ID)
	}
	return logs, nextCursor, nil
}

type auditScanner interface {
	Scan(dest ...any) error
}

func scanAuditLog(row auditScanner) (*domain.AuditLog, error) {
	var (
		log         domain.AuditLog
		workspaceID sql.NullString
		statusCode  sql.NullInt64
		actorID     sql.NullString
		actorEmail  sql.NullString
		actorName   sql.NullString
		actorRole   sql.NullString
		authMethod  sql.NullString
		targetType  sql.NullString
		targetID    sql.NullString
		targetName  sql.NullString
		ipAddress   sql.NullString
		userAgent   sql.NullString
		requestID   sql.NullString
		changes     []byte
		metadata    []byte
	)
	if err := row.Scan(
		&log.ID, &log.OccurredAt, &workspaceID, &log.Action, &log.Category, &log.Outcome, &statusCode,
		&log.ActorType, &actorID, &actorEmail, &actorName, &actorRole, &authMethod,
		&targetType, &targetID, &targetName, &ipAddress, &userAgent, &requestID,
		&changes, &metadata,
	); err != nil {
		return nil, err
	}
	log.WorkspaceID = workspaceID.String
	log.StatusCode = int(statusCode.Int64)
	log.ActorID = actorID.String
	log.ActorEmail = actorEmail.String
	log.ActorName = actorName.String
	log.ActorRole = actorRole.String
	log.AuthMethod = authMethod.String
	log.TargetType = targetType.String
	log.TargetID = targetID.String
	log.TargetName = targetName.String
	log.IPAddress = ipAddress.String
	log.UserAgent = userAgent.String
	log.RequestID = requestID.String
	if len(changes) > 0 {
		if err := json.Unmarshal(changes, &log.Changes); err != nil {
			return nil, fmt.Errorf("failed to decode audit changes: %w", err)
		}
	}
	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &log.Metadata); err != nil {
			return nil, fmt.Errorf("failed to decode audit metadata: %w", err)
		}
	}
	return &log, nil
}

// auditJSON encodes a map for a JSONB column, NULL when empty.
func auditJSON(value any) (any, error) {
	switch typed := value.(type) {
	case map[string]domain.AuditChange:
		if len(typed) == 0 {
			return nil, nil
		}
	case map[string]any:
		if len(typed) == 0 {
			return nil, nil
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}
