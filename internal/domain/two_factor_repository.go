package domain

import (
	"context"
	"database/sql"
	"time"
)

//go:generate mockgen -destination mocks/mock_two_factor_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain TwoFactorRepository,AuthTxRepository,InvitationTxRepository,SystemTxRunner

// DBExecutor runs queries on the system database, inside a transaction or not: *sql.DB and
// *sql.Tx both satisfy it.
type DBExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// SystemTxRunner runs a function in one READ COMMITTED transaction on the system database.
//
// The transaction commits when fn returns nil. When fn returns an error wrapped by
// CommitAndFail, it commits too, then returns the unwrapped error; if that commit fails,
// it returns the commit's error instead, never the business error. Any other error rolls
// back. A function that may return CommitAndFail writes nothing but its failure counters
// before checking the proof it was given.
type SystemTxRunner interface {
	WithSystemTx(ctx context.Context, fn func(tx DBExecutor) error) error
}

// UserTOTP is a user's authenticator app (user_totp). Secrets are sealed with SECRET_KEY
// and bound to the user (crypto.EncryptStringAAD with TOTPAAD).
type UserTOTP struct {
	UserID                 string
	SecretEncrypted        *string
	EnabledAt              *time.Time
	LastUsedStep           int64
	PendingSecretEncrypted *string
	PendingExpiresAt       *time.Time
	PendingAttempts        int
	FailedAttempts         int
	LockedUntil            *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// Enabled reports whether two-factor authentication is on. A row without enabled_at is a
// setup started and not confirmed.
func (t *UserTOTP) Enabled() bool {
	return t != nil && t.EnabledAt != nil && t.SecretEncrypted != nil
}

// TwoFactorRepository reads and writes user_totp and user_recovery_codes. Every method
// takes the executor to run on, so a service holds the locks of one transaction across
// several calls. Deadlines are compared in SQL or against the time ClockTimestamp read.
type TwoFactorRepository interface {
	// ClockTimestamp returns clock_timestamp(): the database's current time, the same on
	// every replica. Read it after the transaction's last lock.
	ClockTimestamp(ctx context.Context, db DBExecutor) (time.Time, error)
	// GetTOTP returns the user's row, nil when there is none.
	GetTOTP(ctx context.Context, db DBExecutor, userID string) (*UserTOTP, error)
	// GetTOTPForUpdate is GetTOTP with FOR UPDATE.
	GetTOTPForUpdate(ctx context.Context, db DBExecutor, userID string) (*UserTOTP, error)
	// SavePending stores a secret being set up, replacing any other, with a fresh attempt
	// count. It creates the row when there is none.
	SavePending(ctx context.Context, db DBExecutor, userID, sealedSecret string, expiresAt, now time.Time) error
	// RecordPendingFailure counts a wrong code against the secret being set up, and drops
	// that secret once it reached maxAttempts. It reports whether the secret was dropped.
	RecordPendingFailure(ctx context.Context, db DBExecutor, userID string, maxAttempts int, now time.Time) (bool, error)
	// Enable makes sealedSecret the active secret, accepted at step, clears the pending
	// secret and the failure counters, and sets enabled_at when it was not set.
	Enable(ctx context.Context, db DBExecutor, userID, sealedSecret string, step int64, now time.Time) error
	// RecordFailure stores the consecutive failure count and the wait it brings.
	RecordFailure(ctx context.Context, db DBExecutor, userID string, failedAttempts int, lockedUntil *time.Time, now time.Time) error
	// RecordSuccess clears the failure counters and, when step is not nil, records the
	// step just accepted.
	RecordSuccess(ctx context.Context, db DBExecutor, userID string, step *int64, now time.Time) error
	// DeleteTOTP deletes the user's row: the authenticator app and any pending setup.
	DeleteTOTP(ctx context.Context, db DBExecutor, userID string) error
	// ReplaceRecoveryCodes deletes the user's codes and stores hashes in their place.
	ReplaceRecoveryCodes(ctx context.Context, db DBExecutor, userID string, hashes []string, now time.Time) error
	// DeleteRecoveryCodes deletes the user's codes.
	DeleteRecoveryCodes(ctx context.Context, db DBExecutor, userID string) error
	// ConsumeRecoveryCode marks the unused code with hash used, and reports whether there
	// was one. Two concurrent calls with the same code: one succeeds.
	ConsumeRecoveryCode(ctx context.Context, db DBExecutor, userID, hash string, now time.Time) (bool, error)
	// CountRemainingRecoveryCodes counts the unused codes, as the executor sees them.
	CountRemainingRecoveryCodes(ctx context.Context, db DBExecutor, userID string) (int, error)
	// ListUsersWithoutTwoFactor returns the emails of the people (not API keys) whose
	// two-factor authentication is not on, sorted.
	ListUsersWithoutTwoFactor(ctx context.Context, db DBExecutor) ([]string, error)
}

// AuthTxRepository holds the writes to users and user_sessions that a sign-in or a
// two-factor operation makes inside its transaction.
type AuthTxRepository interface {
	// LockUser takes the user's row FOR UPDATE: the first lock of every transaction that
	// touches a user. ErrUserNotFound when there is none.
	LockUser(ctx context.Context, db DBExecutor, userID string) error
	// CreateUserIfAbsent inserts the user unless the email is taken, and reports whether it
	// did. A conflict never aborts the transaction.
	CreateUserIfAbsent(ctx context.Context, db DBExecutor, user *User) (bool, error)
	// CreateSession inserts a session with its new columns.
	CreateSession(ctx context.Context, db DBExecutor, session *Session) error
	// ConsumeMagicCode clears the unexpired magic code whose hash is codeHash on one of
	// the user's sessions, and returns that session; ErrSessionNotFound when none matches.
	ConsumeMagicCode(ctx context.Context, db DBExecutor, userID, codeHash string) (*Session, error)
	// HasExpiredMagicCode reports whether one of the user's sessions holds this code past
	// its expiry, to tell an expired code from a wrong one.
	HasExpiredMagicCode(ctx context.Context, db DBExecutor, userID, codeHash string) (bool, error)
	// PurgeExpiredPendingSessions deletes the user's sessions that wait for a code or a
	// challenge that expired.
	PurgeExpiredPendingSessions(ctx context.Context, db DBExecutor, userID string) error
	// ActivateSession records how the session was opened and that it is usable now.
	ActivateSession(ctx context.Context, db DBExecutor, sessionID string, method AuthMethod, now time.Time) error
	// SetSessionChallenge makes the session wait for its second factor.
	SetSessionChallenge(ctx context.Context, db DBExecutor, sessionID string, method AuthMethod, challengeHash string, expiresAt time.Time, pendingInvitationID *string) error
	// FindChallengeUser returns the user whose session holds the challenge, expired or not,
	// read without a lock; ErrTwoFactorChallengeInvalid when none does. Expiry is judged
	// after the locks, so that a real challenge that expired is audited under its user
	// rather than skipped like a token nobody issued.
	FindChallengeUser(ctx context.Context, db DBExecutor, challengeHash string) (string, error)
	// GetChallengeSessionForUpdate locks the user's session that holds the challenge;
	// ErrTwoFactorChallengeInvalid when it no longer does. It does not judge the challenge:
	// the caller compares ChallengeExpiresAt with ClockTimestamp, read after this lock, and
	// ChallengeAttempts with MaxChallengeAttempts.
	GetChallengeSessionForUpdate(ctx context.Context, db DBExecutor, userID, challengeHash string) (*Session, error)
	// FailChallenge counts one attempt against the session's challenge.
	FailChallenge(ctx context.Context, db DBExecutor, sessionID string) error
	// CompleteChallenge clears the challenge and records the second factor passed.
	CompleteChallenge(ctx context.Context, db DBExecutor, sessionID string, factor SecondFactor, now time.Time) error
	// GetSessionForUpdate locks one of the user's usable sessions.
	GetSessionForUpdate(ctx context.Context, db DBExecutor, userID, sessionID string) (*Session, error)
	// SetSessionSecondFactor records the second factor a usable session of the user passed;
	// ErrSessionNotFound when the session is gone or not the user's.
	SetSessionSecondFactor(ctx context.Context, db DBExecutor, userID, sessionID string, factor SecondFactor, now time.Time) error
	// ClearSecondFactorOnSessions forgets the second factor of every session of the user.
	ClearSecondFactorOnSessions(ctx context.Context, db DBExecutor, userID string) error
	// DeleteSessionsExcept deletes the user's sessions but keep.
	DeleteSessionsExcept(ctx context.Context, db DBExecutor, userID, keep string) error
	// DeleteAllSessions deletes every session of the user.
	DeleteAllSessions(ctx context.Context, db DBExecutor, userID string) error
}

// InvitationTxRepository holds what accepting an invitation does inside a transaction.
type InvitationTxRepository interface {
	// LockInvitation takes the invitation FOR UPDATE and says whether it expired, compared
	// in SQL; ErrInvitationNotFound when it is gone.
	LockInvitation(ctx context.Context, db DBExecutor, invitationID string) (*WorkspaceInvitation, bool, error)
	// DeleteInvitationsByEmail deletes the workspace's invitations to email, every case of it.
	DeleteInvitationsByEmail(ctx context.Context, db DBExecutor, workspaceID, email string) error
	// IsUserWorkspaceMember reports whether the user is a member of the workspace.
	IsUserWorkspaceMember(ctx context.Context, db DBExecutor, userID, workspaceID string) (bool, error)
	// CountWorkspaceMembersAndInvitations counts human members and live invitations, those to
	// excludeEmail left out.
	CountWorkspaceMembersAndInvitations(ctx context.Context, db DBExecutor, workspaceID, excludeEmail string) (int, error)
	// AddUserToWorkspaceIfAbsent adds the membership unless one exists, and reports whether
	// it did. An existing membership is never rewritten.
	AddUserToWorkspaceIfAbsent(ctx context.Context, db DBExecutor, membership *UserWorkspace) (bool, error)
}
