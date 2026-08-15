package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"go.opencensus.io/trace"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/tracing"
)

type federatedIdentityRepository struct {
	systemDB *sql.DB
}

// NewFederatedIdentityRepository creates a new PostgreSQL federated identity repository.
func NewFederatedIdentityRepository(db *sql.DB) domain.FederatedIdentityRepository {
	return &federatedIdentityRepository{systemDB: db}
}

const federatedIdentitySelectCols = `id, user_id, idp_issuer, idp_sub, created_at`

func scanFederatedIdentity(row interface {
	Scan(dest ...interface{}) error
}) (*domain.FederatedIdentity, error) {
	var fi domain.FederatedIdentity
	if err := row.Scan(&fi.ID, &fi.UserID, &fi.IDPIssuer, &fi.IDPSub, &fi.CreatedAt); err != nil {
		return nil, err
	}
	return &fi, nil
}

// GetByIssuerSubject returns the identity for (issuer, sub). Served by
// UNIQUE(idp_issuer, idp_sub).
func (r *federatedIdentityRepository) GetByIssuerSubject(ctx context.Context, issuer, sub string) (*domain.FederatedIdentity, error) {
	ctx, span := tracing.StartServiceSpan(ctx, "FederatedIdentityRepository", "GetByIssuerSubject")
	defer span.End()

	query := `SELECT ` + federatedIdentitySelectCols + ` FROM federated_identities WHERE idp_issuer=$1 AND idp_sub=$2`
	fi, err := scanFederatedIdentity(r.systemDB.QueryRowContext(ctx, query, issuer, sub))
	if err == sql.ErrNoRows {
		return nil, &domain.ErrFederatedIdentityNotFound{Message: "federated identity not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get federated identity by issuer/subject: %w", err)
	}
	return fi, nil
}

// GetByUserAndIssuer returns the link for (user_id, issuer). Deterministic (0-or-1)
// via UNIQUE(user_id, idp_issuer).
func (r *federatedIdentityRepository) GetByUserAndIssuer(ctx context.Context, userID, issuer string) (*domain.FederatedIdentity, error) {
	ctx, span := tracing.StartServiceSpan(ctx, "FederatedIdentityRepository", "GetByUserAndIssuer")
	defer span.End()

	query := `SELECT ` + federatedIdentitySelectCols + ` FROM federated_identities WHERE user_id=$1 AND idp_issuer=$2`
	fi, err := scanFederatedIdentity(r.systemDB.QueryRowContext(ctx, query, userID, issuer))
	if err == sql.ErrNoRows {
		return nil, &domain.ErrFederatedIdentityNotFound{Message: "federated identity not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get federated identity by user/issuer: %w", err)
	}
	return fi, nil
}

// Create inserts a new federated identity link. A UNIQUE violation on either
// constraint maps to *ErrFederatedIdentityExists.
func (r *federatedIdentityRepository) Create(ctx context.Context, fi *domain.FederatedIdentity) error {
	ctx, span := tracing.StartServiceSpan(ctx, "FederatedIdentityRepository", "Create")
	defer span.End()

	if fi.ID == "" {
		fi.ID = uuid.New().String()
	}
	if fi.CreatedAt.IsZero() {
		fi.CreatedAt = time.Now().UTC()
	}
	span.AddAttributes(
		trace.StringAttribute("oidc.issuer", fi.IDPIssuer),
		trace.StringAttribute("user.id", fi.UserID),
	)

	query := `INSERT INTO federated_identities (id, user_id, idp_issuer, idp_sub, created_at) VALUES ($1, $2, $3, $4, $5)`
	_, err := r.systemDB.ExecContext(ctx, query, fi.ID, fi.UserID, fi.IDPIssuer, fi.IDPSub, fi.CreatedAt)
	if err != nil {
		// Match the SQLSTATE, never the message. PostgreSQL translates error text per
		// lc_messages, so a non-English server renders this same 23505 in its own locale
		// and no text match survives it. The INSERT fails either way, so the takeover
		// guard in OIDCService.linkIdentity still holds; what a missed match costs is the
		// diagnosis - the caller can no longer tell a conflict from an outage, so a
		// genuine same-link race stops resolving as an idempotent success and the user
		// is sent to ?oidc_error=auth_failed, told to retry what cannot succeed.
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return &domain.ErrFederatedIdentityExists{Message: "federated identity already exists"}
		}
		return fmt.Errorf("failed to create federated identity: %w", err)
	}
	return nil
}

// ListByUserID returns all federated identities for a user, oldest first. Uses
// idx_federated_identities_user_id.
func (r *federatedIdentityRepository) ListByUserID(ctx context.Context, userID string) ([]*domain.FederatedIdentity, error) {
	ctx, span := tracing.StartServiceSpan(ctx, "FederatedIdentityRepository", "ListByUserID")
	defer span.End()

	query := `SELECT ` + federatedIdentitySelectCols + ` FROM federated_identities WHERE user_id=$1 ORDER BY created_at`
	rows, err := r.systemDB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list federated identities: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*domain.FederatedIdentity
	for rows.Next() {
		fi, err := scanFederatedIdentity(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan federated identity: %w", err)
		}
		out = append(out, fi)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating federated identities: %w", err)
	}
	return out, nil
}
