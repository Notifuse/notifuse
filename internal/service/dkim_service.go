package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/dkim"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// DKIMService manages the DKIM keys of a workspace's domains. Key generation and DNS
// lookups run outside the transaction that writes the keys, which re-checks under the
// row lock what it decided on.
type DKIMService struct {
	repo        domain.WorkspaceRepository
	authService domain.AuthService
	store       *DKIMKeyStore
	resolver    dkim.TXTResolver
	secretKey   string
	logger      logger.Logger

	now      func() time.Time
	random   io.Reader
	generate func() (privateKeyPEM, publicKeyBase64 string, err error)
	newID    func() string
}

// NewDKIMService returns the service. store is the one EmailService signs with, so that
// a change is seen by the next send on this instance; resolver is dkim.DefaultResolver
// outside tests.
func NewDKIMService(repo domain.WorkspaceRepository, authService domain.AuthService, store *DKIMKeyStore, resolver dkim.TXTResolver, secretKey string, logger logger.Logger) *DKIMService {
	if resolver == nil {
		resolver = dkim.DefaultResolver
	}
	return &DKIMService{
		repo:        repo,
		authService: authService,
		store:       store,
		resolver:    resolver,
		secretKey:   secretKey,
		logger:      logger,
		now:         func() time.Time { return time.Now().UTC() },
		random:      rand.Reader,
		generate:    dkim.GenerateRSA2048,
		newID:       func() string { return uuid.New().String() },
	}
}

var _ domain.DKIMService = (*DKIMService)(nil)

func (s *DKIMService) authorize(ctx context.Context, workspaceID string, owner bool) (context.Context, error) {
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return ctx, fmt.Errorf("failed to authenticate user: %w", err)
	}
	if owner {
		if userWorkspace.Role != "owner" {
			return ctx, &domain.ErrUnauthorized{Message: "Only workspace owners can manage DKIM keys"}
		}
		return ctx, nil
	}
	if !userWorkspace.HasPermission(domain.PermissionResourceWorkspace, domain.PermissionTypeRead) &&
		!userWorkspace.HasPermission(domain.PermissionResourceWorkspace, domain.PermissionTypeWrite) {
		return ctx, &domain.ErrUnauthorized{Message: "You do not have permission to read the workspace settings"}
	}
	return ctx, nil
}

func normalizeDomainArg(raw string) (string, error) {
	d, err := domain.NormalizeDKIMDomain(raw)
	if err != nil {
		return "", &domain.ErrDKIMInvalidDomain{Message: err.Error()}
	}
	return d, nil
}

func auditDKIM(ctx context.Context, workspaceID, domainName string, change domain.AuditChange) {
	if audit := domain.AuditFromContext(ctx); audit != nil {
		audit.SetTarget(domain.AuditTargetWorkspace, workspaceID, "")
		audit.SetChanges(map[string]domain.AuditChange{domainName: change})
	}
}

// ListDomains returns the workspace's domains with the integrations that use each.
func (s *DKIMService) ListDomains(ctx context.Context, workspaceID string) ([]domain.DKIMDomainView, error) {
	ctx, err := s.authorize(ctx, workspaceID, false)
	if err != nil {
		return nil, err
	}
	workspace, err := s.repo.GetByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	views := make([]domain.DKIMDomainView, 0, len(workspace.Settings.DKIMKeys))
	for _, k := range workspace.Settings.DKIMKeys {
		views = append(views, domain.NewDKIMDomainView(k, domain.DKIMDomainUsers(workspace.Integrations, k.Domain)))
	}
	return views, nil
}

// newKey generates a key for a domain, outside any transaction (RSA 2048 takes time).
func (s *DKIMService) newKey(domainName string) (domain.DKIMKey, error) {
	privPEM, pub, err := s.generate()
	if err != nil {
		return domain.DKIMKey{}, err
	}
	sealed, err := dkim.EncryptPrivateKey(privPEM, s.secretKey)
	if err != nil {
		return domain.DKIMKey{}, fmt.Errorf("failed to encrypt the private key: %w", err)
	}
	now := s.now()
	selector, err := domain.NewDKIMSelector(now, s.random)
	if err != nil {
		return domain.DKIMKey{}, err
	}
	return domain.DKIMKey{
		ID:                  s.newID(),
		Revision:            1,
		Domain:              domainName,
		Selector:            selector,
		PublicKey:           pub,
		EncryptedPrivateKey: sealed,
		Status:              domain.DKIMKeyStatusPending,
		CreatedAt:           now,
	}, nil
}

// CreateDomain generates a key for a new domain.
func (s *DKIMService) CreateDomain(ctx context.Context, workspaceID, rawDomain string) (*domain.DKIMDomainView, error) {
	ctx, err := s.authorize(ctx, workspaceID, true)
	if err != nil {
		return nil, err
	}
	domainName, err := normalizeDomainArg(rawDomain)
	if err != nil {
		return nil, err
	}
	key, err := s.newKey(domainName)
	if err != nil {
		return nil, err
	}

	var usedBy []domain.DKIMDomainUser
	_, err = s.repo.UpdateDKIMKeys(ctx, workspaceID, func(keys []domain.DKIMKey, integrations domain.Integrations) (domain.DKIMKeysMutation, error) {
		if domain.FindDKIMKey(keys, domainName) >= 0 {
			return domain.DKIMKeysMutation{}, &domain.ErrDKIMConflict{Message: fmt.Sprintf("%s already has a DKIM key", domainName)}
		}
		if len(keys) >= domain.MaxDKIMKeysPerWorkspace {
			return domain.DKIMKeysMutation{}, &domain.ErrDKIMConflict{Message: fmt.Sprintf("a workspace can have at most %d DKIM domains", domain.MaxDKIMKeysPerWorkspace)}
		}
		usedBy = domain.DKIMDomainUsers(integrations, domainName)
		return domain.DKIMKeysMutation{Keys: append(append([]domain.DKIMKey{}, keys...), key), Changed: true, TouchUpdatedAt: true}, nil
	})
	if err != nil {
		return nil, err
	}
	s.store.Invalidate(workspaceID)
	auditDKIM(ctx, workspaceID, domainName, domain.AuditChange{New: key.Selector})

	view := domain.NewDKIMDomainView(key, usedBy)
	return &view, nil
}

// RegenerateKey replaces the key of a domain that does not sign yet (pending or
// mismatch) with a new key and selector. A published key is refused: rotating a key
// that signs needs two selectors, which comes later.
func (s *DKIMService) RegenerateKey(ctx context.Context, workspaceID, rawDomain string) (*domain.DKIMDomainView, error) {
	ctx, err := s.authorize(ctx, workspaceID, true)
	if err != nil {
		return nil, err
	}
	domainName, err := normalizeDomainArg(rawDomain)
	if err != nil {
		return nil, err
	}
	key, err := s.newKey(domainName)
	if err != nil {
		return nil, err
	}

	var oldSelector string
	var usedBy []domain.DKIMDomainUser
	_, err = s.repo.UpdateDKIMKeys(ctx, workspaceID, func(keys []domain.DKIMKey, integrations domain.Integrations) (domain.DKIMKeysMutation, error) {
		i := domain.FindDKIMKey(keys, domainName)
		if i < 0 {
			return domain.DKIMKeysMutation{}, &domain.ErrDKIMDomainNotFound{Domain: domainName}
		}
		if keys[i].Status == domain.DKIMKeyStatusPublished {
			return domain.DKIMKeysMutation{}, &domain.ErrDKIMConflict{Message: fmt.Sprintf("the key of %s is published and signs: it cannot be regenerated", domainName)}
		}
		oldSelector = keys[i].Selector
		updated := append([]domain.DKIMKey{}, keys...)
		updated[i] = key
		usedBy = domain.DKIMDomainUsers(integrations, domainName)
		return domain.DKIMKeysMutation{Keys: updated, Changed: true, TouchUpdatedAt: true}, nil
	})
	if err != nil {
		return nil, err
	}
	s.store.Invalidate(workspaceID)
	auditDKIM(ctx, workspaceID, domainName, domain.AuditChange{Old: oldSelector, New: key.Selector})

	view := domain.NewDKIMDomainView(key, usedBy)
	return &view, nil
}

// VerifyDomain looks up the domain's record now and applies the result, unless the key
// changed during the lookup.
func (s *DKIMService) VerifyDomain(ctx context.Context, workspaceID, rawDomain string) (*domain.DKIMVerifyResult, error) {
	ctx, err := s.authorize(ctx, workspaceID, true)
	if err != nil {
		return nil, err
	}
	domainName, err := normalizeDomainArg(rawDomain)
	if err != nil {
		return nil, err
	}

	keys, err := s.repo.GetWorkspaceDKIMKeys(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	i := domain.FindDKIMKey(keys, domainName)
	if i < 0 {
		return nil, &domain.ErrDKIMDomainNotFound{Domain: domainName}
	}
	checked := keys[i]

	result, detail := dkim.VerifyDNS(ctx, s.resolver, checked.Selector, checked.Domain, checked.PublicKey)
	check := domain.DKIMCheck{Result: domain.DKIMCheckResult(result), Detail: detail}
	now := s.now()

	var stored domain.DKIMKey
	var transition *domain.DKIMTransition
	var usedBy []domain.DKIMDomainUser
	stale := false
	_, err = s.repo.UpdateDKIMKeys(ctx, workspaceID, func(current []domain.DKIMKey, integrations domain.Integrations) (domain.DKIMKeysMutation, error) {
		j := domain.FindDKIMKey(current, domainName)
		if j < 0 {
			return domain.DKIMKeysMutation{}, &domain.ErrDKIMDomainNotFound{Domain: domainName}
		}
		usedBy = domain.DKIMDomainUsers(integrations, domainName)
		if current[j].ID != checked.ID || current[j].Revision != checked.Revision {
			stale = true
			stored = current[j]
			return domain.DKIMKeysMutation{}, nil
		}
		stored, transition = domain.EvaluateDKIMCheck(current[j], check.Result, detail, now)
		updated := append([]domain.DKIMKey{}, current...)
		updated[j] = stored
		return domain.DKIMKeysMutation{Keys: updated, Changed: true, TouchUpdatedAt: transition != nil}, nil
	})
	if err != nil {
		return nil, err
	}
	if !stale {
		s.store.Invalidate(workspaceID)
	}
	// The audit records what this call changed: nothing when another writer got there
	// first.
	switch {
	case stale:
		auditDKIM(ctx, workspaceID, domainName, domain.AuditChange{})
	case transition != nil:
		transition.WorkspaceID = workspaceID
		logDKIMTransition(s.logger, transition)
		auditDKIM(ctx, workspaceID, domainName, domain.AuditChange{Old: transition.OldStatus, New: transition.NewStatus})
	default:
		auditDKIM(ctx, workspaceID, domainName, domain.AuditChange{Old: checked.Status, New: stored.Status})
	}

	return &domain.DKIMVerifyResult{
		Domain:        domain.NewDKIMDomainView(stored, usedBy),
		Check:         check,
		StaleConflict: stale,
	}, nil
}

// DeleteDomain removes the key of a domain no SMTP sender uses, whatever its status.
func (s *DKIMService) DeleteDomain(ctx context.Context, workspaceID, rawDomain string) error {
	ctx, err := s.authorize(ctx, workspaceID, true)
	if err != nil {
		return err
	}
	domainName, err := normalizeDomainArg(rawDomain)
	if err != nil {
		return err
	}

	var selector string
	_, err = s.repo.UpdateDKIMKeys(ctx, workspaceID, func(keys []domain.DKIMKey, integrations domain.Integrations) (domain.DKIMKeysMutation, error) {
		i := domain.FindDKIMKey(keys, domainName)
		if i < 0 {
			return domain.DKIMKeysMutation{}, &domain.ErrDKIMDomainNotFound{Domain: domainName}
		}
		if users := domain.DKIMDomainUsers(integrations, domainName); len(users) > 0 {
			return domain.DKIMKeysMutation{}, &domain.ErrDKIMConflict{
				Message: fmt.Sprintf("%s is used by senders of %d integration(s): change those senders first", domainName, len(users)),
				UsedBy:  users,
			}
		}
		selector = keys[i].Selector
		updated := append(append([]domain.DKIMKey{}, keys[:i]...), keys[i+1:]...)
		return domain.DKIMKeysMutation{Keys: updated, Changed: true, TouchUpdatedAt: true}, nil
	})
	if err != nil {
		return err
	}
	s.store.Invalidate(workspaceID)
	auditDKIM(ctx, workspaceID, domainName, domain.AuditChange{Old: selector})
	return nil
}

// logDKIMTransition logs a status change: at warn level when a key stops signing.
func logDKIMTransition(log logger.Logger, t *domain.DKIMTransition) {
	entry := log.WithFields(map[string]interface{}{
		"workspace_id": t.WorkspaceID,
		"domain":       t.Domain,
		"selector":     t.Selector,
		"old_status":   string(t.OldStatus),
		"new_status":   string(t.NewStatus),
		"reason":       t.Reason,
	})
	if t.IsDowngrade() {
		entry.Warn("DKIM key stopped signing: its DNS record is no longer found or no longer matches")
		return
	}
	entry.Info("DKIM key status changed")
}
