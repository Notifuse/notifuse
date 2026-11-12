package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// Every query compares the bare contact_email column, so the partial indexes
// idx_web_sessions_contact_email, idx_web_pages_contact_email and idx_web_goals_contact_email
// serve it. The pages route reads by primary key once the session is known to be the
// contact's.

type webAnalyticsContactVisitsRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewWebAnalyticsContactVisitsRepository creates the repository of a contact's web visits.
func NewWebAnalyticsContactVisitsRepository(workspaceRepo domain.WorkspaceRepository) domain.WebAnalyticsContactVisitsRepository {
	return &webAnalyticsContactVisitsRepository{workspaceRepo: workspaceRepo}
}

// contactVisitSessionSelect reads the columns of domain.ContactVisitSession. The first goal
// is read by the session's key rather than by contact_email: a goal reached in a tab before
// the visitor was identified carries no email but belongs to the session all the same.
const contactVisitSessionSelect = `SELECT s.id, s.session_date, s.created_at, s.duration_ms, s.pageview_count,
	s.landing_path, s.exit_path, s.channel, s.channel_group, s.referrer_domain, s.utm_source, s.utm_campaign,
	s.device, s.os, s.browser, s.country, s.goal_count, s.goal_value,
	(SELECT g.goal_name FROM web_goals g
	 WHERE g.session_date = s.session_date AND g.session_id = s.id
	 ORDER BY g.goal_at, g.tab_id, g.client_ts_ms, g.goal_name LIMIT 1) AS first_goal_name
	FROM web_sessions s
	WHERE s.contact_email = $1`

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanContactVisitSession(row rowScanner) (domain.ContactVisitSession, error) {
	var s domain.ContactVisitSession
	var sessionDate time.Time
	var firstGoal sql.NullString
	err := row.Scan(&s.ID, &sessionDate, &s.CreatedAt, &s.DurationMs, &s.PageviewCount,
		&s.LandingPath, &s.ExitPath, &s.Channel, &s.ChannelGroup, &s.ReferrerDomain, &s.UTMSource, &s.UTMCampaign,
		&s.Device, &s.OS, &s.Browser, &s.Country, &s.GoalCount, &s.GoalValue, &firstGoal)
	if err != nil {
		return s, err
	}
	s.SessionDate = sessionDate.Format(time.DateOnly)
	if firstGoal.Valid {
		s.FirstGoalName = &firstGoal.String
	}
	return s, nil
}

func (r *webAnalyticsContactVisitsRepository) ListContactSessions(ctx context.Context, workspaceID string, email string, filter string, limit int, cursor string) ([]domain.ContactVisitSession, string, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := contactVisitSessionSelect
	args := []interface{}{email}
	if cursor != "" {
		createdAt, id, err := domain.DecodeContactVisitsCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		query += ` AND (s.created_at < $2 OR (s.created_at = $2 AND s.id < $3))`
		args = append(args, createdAt, id)
	}
	switch filter {
	case domain.ContactVisitsFilterGoal:
		query += ` AND s.goal_count > 0`
	case domain.ContactVisitsFilterEmail:
		query += ` AND s.channel_group = 'email'`
	}
	query += fmt.Sprintf(` ORDER BY s.created_at DESC, s.id DESC LIMIT $%d`, len(args)+1)
	args = append(args, limit+1)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list contact sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	sessions := []domain.ContactVisitSession{}
	for rows.Next() {
		s, err := scanContactVisitSession(rows)
		if err != nil {
			return nil, "", fmt.Errorf("failed to scan contact session: %w", err)
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("failed to read contact sessions: %w", err)
	}

	next := ""
	if len(sessions) > limit {
		sessions = sessions[:limit]
		last := sessions[limit-1]
		next = domain.EncodeContactVisitsCursor(last.CreatedAt, last.ID)
	}
	return sessions, next, nil
}

func (r *webAnalyticsContactVisitsRepository) ContactVisitsSummary(ctx context.Context, workspaceID string, email string) (*domain.ContactVisitsSummary, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	summary := &domain.ContactVisitsSummary{Goals: []domain.ContactVisitGoal{}, TopPages: []domain.ContactVisitPage{}}
	err = db.QueryRowContext(ctx, `SELECT COUNT(*),
		COUNT(*) FILTER (WHERE goal_count > 0),
		COUNT(*) FILTER (WHERE channel_group = 'email'),
		COALESCE(ROUND(percentile_cont(0.5) WITHIN GROUP (ORDER BY duration_ms))::bigint, 0)
		FROM web_sessions WHERE contact_email = $1`, email).
		Scan(&summary.Sessions, &summary.GoalSessions, &summary.EmailSessions, &summary.MedianDurationMs)
	if err != nil {
		return nil, fmt.Errorf("failed to count contact sessions: %w", err)
	}

	first, err := scanContactVisitSession(db.QueryRowContext(ctx,
		contactVisitSessionSelect+` ORDER BY s.created_at ASC, s.id ASC LIMIT 1`, email))
	switch {
	case err == sql.ErrNoRows:
	case err != nil:
		return nil, fmt.Errorf("failed to read first contact session: %w", err)
	default:
		summary.FirstSession = &first
	}

	goals, err := db.QueryContext(ctx, `SELECT goal_name, COUNT(*), COALESCE(SUM(goal_value), 0)
		FROM web_goals WHERE contact_email = $1
		GROUP BY goal_name ORDER BY COUNT(*) DESC, goal_name LIMIT $2`, email, domain.ContactVisitsSummaryGoals)
	if err != nil {
		return nil, fmt.Errorf("failed to summarize contact goals: %w", err)
	}
	defer func() { _ = goals.Close() }()
	for goals.Next() {
		var g domain.ContactVisitGoal
		if err := goals.Scan(&g.GoalName, &g.Count, &g.Value); err != nil {
			return nil, fmt.Errorf("failed to scan contact goal: %w", err)
		}
		summary.Goals = append(summary.Goals, g)
	}
	if err := goals.Err(); err != nil {
		return nil, fmt.Errorf("failed to read contact goals: %w", err)
	}

	pages, err := db.QueryContext(ctx, `SELECT path, COUNT(*)
		FROM web_pages WHERE contact_email = $1
		GROUP BY path ORDER BY COUNT(*) DESC, path LIMIT $2`, email, domain.ContactVisitsSummaryTopPages)
	if err != nil {
		return nil, fmt.Errorf("failed to summarize contact pages: %w", err)
	}
	defer func() { _ = pages.Close() }()
	for pages.Next() {
		var p domain.ContactVisitPage
		if err := pages.Scan(&p.Path, &p.Views); err != nil {
			return nil, fmt.Errorf("failed to scan contact page: %w", err)
		}
		summary.TopPages = append(summary.TopPages, p)
	}
	if err := pages.Err(); err != nil {
		return nil, fmt.Errorf("failed to read contact pages: %w", err)
	}

	return summary, nil
}

func (r *webAnalyticsContactVisitsRepository) ContactSessionPages(ctx context.Context, workspaceID string, email string, sessionDate string, sessionID string) (*domain.ContactSessionPagesResponse, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	result := &domain.ContactSessionPagesResponse{Pages: []domain.ContactSessionPage{}, Goals: []domain.ContactSessionGoal{}}

	// The session must be the contact's: a member cannot read another visitor's session by
	// changing the ID. Its pages and goals are then read by key, so a page written by a tab
	// before the visitor was identified, which carries no email, is not lost.
	var owned bool
	if err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM web_sessions WHERE session_date = $1 AND id = $2 AND contact_email = $3)`,
		sessionDate, sessionID, email).Scan(&owned); err != nil {
		return nil, fmt.Errorf("failed to check contact session: %w", err)
	}
	if !owned {
		return result, nil
	}

	pages, err := db.QueryContext(ctx, `SELECT entered_at, path, duration_ms, page_number, tab_id
		FROM web_pages WHERE session_date = $1 AND session_id = $2
		ORDER BY entered_at, tab_id, page_number LIMIT $3`, sessionDate, sessionID, domain.ContactSessionPagesMax)
	if err != nil {
		return nil, fmt.Errorf("failed to list session pages: %w", err)
	}
	defer func() { _ = pages.Close() }()
	for pages.Next() {
		var p domain.ContactSessionPage
		if err := pages.Scan(&p.EnteredAt, &p.Path, &p.DurationMs, &p.PageNumber, &p.TabID); err != nil {
			return nil, fmt.Errorf("failed to scan session page: %w", err)
		}
		result.Pages = append(result.Pages, p)
	}
	if err := pages.Err(); err != nil {
		return nil, fmt.Errorf("failed to read session pages: %w", err)
	}

	goals, err := db.QueryContext(ctx, `SELECT goal_at, goal_name, goal_value, path, tab_id
		FROM web_goals WHERE session_date = $1 AND session_id = $2
		ORDER BY goal_at, tab_id, client_ts_ms, goal_name LIMIT $3`, sessionDate, sessionID, domain.ContactSessionPagesMax)
	if err != nil {
		return nil, fmt.Errorf("failed to list session goals: %w", err)
	}
	defer func() { _ = goals.Close() }()
	for goals.Next() {
		var g domain.ContactSessionGoal
		if err := goals.Scan(&g.GoalAt, &g.GoalName, &g.GoalValue, &g.Path, &g.TabID); err != nil {
			return nil, fmt.Errorf("failed to scan session goal: %w", err)
		}
		result.Goals = append(result.Goals, g)
	}
	if err := goals.Err(); err != nil {
		return nil, fmt.Errorf("failed to read session goals: %w", err)
	}

	return result, nil
}
