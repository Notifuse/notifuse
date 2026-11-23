package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// fakeOAuthRepo is an in-memory domain.OAuthRepository with the semantics of
// oauth_postgres.go: conditional updates, a transaction that serializes every
// other transaction (standing in for the grant row lock) and rolls back on
// error. The real statements are tested against PostgreSQL in the repository
// package.
type fakeOAuthRepo struct {
	txMu sync.Mutex
	mu   sync.Mutex

	clients     map[string]domain.OAuthClient
	requests    map[string]domain.OAuthAuthorizationRequest
	grants      map[string]domain.OAuthGrant
	codes       map[string]domain.OAuthAuthorizationCode
	tokens      map[string]domain.OAuthToken
	keys        map[string]domain.User
	memberships map[string]domain.UserWorkspace

	// grantlessOverride, when set, is what CountGrantlessClients answers.
	grantlessOverride *int
	// failOn makes a method fail.
	failOn map[string]error
	calls  []string
}

func newFakeOAuthRepo() *fakeOAuthRepo {
	return &fakeOAuthRepo{
		clients:     map[string]domain.OAuthClient{},
		requests:    map[string]domain.OAuthAuthorizationRequest{},
		grants:      map[string]domain.OAuthGrant{},
		codes:       map[string]domain.OAuthAuthorizationCode{},
		tokens:      map[string]domain.OAuthToken{},
		keys:        map[string]domain.User{},
		memberships: map[string]domain.UserWorkspace{},
		failOn:      map[string]error{},
	}
}

func cloneMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func timeRef(t time.Time) *time.Time { return &t }

// op records a call and returns the injected failure, if any. Callers hold mu.
func (f *fakeOAuthRepo) op(name string) error {
	f.calls = append(f.calls, name)
	return f.failOn[name]
}

func (f *fakeOAuthRepo) called(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, call := range f.calls {
		if call == name {
			n++
		}
	}
	return n
}

func (f *fakeOAuthRepo) WithTx(ctx context.Context, fn func(tx domain.OAuthStore) error) error {
	f.txMu.Lock()
	defer f.txMu.Unlock()
	f.mu.Lock()
	snapshot := []any{cloneMap(f.clients), cloneMap(f.requests), cloneMap(f.grants), cloneMap(f.codes), cloneMap(f.tokens), cloneMap(f.keys), cloneMap(f.memberships)}
	f.mu.Unlock()
	if err := fn(f); err != nil {
		f.mu.Lock()
		f.clients = snapshot[0].(map[string]domain.OAuthClient)
		f.requests = snapshot[1].(map[string]domain.OAuthAuthorizationRequest)
		f.grants = snapshot[2].(map[string]domain.OAuthGrant)
		f.codes = snapshot[3].(map[string]domain.OAuthAuthorizationCode)
		f.tokens = snapshot[4].(map[string]domain.OAuthToken)
		f.keys = snapshot[5].(map[string]domain.User)
		f.memberships = snapshot[6].(map[string]domain.UserWorkspace)
		f.mu.Unlock()
		return err
	}
	return nil
}

func (f *fakeOAuthRepo) GetClient(ctx context.Context, clientID string) (*domain.OAuthClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("GetClient"); err != nil {
		return nil, err
	}
	client, ok := f.clients[clientID]
	if !ok {
		return nil, domain.ErrOAuthNotFound
	}
	return &client, nil
}

func (f *fakeOAuthRepo) SaveClient(ctx context.Context, client *domain.OAuthClient) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("SaveClient"); err != nil {
		return err
	}
	stored := *client
	if existing, ok := f.clients[client.ClientID]; ok {
		stored.CreatedAt = existing.CreatedAt
		stored.LastUsedAt = existing.LastUsedAt
	}
	stored.FetchFailedUntil = nil
	f.clients[client.ClientID] = stored
	return nil
}

func (f *fakeOAuthRepo) MarkClientFetchFailed(ctx context.Context, clientID string, until, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("MarkClientFetchFailed"); err != nil {
		return err
	}
	client, ok := f.clients[clientID]
	if !ok {
		client = domain.OAuthClient{ClientID: clientID, CreatedAt: now}
	}
	client.FetchFailedUntil = timeRef(until)
	f.clients[clientID] = client
	return nil
}

func (f *fakeOAuthRepo) CountGrantlessClients(ctx context.Context, exemptHosts []string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("CountGrantlessClients"); err != nil {
		return 0, err
	}
	if f.grantlessOverride != nil {
		return *f.grantlessOverride, nil
	}
	if !slices.Equal(exemptHosts, domain.OAuthKnownClientHosts) {
		return 0, errors.New("the fake exempts the known hosts only")
	}
	count := 0
	for clientID, client := range f.clients {
		exempt := client.FetchFailedUntil == nil && domain.IsKnownOAuthClient(clientID)
		granted := false
		for _, grant := range f.grants {
			if grant.ClientID == clientID {
				granted = true
			}
		}
		if !exempt && !granted {
			count++
		}
	}
	return count, nil
}

func (f *fakeOAuthRepo) TouchClient(ctx context.Context, clientID string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("TouchClient"); err != nil {
		return err
	}
	if client, ok := f.clients[clientID]; ok && (client.LastUsedAt == nil || !client.LastUsedAt.After(now.Add(-domain.OAuthLastUsedInterval))) {
		client.LastUsedAt = timeRef(now)
		f.clients[clientID] = client
	}
	return nil
}

func (f *fakeOAuthRepo) CreateRequest(ctx context.Context, request *domain.OAuthAuthorizationRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("CreateRequest"); err != nil {
		return err
	}
	f.requests[request.ID] = *request
	return nil
}

func (f *fakeOAuthRepo) GetRequest(ctx context.Context, id string) (*domain.OAuthAuthorizationRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("GetRequest"); err != nil {
		return nil, err
	}
	request, ok := f.requests[id]
	if !ok {
		return nil, domain.ErrOAuthNotFound
	}
	return &request, nil
}

func (f *fakeOAuthRepo) ConsumeRequest(ctx context.Context, id string, now time.Time) (*domain.OAuthAuthorizationRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("ConsumeRequest"); err != nil {
		return nil, err
	}
	request, ok := f.requests[id]
	if !ok || request.ConsumedAt != nil || !request.ExpiresAt.After(now) {
		return nil, domain.ErrOAuthNotFound
	}
	request.ConsumedAt = timeRef(now)
	f.requests[id] = request
	return &request, nil
}

func (f *fakeOAuthRepo) CreateGrant(ctx context.Context, grant *domain.OAuthGrant) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("CreateGrant"); err != nil {
		return err
	}
	f.grants[grant.ID] = *grant
	return nil
}

func (f *fakeOAuthRepo) GetGrant(ctx context.Context, id string) (*domain.OAuthGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("GetGrant"); err != nil {
		return nil, err
	}
	grant, ok := f.grants[id]
	if !ok {
		return nil, domain.ErrOAuthNotFound
	}
	return &grant, nil
}

func (f *fakeOAuthRepo) LockGrant(ctx context.Context, id string) (*domain.OAuthGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("LockGrant"); err != nil {
		return nil, err
	}
	grant, ok := f.grants[id]
	if !ok {
		return nil, domain.ErrOAuthNotFound
	}
	return &grant, nil
}

func (f *fakeOAuthRepo) RevokeGrant(ctx context.Context, id, reason string, now time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("RevokeGrant"); err != nil {
		return false, err
	}
	grant, ok := f.grants[id]
	if !ok || grant.RevokedAt != nil {
		return false, nil
	}
	grant.RevokedAt = timeRef(now)
	grant.RevokedReason = reason
	f.grants[id] = grant
	return true, nil
}

func (f *fakeOAuthRepo) TouchGrant(ctx context.Context, id string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("TouchGrant"); err != nil {
		return err
	}
	if grant, ok := f.grants[id]; ok && (grant.LastUsedAt == nil || !grant.LastUsedAt.After(now.Add(-domain.OAuthLastUsedInterval))) {
		grant.LastUsedAt = timeRef(now)
		f.grants[id] = grant
	}
	return nil
}

func (f *fakeOAuthRepo) ListWorkspaceGrants(ctx context.Context, workspaceID string) ([]*domain.OAuthGrantListItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("ListWorkspaceGrants"); err != nil {
		return nil, err
	}
	items := []*domain.OAuthGrantListItem{}
	for _, grant := range f.grants {
		if grant.WorkspaceID != workspaceID {
			continue
		}
		status := domain.OAuthGrantStatusActive
		if grant.Revoked() {
			status = domain.OAuthGrantStatusRevoked
		}
		items = append(items, &domain.OAuthGrantListItem{OAuthGrant: grant, Status: status})
	}
	return items, nil
}

func (f *fakeOAuthRepo) CreateCode(ctx context.Context, code *domain.OAuthAuthorizationCode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("CreateCode"); err != nil {
		return err
	}
	f.codes[code.CodeHash] = *code
	return nil
}

func (f *fakeOAuthRepo) GetCode(ctx context.Context, codeHash string) (*domain.OAuthAuthorizationCode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("GetCode"); err != nil {
		return nil, err
	}
	code, ok := f.codes[codeHash]
	if !ok {
		return nil, domain.ErrOAuthNotFound
	}
	return &code, nil
}

func (f *fakeOAuthRepo) UseCode(ctx context.Context, codeHash string, now time.Time) (*domain.OAuthAuthorizationCode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("UseCode"); err != nil {
		return nil, err
	}
	code, ok := f.codes[codeHash]
	if !ok || code.UsedAt != nil || !code.ExpiresAt.After(now) {
		return nil, domain.ErrOAuthNotFound
	}
	code.UsedAt = timeRef(now)
	f.codes[codeHash] = code
	return &code, nil
}

func (f *fakeOAuthRepo) CreateTokens(ctx context.Context, tokens ...*domain.OAuthToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("CreateTokens"); err != nil {
		return err
	}
	for _, token := range tokens {
		f.tokens[token.TokenHash] = *token
	}
	return nil
}

func (f *fakeOAuthRepo) GetToken(ctx context.Context, tokenHash string) (*domain.OAuthToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("GetToken"); err != nil {
		return nil, err
	}
	token, ok := f.tokens[tokenHash]
	if !ok {
		return nil, domain.ErrOAuthNotFound
	}
	return &token, nil
}

func (f *fakeOAuthRepo) GetAccessTokenGrant(ctx context.Context, tokenHash string, now time.Time) (*domain.OAuthGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("GetAccessTokenGrant"); err != nil {
		return nil, err
	}
	token, ok := f.tokens[tokenHash]
	if !ok || token.Kind != domain.OAuthTokenKindAccess || token.InvalidatedAt != nil || !token.ExpiresAt.After(now) {
		return nil, domain.ErrOAuthNotFound
	}
	grant, ok := f.grants[token.GrantID]
	if !ok || grant.RevokedAt != nil {
		return nil, domain.ErrOAuthNotFound
	}
	return &grant, nil
}

func (f *fakeOAuthRepo) MarkTokenRotated(ctx context.Context, tokenHash, replacedByHash string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("MarkTokenRotated"); err != nil {
		return err
	}
	if token, ok := f.tokens[tokenHash]; ok && token.RotatedAt == nil {
		token.RotatedAt = timeRef(now)
		token.ReplacedByHash = replacedByHash
		f.tokens[tokenHash] = token
	}
	return nil
}

func (f *fakeOAuthRepo) InvalidateToken(ctx context.Context, tokenHash string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("InvalidateToken"); err != nil {
		return err
	}
	if token, ok := f.tokens[tokenHash]; ok && token.InvalidatedAt == nil {
		token.InvalidatedAt = timeRef(now)
		f.tokens[tokenHash] = token
	}
	return nil
}

func (f *fakeOAuthRepo) InvalidatePair(ctx context.Context, grantID, pairID string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("InvalidatePair"); err != nil {
		return err
	}
	for hash, token := range f.tokens {
		if token.GrantID == grantID && token.PairID == pairID && token.InvalidatedAt == nil {
			token.InvalidatedAt = timeRef(now)
			f.tokens[hash] = token
		}
	}
	return nil
}

func (f *fakeOAuthRepo) InvalidateLiveAccessTokens(ctx context.Context, grantID string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("InvalidateLiveAccessTokens"); err != nil {
		return err
	}
	for hash, token := range f.tokens {
		if token.GrantID == grantID && token.Kind == domain.OAuthTokenKindAccess && token.InvalidatedAt == nil && token.ExpiresAt.After(now) {
			token.InvalidatedAt = timeRef(now)
			f.tokens[hash] = token
		}
	}
	return nil
}

func (f *fakeOAuthRepo) CreateAPIKeyAccount(ctx context.Context, key *domain.User, membership *domain.UserWorkspace) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.op("CreateAPIKeyAccount"); err != nil {
		return err
	}
	for _, existing := range f.keys {
		if existing.Email == key.Email {
			// users.email is unique, as insertUserRow reports it.
			return &domain.ErrUserExists{Message: "user already exists"}
		}
	}
	f.keys[key.ID] = *key
	f.memberships[key.ID] = *membership
	return nil
}

func (f *fakeOAuthRepo) RevokeGrantsWithoutCredential(ctx context.Context, now time.Time) (int64, error) {
	return 0, errors.New("not used by the service tests")
}

func (f *fakeOAuthRepo) PurgeExpired(ctx context.Context, now time.Time) (int64, int64, int64, error) {
	return 0, 0, 0, errors.New("not used by the service tests")
}

func (f *fakeOAuthRepo) PurgeClients(ctx context.Context, now time.Time) (int64, error) {
	return 0, errors.New("not used by the service tests")
}

// deleteKey deletes an API key as RemoveMember does, with the cascade of the
// foreign keys: its grants, and their codes and tokens.
func (f *fakeOAuthRepo) deleteKey(keyID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.keys, keyID)
	delete(f.memberships, keyID)
	for id, grant := range f.grants {
		if grant.APIKeyUserID != keyID {
			continue
		}
		delete(f.grants, id)
		for hash, code := range f.codes {
			if code.GrantID == id {
				delete(f.codes, hash)
			}
		}
		for hash, token := range f.tokens {
			if token.GrantID == id {
				delete(f.tokens, hash)
			}
		}
	}
}

func (f *fakeOAuthRepo) grant(id string) domain.OAuthGrant {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.grants[id]
}

func (f *fakeOAuthRepo) onlyGrant() domain.OAuthGrant {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.grants) != 1 {
		panic("expected exactly one grant")
	}
	for _, grant := range f.grants {
		return grant
	}
	return domain.OAuthGrant{}
}

func (f *fakeOAuthRepo) token(clear string) domain.OAuthToken {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tokens[domain.HashOAuthSecret(clear)]
}

func (f *fakeOAuthRepo) request(id string) domain.OAuthAuthorizationRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[id]
}

func (f *fakeOAuthRepo) client(id string) (domain.OAuthClient, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	client, ok := f.clients[id]
	return client, ok
}

func (f *fakeOAuthRepo) counts() (grants, keys int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.grants), len(f.keys)
}

// fakeOAuthAuth plays the auth service: API key tokens, console sessions,
// memberships and root, over the accounts of the test and the keys the fake
// repository created. It implements domain.AuthService too, so the real
// WorkspaceService creates keys through it.
type fakeOAuthAuth struct {
	mu          sync.Mutex
	repo        *fakeOAuthRepo
	accounts    map[string]*domain.User
	apiKeys     map[string]string // token → account ID
	sessions    map[string]bool
	memberships map[string]map[string]*domain.UserWorkspace
	rootEmail   string
	minted      int
}

func newFakeOAuthAuth(repo *fakeOAuthRepo) *fakeOAuthAuth {
	return &fakeOAuthAuth{
		repo:        repo,
		accounts:    map[string]*domain.User{},
		apiKeys:     map[string]string{},
		sessions:    map[string]bool{},
		memberships: map[string]map[string]*domain.UserWorkspace{},
	}
}

func (a *fakeOAuthAuth) account(id string) *domain.User {
	a.mu.Lock()
	user, ok := a.accounts[id]
	a.mu.Unlock()
	if ok {
		return user
	}
	a.repo.mu.Lock()
	defer a.repo.mu.Unlock()
	if key, ok := a.repo.keys[id]; ok {
		return &key
	}
	return nil
}

func (a *fakeOAuthAuth) membership(userID, workspaceID string) *domain.UserWorkspace {
	a.mu.Lock()
	membership := a.memberships[userID][workspaceID]
	a.mu.Unlock()
	if membership != nil {
		copied := *membership
		return &copied
	}
	a.repo.mu.Lock()
	defer a.repo.mu.Unlock()
	if m, ok := a.repo.memberships[userID]; ok && m.WorkspaceID == workspaceID {
		return &m
	}
	return nil
}

func (a *fakeOAuthAuth) AuthenticateAPIKeyToken(ctx context.Context, token string) (*domain.User, error) {
	a.mu.Lock()
	id, ok := a.apiKeys[token]
	a.mu.Unlock()
	if !ok {
		return nil, domain.ErrNotAnAPIKey
	}
	user := a.account(id)
	if user == nil || user.Type != domain.UserTypeAPIKey {
		return nil, domain.ErrAPIKeyRevoked
	}
	return user, nil
}

func (a *fakeOAuthAuth) AuthenticateUserFromContext(ctx context.Context) (*domain.User, error) {
	userID, _ := ctx.Value(domain.UserIDKey).(string)
	userType, _ := ctx.Value(domain.UserTypeKey).(string)
	user := a.account(userID)
	switch userType {
	case string(domain.UserTypeAPIKey):
		if user == nil || user.Type != domain.UserTypeAPIKey {
			return nil, domain.ErrAPIKeyRevoked
		}
		return user, nil
	case string(domain.UserTypeUser):
		a.mu.Lock()
		live := a.sessions[userID]
		a.mu.Unlock()
		if !live || user == nil || user.Type != domain.UserTypeUser {
			return nil, fmt.Errorf("%w: %w", domain.ErrSessionAuthFailed, ErrSessionExpired)
		}
		return user, nil
	}
	return nil, fmt.Errorf("%w: %w", domain.ErrSessionAuthFailed, ErrUserNotFound)
}

func (a *fakeOAuthAuth) AuthenticateUserForWorkspace(ctx context.Context, workspaceID string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
	user, err := a.AuthenticateUserFromContext(ctx)
	if err != nil {
		return ctx, nil, nil, err
	}
	isRoot := user.Type == domain.UserTypeUser && user.Email == a.rootEmail
	membership := a.membership(user.ID, workspaceID)
	switch {
	case isRoot:
		membership = &domain.UserWorkspace{UserID: user.ID, WorkspaceID: workspaceID, Role: "owner", Permissions: domain.NewFullPermissions()}
	case membership == nil:
		return ctx, nil, nil, domain.ErrUserNotInWorkspace
	}
	if record := domain.AuditFromContext(ctx); record != nil {
		record.SetActor(user, membership, isRoot)
		record.SetWorkspace(workspaceID)
	}
	return ctx, user, membership, nil
}

func (a *fakeOAuthAuth) VerifyUserSession(ctx context.Context, userID, sessionID string) (*domain.User, error) {
	return nil, errors.New("not used")
}

func (a *fakeOAuthAuth) AuthenticateSession(ctx context.Context) (*domain.User, *domain.SessionAuth, error) {
	return nil, nil, errors.New("not used")
}

func (a *fakeOAuthAuth) RequireCompliantSession(ctx context.Context) error {
	return nil
}

func (a *fakeOAuthAuth) RequireCompliantRootSession(ctx context.Context) error {
	return nil
}

func (a *fakeOAuthAuth) GenerateUserAuthToken(user *domain.User, sessionID string, expiresAt time.Time) string {
	return ""
}

func (a *fakeOAuthAuth) GenerateAPIAuthToken(user *domain.User) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.minted++
	return "minted"
}

func (a *fakeOAuthAuth) GenerateInvitationToken(invitation *domain.WorkspaceInvitation) string {
	return ""
}

func (a *fakeOAuthAuth) ValidateInvitationToken(token string) (string, string, string, error) {
	return "", "", "", errors.New("not used")
}

func (a *fakeOAuthAuth) InvalidateSecretCache() {}

// fakeOAuthWorkspaces reads workspaces and the memberships of fakeOAuthAuth.
type fakeOAuthWorkspaces struct {
	auth       *fakeOAuthAuth
	workspaces map[string]*domain.Workspace
}

func (w *fakeOAuthWorkspaces) GetByID(ctx context.Context, id string) (*domain.Workspace, error) {
	workspace, ok := w.workspaces[id]
	if !ok {
		return nil, &domain.ErrWorkspaceNotFound{WorkspaceID: id}
	}
	copied := *workspace
	return &copied, nil
}

func (w *fakeOAuthWorkspaces) GetUserWorkspaces(ctx context.Context, userID string) ([]*domain.UserWorkspace, error) {
	var out []*domain.UserWorkspace
	w.auth.mu.Lock()
	for _, membership := range w.auth.memberships[userID] {
		copied := *membership
		out = append(out, &copied)
	}
	w.auth.mu.Unlock()
	w.auth.repo.mu.Lock()
	if membership, ok := w.auth.repo.memberships[userID]; ok {
		out = append(out, &membership)
	}
	w.auth.repo.mu.Unlock()
	return out, nil
}

func (w *fakeOAuthWorkspaces) List(ctx context.Context) ([]*domain.Workspace, error) {
	var out []*domain.Workspace
	for _, id := range []string{"ws1", "ws2", "ws-off"} {
		if workspace, ok := w.workspaces[id]; ok {
			out = append(out, workspace)
		}
	}
	return out, nil
}

// fakeOAuthFetcher serves metadata documents by client_id and counts fetches.
type fakeOAuthFetcher struct {
	mu        sync.Mutex
	documents map[string]*OAuthClientDocument
	errs      map[string]error
	fetches   int
}

func (f *fakeOAuthFetcher) Fetch(ctx context.Context, clientID string) (*OAuthClientDocument, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fetches++
	if err, ok := f.errs[clientID]; ok {
		return nil, err
	}
	document, ok := f.documents[clientID]
	if !ok {
		return nil, errors.New("the client metadata document answered 404, not 200")
	}
	return document, nil
}

func (f *fakeOAuthFetcher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fetches
}

// recordingAuditRecorder keeps every row.
type recordingAuditRecorder struct {
	mu   sync.Mutex
	rows []domain.AuditLog
}

func (r *recordingAuditRecorder) Record(ctx context.Context, log *domain.AuditLog) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, *log)
}

func (r *recordingAuditRecorder) all() []domain.AuditLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.AuditLog(nil), r.rows...)
}
