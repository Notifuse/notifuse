package domain

//go:generate mockgen -destination mocks/mock_audit_log_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain AuditLogRepository
//go:generate mockgen -destination mocks/mock_audit_log_service.go -package mocks github.com/Notifuse/notifuse/internal/domain AuditLogService
//go:generate mockgen -destination mocks/mock_audit_log_recorder.go -package mocks github.com/Notifuse/notifuse/internal/domain AuditLogRecorder

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The audit log records control-plane actions: who did what, to which
// resource, from where, and whether it was allowed. It never records the data
// plane — contact upserts, sends, tracking — and it never records secrets.
//
// One row per action. The action name is the RPC route that performed it
// (workspaces.inviteMember, templates.update), which keeps the catalogue
// honest: a route is either audited or explicitly excluded, and a test walks
// every registered route to make sure it is one or the other. Rows are written
// by the audit middleware after the response; services add what only they
// know (the target's name, a redacted before/after) through the AuditRecord
// they find in the request context.

const (
	AuditOutcomeSuccess = "success"
	AuditOutcomeFailure = "failure"
	AuditOutcomeDenied  = "denied"

	AuditActorUser      = "user"
	AuditActorAPIKey    = "api_key"
	AuditActorSystem    = "system"
	AuditActorAnonymous = "anonymous"

	AuditRoleOwner  = "owner"
	AuditRoleMember = "member"
	AuditRoleRoot   = "root"

	AuditAuthSession      = "session"
	AuditAuthAPIKey       = "api_key"
	AuditAuthMagicCode    = "magic_code"
	AuditAuthSSO          = "sso"
	AuditAuthRootPassword = "root_password"

	// AuditScopeWorkspace lists one workspace's rows and needs a workspace_id;
	// AuditScopeDeployment is the root user's view of every row, deployment-level
	// rows (no workspace) included.
	AuditScopeWorkspace  = "workspace"
	AuditScopeDeployment = "deployment"

	AuditExportFormatCSV    = "csv"
	AuditExportFormatNDJSON = "ndjson"

	DefaultAuditLogRetentionDays = 365
	AuditLogRetentionMinDays     = 30
	AuditLogRetentionMaxDays     = 3650

	AuditLogDefaultListLimit = 50
	AuditLogMaxListLimit     = 100
	AuditLogExportMaxRows    = 100_000

	// AuditRedactedValue replaces both sides of a change whose key looks like a
	// secret. The change itself stays visible: "the SMTP password changed" is a
	// fact the log must keep; the password is not.
	AuditRedactedValue = "[redacted]"

	// AuditMaxValueBytes caps a single value inside changes; a template body or a
	// segment tree above it is replaced by a marker so one row cannot bloat the
	// table.
	AuditMaxValueBytes = 2048
)

// AuditLogsRetentionSettingKey is the system settings row holding the
// deployment default retention, in days; absent means
// DefaultAuditLogRetentionDays.
const AuditLogsRetentionSettingKey = "audit_logs_retention_days"

// AuditLogsRecordingSettingKey is the system settings row remembering whether
// rows were being written the last time the recorder looked ("true"/"false"),
// so a licence that lapsed while the server was down still gets its
// licence.recordingStopped marker at the next boot.
const AuditLogsRecordingSettingKey = "audit_logs_recording"

// ErrAuditLogNotFound is returned by GetByID for a missing row.
var ErrAuditLogNotFound = errors.New("audit log not found")

// AuditChange is one field's before and after, the shape contact_timeline
// already uses and the console already renders as "old → new".
type AuditChange struct {
	Old any `json:"old"`
	New any `json:"new"`
}

// AuditLog is one stored row. WorkspaceID is empty for deployment-level
// actions (a login, a licence change, the system settings); StatusCode is zero
// when the event did not come from an HTTP request.
type AuditLog struct {
	ID          string                 `json:"id"`
	OccurredAt  time.Time              `json:"occurred_at"`
	WorkspaceID string                 `json:"workspace_id,omitempty"`
	Action      string                 `json:"action"`
	Category    string                 `json:"category"`
	Outcome     string                 `json:"outcome"`
	StatusCode  int                    `json:"status_code,omitempty"`
	ActorType   string                 `json:"actor_type"`
	ActorID     string                 `json:"actor_id,omitempty"`
	ActorEmail  string                 `json:"actor_email,omitempty"`
	ActorName   string                 `json:"actor_name,omitempty"`
	ActorRole   string                 `json:"actor_role,omitempty"`
	AuthMethod  string                 `json:"auth_method,omitempty"`
	TargetType  string                 `json:"target_type,omitempty"`
	TargetID    string                 `json:"target_id,omitempty"`
	TargetName  string                 `json:"target_name,omitempty"`
	IPAddress   string                 `json:"ip_address,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	RequestID   string                 `json:"request_id,omitempty"`
	Changes     map[string]AuditChange `json:"changes,omitempty"`
	Metadata    map[string]any         `json:"metadata,omitempty"`
}

// AuditRecord is the request-scoped accumulator. The audit middleware creates
// one per request and puts it in the context; the auth layer names the actor,
// services add the target and the changes, and the middleware turns it into
// an AuditLog once the response is written.
//
// Every method is safe on a nil receiver, so a service can write
// AuditFromContext(ctx).SetTarget(...) without asking whether there is a
// request at all — a background job, a test or an internal call simply has no
// record and the call is a no-op.
type AuditRecord struct {
	mu            sync.Mutex
	finalized     bool
	skipped       bool
	forcedOutcome string
	event         AuditLog
}

// NewAuditRecord starts a record with what the middleware knows before the
// handler runs.
func NewAuditRecord(ipAddress, userAgent, requestID string) *AuditRecord {
	return &AuditRecord{event: AuditLog{
		IPAddress: ipAddress,
		UserAgent: userAgent,
		RequestID: requestID,
		ActorType: AuditActorAnonymous,
	}}
}

// SetActorClaims records what the JWT says: the id and whether it is a user
// session or an API key. Called by the auth middleware; the email and name
// arrive later, from whichever service resolves the user.
func (r *AuditRecord) SetActorClaims(userID, userType string) {
	if r == nil || userID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finalized {
		return
	}
	r.event.ActorID = userID
	switch userType {
	case string(UserTypeAPIKey):
		r.event.ActorType = AuditActorAPIKey
		r.event.AuthMethod = AuditAuthAPIKey
	default:
		r.event.ActorType = AuditActorUser
		if r.event.AuthMethod == "" {
			r.event.AuthMethod = AuditAuthSession
		}
	}
}

// SetActor records the resolved user and, when known, their membership.
// isRoot marks a platform administrator (ROOT_EMAIL) whatever their
// membership row says.
func (r *AuditRecord) SetActor(user *User, membership *UserWorkspace, isRoot bool) {
	if r == nil || user == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finalized {
		return
	}
	r.event.ActorID = user.ID
	r.event.ActorEmail = user.Email
	r.event.ActorName = user.Name
	if user.Type == UserTypeAPIKey {
		r.event.ActorType = AuditActorAPIKey
		r.event.AuthMethod = AuditAuthAPIKey
	} else {
		r.event.ActorType = AuditActorUser
		if r.event.AuthMethod == "" {
			r.event.AuthMethod = AuditAuthSession
		}
	}
	switch {
	case isRoot:
		r.event.ActorRole = AuditRoleRoot
	case membership != nil && membership.Role != "":
		r.event.ActorRole = membership.Role
	}
	if membership != nil && membership.WorkspaceID != "" && r.event.WorkspaceID == "" {
		r.event.WorkspaceID = membership.WorkspaceID
	}
}

// SetWorkspace names the workspace the action belongs to. Services override
// whatever the middleware guessed from the request body.
func (r *AuditRecord) SetWorkspace(workspaceID string) {
	if r == nil || workspaceID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.finalized {
		r.event.WorkspaceID = workspaceID
	}
}

// SetTarget names what the action was done to. An empty argument keeps
// whatever was already known for that field.
func (r *AuditRecord) SetTarget(targetType, targetID, targetName string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finalized {
		return
	}
	if targetType != "" {
		r.event.TargetType = targetType
	}
	if targetID != "" {
		r.event.TargetID = targetID
	}
	if targetName != "" {
		r.event.TargetName = targetName
	}
}

// SetChanges attaches a before/after map. Callers pass the output of AuditDiff
// or a map they redacted themselves; the recorder redacts again before writing,
// so a caller who forgets cannot leak a secret.
func (r *AuditRecord) SetChanges(changes map[string]AuditChange) {
	if r == nil || len(changes) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.finalized {
		r.event.Changes = changes
	}
}

// AddMetadata attaches one key to the row's free-form metadata.
func (r *AuditRecord) AddMetadata(key string, value any) {
	if r == nil || key == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finalized {
		return
	}
	if r.event.Metadata == nil {
		r.event.Metadata = make(map[string]any)
	}
	r.event.Metadata[key] = value
}

// SetAuthMethod names how the actor authenticated when it is not the session
// or API-key default: a login records magic_code, sso or root_password.
func (r *AuditRecord) SetAuthMethod(method string) {
	if r == nil || method == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.finalized {
		r.event.AuthMethod = method
	}
}

// Fail marks the outcome as a failure whatever status the response carries,
// with a reason that only the log sees. A sign-in for an unknown address
// answers 200 on purpose so the endpoint cannot be used to enumerate accounts;
// the audit row still says unknown_email.
func (r *AuditRecord) Fail(reason string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finalized {
		return
	}
	r.forcedOutcome = AuditOutcomeFailure
	if reason != "" {
		if r.event.Metadata == nil {
			r.event.Metadata = make(map[string]any)
		}
		r.event.Metadata["reason"] = reason
	}
}

// Failed reports whether a service marked the request as failed, whatever the
// response status says. The middleware uses it to keep a 401 that carries a
// reason — a wrong code, a bad signature — while dropping the anonymous ones.
func (r *AuditRecord) Failed() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.forcedOutcome != ""
}

// HasTargetID reports whether something already named the target, so a
// response-derived id does not overwrite what a service knew better.
func (r *AuditRecord) HasTargetID() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.event.TargetID != ""
}

// Skip tells the middleware not to record this request after all.
func (r *AuditRecord) Skip() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skipped = true
}

// Finalize closes the record and returns its event. Enrichment after this
// point — a handler goroutine that outlives the response — is ignored. The
// outcome is the forced one when Fail was called, else derived from the status.
func (r *AuditRecord) Finalize(statusCode int) (event AuditLog, skipped bool) {
	if r == nil {
		return AuditLog{}, true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalized = true
	if r.skipped {
		return AuditLog{}, true
	}
	event = r.event
	event.StatusCode = statusCode
	event.Outcome = AuditOutcomeFromStatus(statusCode)
	if r.forcedOutcome != "" {
		event.Outcome = r.forcedOutcome
	}
	if event.Changes != nil {
		event.Changes = maps.Clone(event.Changes)
	}
	if event.Metadata != nil {
		event.Metadata = maps.Clone(event.Metadata)
	}
	return event, false
}

// AuditOutcomeFromStatus maps an HTTP status to an outcome: 2xx and 3xx
// succeeded, 402 and 403 were refused, everything else failed.
func AuditOutcomeFromStatus(status int) string {
	switch {
	case status >= 200 && status < 400:
		// A redirect is how the OIDC callback and a few console flows answer;
		// it is not a failure.
		return AuditOutcomeSuccess
	case status == 402 || status == 403:
		return AuditOutcomeDenied
	default:
		return AuditOutcomeFailure
	}
}

// AuditRecordKey is the context key the middleware stores the record under.
const AuditRecordKey contextKey = "audit_record"

// WithAuditRecord attaches a record to the context.
func WithAuditRecord(ctx context.Context, record *AuditRecord) context.Context {
	return context.WithValue(ctx, AuditRecordKey, record)
}

// AuditFromContext returns the request's record, or nil when the context did
// not come from an audited HTTP request. Nil is usable: every method on it is
// a no-op.
func AuditFromContext(ctx context.Context) *AuditRecord {
	if ctx == nil {
		return nil
	}
	record, _ := ctx.Value(AuditRecordKey).(*AuditRecord)
	return record
}

// auditSecretKey matches keys whose values must never reach the log. Matched
// case-insensitively against the JSON key alone, so smtp_password,
// OIDCClientSecret, access_key_id, api_key and SecretKey are all caught. "key"
// is anchored to the end of the key on purpose: api_key_email is an address,
// not a secret, and must stay readable.
var auditSecretKey = regexp.MustCompile(`(?i)(secret|password|passwd|token|credential|private|signature|access_key|tls_key|tlskey|key$)`)

// auditIgnoredKeys are bookkeeping fields that change on every save and would
// only add noise to a diff.
var auditIgnoredKeys = map[string]struct{}{
	"created_at": {},
	"updated_at": {},
	"version":    {},
}

// IsAuditSecretKey reports whether a JSON key names a secret.
func IsAuditSecretKey(key string) bool {
	return auditSecretKey.MatchString(key)
}

// AuditDiff compares two values field by field after marshalling both to JSON,
// and returns the fields that differ as before/after pairs. Comparison is at
// the top level: a nested object or array that changed is reported whole.
// Keys that look like secrets are kept as changes but both sides become
// AuditRedactedValue; values above AuditMaxValueBytes become a size marker.
// Fields tagged json:"-" are invisible to it, which is what keeps a
// workspace's decoded secret key out of the log. Returns nil when nothing
// differs.
func AuditDiff(before, after any) map[string]AuditChange {
	beforeMap := toAuditMap(before)
	afterMap := toAuditMap(after)

	keys := make(map[string]struct{}, len(beforeMap)+len(afterMap))
	for k := range beforeMap {
		keys[k] = struct{}{}
	}
	for k := range afterMap {
		keys[k] = struct{}{}
	}

	var changes map[string]AuditChange
	for key := range keys {
		if _, ignored := auditIgnoredKeys[key]; ignored {
			continue
		}
		oldValue, hadOld := beforeMap[key]
		newValue, hadNew := afterMap[key]
		if hadOld && hadNew && reflect.DeepEqual(oldValue, newValue) {
			continue
		}
		if changes == nil {
			changes = make(map[string]AuditChange)
		}
		changes[key] = AuditChange{
			Old: RedactAuditValue(key, oldValue),
			New: RedactAuditValue(key, newValue),
		}
	}
	return changes
}

// RedactAuditValue returns the value as the log may store it: redacted when
// the key names a secret, recursively redacted inside nested maps, and
// replaced by a size marker when too large.
func RedactAuditValue(key string, value any) any {
	if value == nil {
		return nil
	}
	if IsAuditSecretKey(key) {
		return AuditRedactedValue
	}
	value = redactAuditDeep(value)
	if encoded, err := json.Marshal(value); err == nil && len(encoded) > AuditMaxValueBytes {
		return fmt.Sprintf("[changed, %d bytes]", len(encoded))
	}
	return value
}

// RedactAuditChanges applies RedactAuditValue to every entry, for callers
// that assembled a change map by hand. The recorder runs it on every row.
func RedactAuditChanges(changes map[string]AuditChange) map[string]AuditChange {
	if len(changes) == 0 {
		return changes
	}
	out := make(map[string]AuditChange, len(changes))
	for key, change := range changes {
		out[key] = AuditChange{
			Old: RedactAuditValue(key, change.Old),
			New: RedactAuditValue(key, change.New),
		}
	}
	return out
}

// RedactAuditMetadata redacts secret-looking keys inside metadata.
func RedactAuditMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return metadata
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = RedactAuditValue(key, value)
	}
	return out
}

func redactAuditDeep(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			if IsAuditSecretKey(k) {
				out[k] = AuditRedactedValue
				continue
			}
			out[k] = redactAuditDeep(v)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, v := range typed {
			out[i] = redactAuditDeep(v)
		}
		return out
	default:
		return value
	}
}

// toAuditMap marshals any value to a generic map so that structs, maps and
// pointers all compare the same way. Non-objects (nil, scalars) become an
// empty map.
func toAuditMap(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if reflect.ValueOf(value).Kind() == reflect.Ptr && reflect.ValueOf(value).IsNil() {
		return map[string]any{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(encoded, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

// AuditLogFilter is the list, export and stream selection. Scope decides the
// baseline: a workspace scope needs WorkspaceID and returns that workspace's
// rows; the deployment scope (root only) returns every row, optionally
// narrowed to one workspace.
type AuditLogFilter struct {
	Scope       string     `json:"scope,omitempty"`
	WorkspaceID string     `json:"workspace_id,omitempty"`
	Actions     []string   `json:"actions,omitempty"`
	Categories  []string   `json:"categories,omitempty"`
	ActorID     string     `json:"actor_id,omitempty"`
	ActorEmail  string     `json:"actor_email,omitempty"`
	ActorType   string     `json:"actor_type,omitempty"`
	Outcome     string     `json:"outcome,omitempty"`
	TargetType  string     `json:"target_type,omitempty"`
	TargetID    string     `json:"target_id,omitempty"`
	IPAddress   string     `json:"ip_address,omitempty"`
	From        *time.Time `json:"from,omitempty"`
	To          *time.Time `json:"to,omitempty"`
	Cursor      string     `json:"cursor,omitempty"`
	Limit       int        `json:"limit,omitempty"`
}

// FromURLParams reads the filter from a GET query string. Lists are
// comma-separated; timestamps are RFC3339.
func (f *AuditLogFilter) FromURLParams(values url.Values) error {
	f.Scope = values.Get("scope")
	f.WorkspaceID = values.Get("workspace_id")
	f.Actions = splitAuditList(values.Get("actions"))
	f.Categories = splitAuditList(values.Get("categories"))
	f.ActorID = values.Get("actor_id")
	f.ActorEmail = values.Get("actor_email")
	f.ActorType = values.Get("actor_type")
	f.Outcome = values.Get("outcome")
	f.TargetType = values.Get("target_type")
	f.TargetID = values.Get("target_id")
	f.IPAddress = values.Get("ip_address")
	f.Cursor = values.Get("cursor")

	if raw := values.Get("from"); raw != "" {
		from, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return fmt.Errorf("from must be an RFC3339 timestamp")
		}
		f.From = &from
	}
	if raw := values.Get("to"); raw != "" {
		to, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return fmt.Errorf("to must be an RFC3339 timestamp")
		}
		f.To = &to
	}
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("limit must be an integer")
		}
		f.Limit = limit
	}
	return f.Validate()
}

// Validate fills defaults and rejects what the repository could not query.
// The limit is clamped rather than refused, like every other list endpoint.
func (f *AuditLogFilter) Validate() error {
	if f.Scope == "" {
		f.Scope = AuditScopeWorkspace
	}
	switch f.Scope {
	case AuditScopeWorkspace:
		if f.WorkspaceID == "" {
			return fmt.Errorf("workspace_id is required")
		}
	case AuditScopeDeployment:
	default:
		return fmt.Errorf("scope must be %q or %q", AuditScopeWorkspace, AuditScopeDeployment)
	}
	switch f.Outcome {
	case "", AuditOutcomeSuccess, AuditOutcomeFailure, AuditOutcomeDenied:
	default:
		return fmt.Errorf("outcome must be success, failure or denied")
	}
	switch f.ActorType {
	case "", AuditActorUser, AuditActorAPIKey, AuditActorSystem, AuditActorAnonymous:
	default:
		return fmt.Errorf("actor_type must be user, api_key, system or anonymous")
	}
	if f.From != nil && f.To != nil && f.To.Before(*f.From) {
		return fmt.Errorf("to must not be before from")
	}
	if f.Cursor != "" {
		if _, _, err := DecodeAuditCursor(f.Cursor); err != nil {
			return fmt.Errorf("invalid cursor")
		}
	}
	if f.Limit <= 0 {
		f.Limit = AuditLogDefaultListLimit
	}
	if f.Limit > AuditLogMaxListLimit {
		f.Limit = AuditLogMaxListLimit
	}
	return nil
}

func splitAuditList(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

// EncodeAuditCursor and DecodeAuditCursor carry the keyset position
// (occurred_at, id) the way message history does: base64 of
// "RFC3339Nano~id".
func EncodeAuditCursor(occurredAt time.Time, id string) string {
	return base64.StdEncoding.EncodeToString([]byte(occurredAt.UTC().Format(time.RFC3339Nano) + "~" + id))
}

func DecodeAuditCursor(cursor string) (time.Time, string, error) {
	decoded, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor encoding")
	}
	parts := strings.SplitN(string(decoded), "~", 2)
	if len(parts) != 2 || parts[1] == "" {
		return time.Time{}, "", fmt.Errorf("invalid cursor format")
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor timestamp")
	}
	return occurredAt, parts[1], nil
}

// ListAuditLogsResponse is one page.
type ListAuditLogsResponse struct {
	Logs       []*AuditLog `json:"logs"`
	NextCursor string      `json:"next_cursor,omitempty"`
	HasMore    bool        `json:"has_more"`
}

// ExportAuditLogsRequest is the POST body of auditLogs.export: the same filter
// plus a format. The cursor is ignored; an export always starts from the top.
type ExportAuditLogsRequest struct {
	AuditLogFilter
	Format string `json:"format"`
}

// Validate normalises the format and the filter.
func (r *ExportAuditLogsRequest) Validate() error {
	switch r.Format {
	case "":
		r.Format = AuditExportFormatCSV
	case AuditExportFormatCSV, AuditExportFormatNDJSON:
	default:
		return fmt.Errorf("format must be csv or ndjson")
	}
	r.Cursor = ""
	return r.AuditLogFilter.Validate()
}

// AuditActionDescriptor is one catalogue entry, served to the console for its
// filter and to the docs for the event table.
type AuditActionDescriptor struct {
	Action     string `json:"action"`
	Category   string `json:"category"`
	TargetType string `json:"target_type,omitempty"`
}

// AuditLogSettings is the per-workspace retention. RetentionDays nil means
// "the deployment default"; 0 means keep forever; anything else is bounded.
type AuditLogSettings struct {
	RetentionDays *int `json:"retention_days,omitempty"`
}

// ValidateForSave rejects a retention the purge could not honour.
func (s *AuditLogSettings) ValidateForSave() error {
	if s == nil || s.RetentionDays == nil {
		return nil
	}
	return ValidateAuditRetentionDays(*s.RetentionDays)
}

// ValidateAuditRetentionDays is shared by the workspace setting and the
// deployment default: 0 (forever) or 30–3650.
func ValidateAuditRetentionDays(days int) error {
	if days == 0 {
		return nil
	}
	if days < AuditLogRetentionMinDays || days > AuditLogRetentionMaxDays {
		return fmt.Errorf("retention_days must be 0 (keep forever) or between %d and %d", AuditLogRetentionMinDays, AuditLogRetentionMaxDays)
	}
	return nil
}

// EffectiveRetentionDays resolves nil to the deployment default. 0 stays 0.
func (s *AuditLogSettings) EffectiveRetentionDays(defaultDays int) int {
	if s == nil || s.RetentionDays == nil {
		return defaultDays
	}
	return *s.RetentionDays
}

// SetAuditLogSettingsRequest is the body of workspaces.setAuditLogSettings.
type SetAuditLogSettingsRequest struct {
	WorkspaceID string           `json:"workspace_id"`
	Settings    AuditLogSettings `json:"settings"`
}

// Validate mirrors SetWebAnalyticsSettingsRequest.Validate.
func (r *SetAuditLogSettingsRequest) Validate() (workspaceID string, settings *AuditLogSettings, err error) {
	if r.WorkspaceID == "" {
		return "", nil, fmt.Errorf("workspace_id is required")
	}
	if err := r.Settings.ValidateForSave(); err != nil {
		return "", nil, err
	}
	copied := r.Settings
	return r.WorkspaceID, &copied, nil
}

// AuditLogRepository is the system-database store. Every row is written once;
// the only deletes are the retention purges, which go through the SQL
// functions the guard trigger lets through.
type AuditLogRepository interface {
	Insert(ctx context.Context, log *AuditLog) error
	// List returns one page, newest first, and the cursor of the next one ("" at
	// the end).
	List(ctx context.Context, filter AuditLogFilter) ([]*AuditLog, string, error)
	// GetByID returns an error wrapping ErrAuditLogNotFound for a missing row.
	GetByID(ctx context.Context, id string) (*AuditLog, error)
	// Stream walks every row the filter selects, newest first, calling fn for
	// each until it returns an error, which Stream returns unwrapped.
	Stream(ctx context.Context, filter AuditLogFilter, fn func(*AuditLog) error) error
	// Purge deletes one workspace's rows older than before, or the
	// deployment-level rows when workspaceID is nil.
	Purge(ctx context.Context, workspaceID *string, before time.Time) (int64, error)
	// PurgeOrphans deletes rows older than before whose workspace is not in
	// keep. Callers must not pass an empty keep unless every workspace is gone.
	PurgeOrphans(ctx context.Context, keep []string, before time.Time) (int64, error)
}

// AuditLogRecorder is the write port. It never returns an error: the audit
// log must never be the reason a user's action fails, so a write that cannot
// happen is logged and dropped.
type AuditLogRecorder interface {
	Record(ctx context.Context, log *AuditLog)
}

// NoopAuditLogRecorder records nothing, for tests and for code paths that
// deliberately run without a log.
type NoopAuditLogRecorder struct{}

func (NoopAuditLogRecorder) Record(context.Context, *AuditLog) {}

// AuditLogService is the read side plus export.
type AuditLogService interface {
	List(ctx context.Context, filter AuditLogFilter) (*ListAuditLogsResponse, error)
	Get(ctx context.Context, scope, workspaceID, id string) (*AuditLog, error)
	// Export writes the selection to w in the requested format, calling flush
	// after each batch, and stops at AuditLogExportMaxRows.
	Export(ctx context.Context, req ExportAuditLogsRequest, w io.Writer, flush func()) (rows int64, truncated bool, err error)
	Actions() []AuditActionDescriptor
}
