package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/license"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// GracePeriod is how long a deployment keeps everything its key granted after that key has
// expired.
//
// It lives here, in code, and deliberately NOT in the signed payload. A key already in a
// customer's hands cannot be edited, so a grace period baked into it would freeze the policy
// for a year across every installation; as a constant it can be lengthened — or shortened —
// in a release, and the change reaches every deployment on upgrade without re-minting a
// single key.
//
// Thirty days rather than a fortnight because Stripe's dunning makes eight retry attempts
// spread over about two weeks. A grace period shorter than the retry schedule would degrade
// a customer whose card is in the middle of succeeding.
const GracePeriod = 30 * 24 * time.Hour

// LicenseSettingKey is the row in the system settings table that holds the raw key. One row,
// the envelope verbatim: everything else about a licence is derived from the signature, so
// storing anything alongside it would be a second source of truth that could disagree.
const LicenseSettingKey = "license_key"

// licenseLoadTimeout bounds each settings read this service performs. Construction happens
// on the boot path and every retry after that happens inline on a request path, so an
// unreachable database must cost a few seconds and a free-tier deployment, never a process
// that never finishes starting or a request that never finishes serving.
const licenseLoadTimeout = 5 * time.Second

// The retry schedule for a settings read that failed for a reason that is not an answer.
//
// "No row is stored" is a settled answer and is cached forever. "I could not find out" —
// connection refused, a timeout, a permissions error — is not an answer at all, and caching
// it for the lifetime of the process is how a paying customer whose database blinked during
// a rolling restart runs Community until somebody restarts them again. With OIDC on, that
// means a permanently read-only console for a reason nobody can diagnose from the outside.
//
// The retry is lazy: it happens on the next Entitlements call that finds the question still
// unanswered and the backoff elapsed. There is deliberately no ticker and no refresh loop —
// the plan removed those on purpose, and this is not one. Nothing is polled; a deployment
// that never asks never reads, and a deployment that asks a thousand times a second still
// reads at most once per backoff window.
//
// The backoff doubles from the first value to the second so that a database which is down
// for an hour costs a handful of connection attempts rather than one per request.
const (
	licenseRetryInitialBackoff = 30 * time.Second
	licenseRetryMaxBackoff     = 5 * time.Minute
)

// Where the raw key was FOUND. Reported in the startup log as key_source because "I pasted a
// key and nothing happened" is otherwise indistinguishable from "the key is wrong", and the
// two have completely different remedies.
//
// It answers "was a key seen at all", never "did it work" — the state field answers that, and
// keeping the two apart is what makes either worth reading. See keySource on the service.
const (
	licenseSourceNone        = "none"
	licenseSourceEnvironment = "environment"
	licenseSourceDatabase    = "database"
)

// The refusals SetKey can return. Both leave the stored key and the in-memory claims exactly
// as they were: a rejected paste never costs a deployment the licence it already had.
var (
	// ErrLicenseKeyEmpty means the caller submitted nothing. Clearing a licence is
	// deliberately not expressible here — the only way to reach SetKey is a root user
	// pasting a key into the console, where an empty box is a mistake, never an intent.
	// A key is replaced by pasting its successor.
	ErrLicenseKeyEmpty = errors.New("licence key is empty")

	// ErrLicenseKeyLockedByEnv means NOTIFUSE_LICENSE_KEY is set, so the pasted key could
	// never take effect. Storing it anyway would be worse than refusing: the console would
	// report success while the deployment kept running on the environment's key, and the
	// discrepancy would only surface at the next restart, to somebody else.
	ErrLicenseKeyLockedByEnv = errors.New("licence key is set by the NOTIFUSE_LICENSE_KEY environment variable and cannot be changed from the console")
)

// licenseStore is the slice of the system settings table this service needs. Declared here,
// in the consuming package, so the dependency stays visible and narrow — the licence needs
// one key/value row and no schema of its own.
type licenseStore interface {
	Get(ctx context.Context, key string) (*domain.Setting, error)
	Set(ctx context.Context, key, value string) error
}

// LicenseServiceConfig is what the service is built from.
type LicenseServiceConfig struct {
	// SettingRepo reads and writes the stored key. May be nil: a deployment whose licence
	// comes from the environment does not need it, and a nil store is treated as "no key
	// in the database" rather than as an error.
	SettingRepo licenseStore

	// EnvKey is the raw value of NOTIFUSE_LICENSE_KEY. It wins over the database.
	EnvKey string

	// OIDCEnabled is the RESOLVED value of cfg.OIDC.Enabled, not the raw environment
	// variable. config/oidc.go resolves it env-wins-else-database, so a deployment that
	// switched SSO on from the console has it true with no environment variable in sight.
	//
	// Nothing here gates on it. It exists so the startup line can say whether SSO is
	// configured on a deployment whose licence does not cover it — the one state where an
	// operator sees a sign-in button disappear and needs to know why. The gate itself lives
	// in OIDCService.IsEnabled, which asks this service for the entitlement directly.
	OIDCEnabled bool

	Logger logger.Logger
}

// LicenseService answers what this deployment is licensed for.
//
// The whole design is three sentences. The key is read once, at construction, from the
// environment or the settings table, and verified once against the public key compiled into
// this binary. The claims are then held under a mutex and re-read, never re-parsed, until
// somebody installs a different key. The lifecycle state — active, grace, expired — is
// derived on every read by comparing the clock against the key's expiry.
//
// There is no ticker and no background goroutine. A refresh loop would buy nothing (the
// claims are immutable once verified) and cost something real: a window during which the
// answer is stale, plus a goroutine that can die without anybody noticing.
//
// The one thing that does happen more than once is a read the database refused to answer.
// "No key is stored" is an answer and is cached for the life of the process; "I could not
// find out" is not, and is retried lazily, on the next Entitlements call past a doubling
// backoff. That is retry-on-failure, not polling: a healthy deployment reads exactly once,
// and a deployment whose database was down during a rolling restart gets its licence back
// without anybody restarting it. See licenseRetryInitialBackoff.
//
// Every failure — no key, a malformed key, a bad signature, a schema this build does not
// know, a database that is down, a panic anywhere in the chain — degrades this deployment to
// domain.CommunityEntitlements() and is logged once at WARN. Licence handling never panics,
// never blocks a send, never refuses a login and never deletes anything.
type LicenseService struct {
	store       licenseStore
	envKey      string
	oidcEnabled bool
	logger      logger.Logger

	// parse, now and hasTrustedKey are injected so the behaviour that matters here —
	// state derivation across the expiry and grace boundaries, degradation on each class
	// of parse failure, and what a binary carrying the placeholder signing key says about
	// itself — is testable without a signing key and under either build tag. Production
	// always gets license.Parse, time.Now and license.HasTrustedKey; pkg/license does the
	// same thing internally with parseAt.
	parse         func(raw string) (*license.Claims, error)
	now           func() time.Time
	hasTrustedKey func() bool

	// reloadMu serialises everything that can decide what this deployment is licensed for:
	// the lazy retry and SetKey. It is separate from mu on purpose — holding the state lock
	// across a settings read would block every gate in the process for as long as the
	// database takes to time out.
	//
	// The two callers take it differently, and the asymmetry is the design. A retry uses
	// TryLock and gives up, because a gate must never wait on a database. SetKey uses a
	// blocking Lock, because it is a root-only action that must not be silently undone by a
	// read that started before it — see SetKey for the failure that produced this.
	//
	// Lock order is reloadMu → store → mu, on every path. Nothing takes reloadMu while
	// holding mu.
	reloadMu sync.Mutex

	mu     sync.RWMutex
	claims *license.Claims

	// keySource is where a raw key was FOUND, not where the resolved grant came from, and
	// the distinction is the whole point of the field.
	//
	// It used to be set by markResolved, which is called with licenseSourceNone on every
	// verification failure — so a deployment that set NOTIFUSE_LICENSE_KEY and had it
	// refused logged source: "none", which reads as "your variable was never seen". That is
	// exactly the confusion the constants below say this field exists to prevent, and it
	// also made the placeholder-key ERROR escalation unreachable: it tests this value, and
	// on a placeholder build every key fails to verify, so it was always "none".
	//
	// Set before verification and left alone by markResolved. "none" now means one thing
	// only: no key was supplied anywhere.
	keySource string

	// resolved records whether the question "is a key stored here" has an answer yet.
	// False means the last attempt failed for a reason that was not an answer, and the
	// next Entitlements call past retryAt tries again. It is NOT "a key was found":
	// finding no key is an answer, and a settled one.
	resolved bool
	retryAt  time.Time
	backoff  time.Duration
}

// The provider port every gate consumes. Asserted here so a signature drift is a compile
// error in this file rather than a wiring error in app.go.
var _ domain.EntitlementProvider = (*LicenseService)(nil)

// NewLicenseService builds the service and resolves the licence immediately.
//
// Resolution happens at construction rather than behind a separate Load call so that no
// caller can ever observe a service that has not read its key yet — an intermediate state
// whose only possible reading is "unlicensed", which is precisely the wrong answer to give a
// paying customer for however long the gap lasts.
func NewLicenseService(cfg LicenseServiceConfig) *LicenseService {
	return newLicenseService(cfg, license.Parse, time.Now, license.HasTrustedKey)
}

// newLicenseService is NewLicenseService with the verifier, the clock and the compiled-key
// probe passed in.
func newLicenseService(
	cfg LicenseServiceConfig,
	parse func(string) (*license.Claims, error),
	now func() time.Time,
	hasTrustedKey func() bool,
) *LicenseService {
	s := &LicenseService{
		store:         cfg.SettingRepo,
		envKey:        strings.TrimSpace(cfg.EnvKey),
		oidcEnabled:   cfg.OIDCEnabled,
		logger:        cfg.Logger,
		parse:         parse,
		now:           now,
		hasTrustedKey: hasTrustedKey,
		keySource:     licenseSourceNone,
	}

	// The two calls after load() are inside the recover too, and that is the point of
	// putting it here rather than only in load: the file's promise is that an unpredicted
	// bug in licence code costs a deployment its paid features, never its ability to
	// start. warnIfNoTrustedKey and logResolved read licence state and hand it to a logger
	// this package does not own — a nil map, a logger that panics on a field type — and a
	// panic in either would take the process down at boot, on the one path where nothing
	// is wrong with the deployment itself.
	s.load()
	// load() recovers its own panics and settles the question itself. The two calls below
	// only READ state and hand it to a logger, so a panic in them must not touch the claims
	// load() just installed: the earlier shape of this block wrapped all three in one
	// recover that called markResolved(nil), which turned a broken logger into a
	// deployment silently on the free tier for its whole lifetime, with nothing logged
	// because the logger was the thing that broke.
	s.logQuietly(func() {
		s.warnIfNoTrustedKey()
		s.logResolved()
	})

	return s
}

// logQuietly runs fn, which may only log, and swallows a panic from it. Nothing about the
// licence state is changed on either path; the deployment simply goes without that line.
func (s *LicenseService) logQuietly(fn func()) {
	defer func() { _ = recover() }()
	fn()
}

// Entitlements returns what this deployment may currently do.
//
// It takes no context and returns no error because gates call it inline on request paths
// where neither is available and neither would help: there is no failure to report, only a
// grant to hand back, and the worst grant it can hand back is the free tier.
func (s *LicenseService) Entitlements() domain.Entitlements {
	s.retryIfUnresolved()
	return s.currentEntitlements()
}

// currentEntitlements reads the resolved grant without attempting a retry. It exists so that
// the logging path, which runs immediately after a load, cannot re-enter the retry it was
// called from.
func (s *LicenseService) currentEntitlements() domain.Entitlements {
	s.mu.RLock()
	claims := s.claims
	s.mu.RUnlock()

	return entitlementsFrom(claims, s.now())
}

// retryIfUnresolved re-attempts the settings read when the last one failed for a reason that
// was not an answer and the backoff has elapsed.
//
// The fast path — the only one a healthy deployment ever takes — is a single read-locked
// look at a bool, because this runs inline on request paths. Everything else is guarded so
// that a database which is down cannot turn into a hot loop: at most one goroutine attempts
// at a time (TryLock, never Lock, so nobody queues behind a five-second timeout), and at
// most one attempt happens per backoff window however many callers ask.
func (s *LicenseService) retryIfUnresolved() {
	if !s.retryDue() {
		return
	}

	if !s.reloadMu.TryLock() {
		// Another goroutine is already reading. Answering from the current state is
		// exactly right: a gate must never block on a database.
		return
	}

	// Re-check under the serialising lock: the goroutine that held it may have resolved
	// the question while this one was deciding to try.
	if !s.retryDue() {
		s.reloadMu.Unlock()
		return
	}

	// The read itself runs off the request goroutine. It is bounded by
	// licenseLoadTimeout, and that bound used to be paid by whichever request happened to
	// win the TryLock while the database was down — a /config.js fetch from the login
	// page, a template save — stalled for up to five seconds once per backoff window,
	// against this file's own rule that a gate never blocks on a database. The caller is
	// answered now from the current state, exactly as a caller that lost the TryLock is;
	// the lock is held until the read completes so no second read can start.
	go func() {
		defer s.reloadMu.Unlock()
		s.load()

		// A licence that only appeared on the third attempt deserves the same startup line
		// as one that was there at boot — otherwise the log says Community and the
		// deployment is not, which is the discrepancy support would chase first.
		s.mu.RLock()
		resolved := s.resolved
		s.mu.RUnlock()
		if resolved {
			s.logQuietly(s.logResolved)
		}
	}()
}

// retryDue reports whether the question is still unanswered and the backoff has elapsed.
func (s *LicenseService) retryDue() bool {
	s.mu.RLock()
	resolved, retryAt := s.resolved, s.retryAt
	s.mu.RUnlock()

	return !resolved && !s.now().Before(retryAt)
}

// SetKey validates a licence key, stores it, and makes it current.
//
// The order is load-bearing: nothing is written and nothing in memory changes until the key
// has verified, so pasting a typo or a key for a different signing authority leaves the
// deployment on exactly the licence it had a moment earlier.
//
// It refuses when NOTIFUSE_LICENSE_KEY is set. See ErrLicenseKeyLockedByEnv — reporting
// success for a write that could not take effect is the worse of the two failures.
//
// It holds reloadMu for the whole verify → write → swap, and that is not tidiness. mu alone
// orders this against nothing: the lazy retry holds reloadMu across a settings read bounded
// only by licenseLoadTimeout, so a read ISSUED BEFORE this paste can LAND AFTER it and call
// markResolved(nil, none) with the row as it was a moment earlier — wiping the key that was
// just bought. That stale answer also closes the question (resolved = true, backoff cleared),
// so nothing ever repairs it: the deployment runs Community until somebody restarts the
// process, while the correct key sits in the settings table and the console has already been
// told, with a 200 carrying the right entitlements, that the paste worked.
//
// The cost is that a paste can wait behind an in-flight read. It is bounded by
// licenseLoadTimeout, it happens only while the database is unwell, and this is a root-only
// action a deployment performs about once a year. The lock order is reloadMu → store → mu on
// both paths, so there is no cycle.
//
// Holding it across verify as well as the write is what makes two simultaneous pastes agree:
// unserialised, both verify, both write, and the last markResolved wins independently of the
// last store.Set — leaving the process running on one key while the row names the other.
func (s *LicenseService) SetKey(ctx context.Context, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ErrLicenseKeyEmpty
	}
	if s.envKey != "" {
		return ErrLicenseKeyLockedByEnv
	}
	if s.store == nil {
		return fmt.Errorf("failed to set %s: no settings store is configured", LicenseSettingKey)
	}

	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()

	claims, err := s.verify(raw)
	if err != nil {
		return fmt.Errorf("invalid licence key: %w", err)
	}

	if err := s.store.Set(ctx, LicenseSettingKey, raw); err != nil {
		return fmt.Errorf("failed to set %s: %w", LicenseSettingKey, err)
	}

	s.mu.Lock()
	s.keySource = licenseSourceDatabase
	s.mu.Unlock()
	s.markResolved(claims)
	s.logResolved()

	return nil
}

// load resolves the key from the environment or the settings table and verifies it. It runs
// once at construction and again on each lazy retry.
//
// The recover is the outer guard of the fail-safe promise. Everything inside is already
// written not to panic, which is exactly why a panic here would be a bug nobody predicted —
// and an unpredicted bug in licence code must cost a deployment its paid features, never its
// ability to start. A panic settles the question rather than scheduling a retry: an
// unpredicted bug will reproduce, and re-entering it once per request path is worse than
// running on the free tier.
func (s *LicenseService) load() {
	defer func() {
		if r := recover(); r != nil {
			s.warn("Licence handling failed; continuing on the free tier", map[string]interface{}{
				"panic": fmt.Sprint(r),
			})
			s.markResolved(nil)
		}
	}()

	raw, source, answered := s.resolveRawKey()
	s.mu.Lock()
	s.keySource = source
	s.mu.Unlock()
	if !answered {
		// Not "there is no key" — "I could not find out". Leave the question open so a
		// database that comes back does not cost this deployment its licence until the
		// next restart.
		s.scheduleRetry()
		return
	}

	if raw == "" {
		s.markResolved(nil)
		return
	}

	claims, err := s.verify(raw)
	if err != nil {
		// One line, at WARN, naming the source. Every parse failure class lands here and
		// they are all the same outcome — the free tier — so the error string exists to
		// tell an operator which of "wrong key", "truncated paste" and "unreleased build"
		// they are looking at.
		//
		// This is a settled answer, not a transient one: the stored bytes will not verify
		// any better on a second reading, and retrying them would log the same line
		// forever.
		s.warn("Licence key could not be verified; continuing on the free tier", map[string]interface{}{
			"source": source,
			"error":  err.Error(),
		})
		s.markResolved(nil)
		return
	}

	s.markResolved(claims)
}

// markResolved installs an answer and closes the question. A nil claims value is a perfectly
// good answer — it is what "no key is stored here" looks like, and it is the state of every
// Community installation.
//
// It does not touch keySource. Where a key was found is a fact about the attempt, and it has
// to survive an attempt that failed — that is the only state in which anybody reads it.
func (s *LicenseService) markResolved(claims *license.Claims) {
	s.mu.Lock()
	s.claims = claims
	s.resolved = true
	s.retryAt = time.Time{}
	s.backoff = 0
	s.mu.Unlock()
}

// scheduleRetry leaves the question open and pushes the next attempt out, doubling the wait
// each time up to licenseRetryMaxBackoff.
func (s *LicenseService) scheduleRetry() {
	s.mu.Lock()
	switch {
	case s.backoff == 0:
		s.backoff = licenseRetryInitialBackoff
	case s.backoff < licenseRetryMaxBackoff:
		s.backoff *= 2
		if s.backoff > licenseRetryMaxBackoff {
			s.backoff = licenseRetryMaxBackoff
		}
	}
	s.resolved = false
	s.retryAt = s.now().Add(s.backoff)
	s.mu.Unlock()
}

// resolveRawKey returns the key to verify and where it came from.
//
// The environment wins, and when it is set the settings table is not even read. That is not
// an optimisation: it is what makes the precedence observable. A GitOps deployment whose key
// is declared in its manifest, and an air-gapped one where the database is restored from
// elsewhere, must both run on the key the operator can see in their configuration, not on
// whatever a previous restore left in a row.
func (s *LicenseService) resolveRawKey() (raw string, source string, answered bool) {
	if s.envKey != "" {
		return s.envKey, licenseSourceEnvironment, true
	}

	if s.store == nil {
		// No store is configured at all. That is a settled answer about this process, not
		// a failure to read one, so there is nothing to retry.
		return "", licenseSourceNone, true
	}

	ctx, cancel := context.WithTimeout(context.Background(), licenseLoadTimeout)
	defer cancel()

	setting, err := s.store.Get(ctx, LicenseSettingKey)
	if err != nil {
		// A missing row is the ordinary state of every Community installation: an answer,
		// and not one worth a line in anybody's logs.
		// errors.As, not a bare type assertion. A wrapped "no such row" would
		// otherwise read as "I could not find out", which is the one classification
		// that must not be got wrong: it leaves the question open and schedules a
		// retry forever, so every Community deployment — which is most of them —
		// would query the settings table once per backoff window for the life of the
		// process, to be told the same thing.
		var notFound *domain.ErrSettingNotFound
		if errors.As(err, &notFound) {
			return "", licenseSourceNone, true
		}

		// Anything else — a database that is down, a timeout, a permissions problem — is
		// not an answer. It is worth a line, it is still not worth failing over, and it
		// must not be cached as "no licence": that is how a transient blip costs a paying
		// customer their entitlements for the lifetime of the process.
		s.warn("Could not read the stored licence key; continuing on the free tier and retrying later", map[string]interface{}{
			"error": err.Error(),
		})
		return "", licenseSourceNone, false
	}

	raw = strings.TrimSpace(setting.Value)
	if raw == "" {
		return "", licenseSourceNone, true
	}

	return raw, licenseSourceDatabase, true
}

// verify parses one envelope, turning a panic into an ordinary error.
//
// pkg/license is written not to panic — it skips public-key slots of the wrong length
// precisely because ed25519.Verify panics on them — but this is the call that runs on a
// customer's server with input a customer supplied, and the belt is cheap.
func (s *LicenseService) verify(raw string) (claims *license.Claims, err error) {
	defer func() {
		if r := recover(); r != nil {
			claims = nil
			err = fmt.Errorf("licence key verification panicked: %v", r)
		}
	}()

	return s.parse(raw)
}

// logResolved writes the one line support will ask for first: what this deployment thinks its
// licence is. It names the state, the tier, the expiry, the features and whether SSO is
// switched on without a licence that covers it — never the key itself, which is a bearer
// credential.
func (s *LicenseService) logResolved() {
	if s.logger == nil {
		return
	}

	ent := s.currentEntitlements()

	s.mu.RLock()
	keySource := s.keySource
	s.mu.RUnlock()

	features := make([]string, 0, len(ent.Features))
	for _, f := range ent.Features {
		features = append(features, string(f))
	}

	tier := ent.Tier
	if tier == "" {
		tier = "community"
	}

	s.logger.WithFields(map[string]interface{}{
		"state":          string(ent.State),
		"tier":           tier,
		"key_source":     keySource,
		"org":            ent.Org,
		"expires_at":     formatLicenseExpiry(ent.ExpiresAt),
		"features":       strings.Join(features, ","),
		"max_workspaces": formatWorkspaceQuota(ent.MaxWorkspaces),
		// The single-sign-on button is configured on this deployment and hidden, because
		// the licence does not carry sso. It is the only licence state that is invisible
		// from the console — the other three gates announce themselves with a 402 the
		// moment somebody presses the control — so it is the one that has to be in the log.
		"sso_gated": s.oidcEnabled && !ent.Has(domain.FeatureSSO),
	}).Info("Licence resolved")
}

// warnIfNoTrustedKey says, loudly and once, that this binary cannot accept any licence key
// that has ever been minted.
//
// pubkey_prod.go ships a placeholder — thirty-two zero bytes — until a human generates the
// real signing pair, and a build carrying it refuses every key in existence with
// ErrNoTrustedKey. From the outside that is indistinguishable from "your key is wrong", and
// the two have completely different remedies: one is the customer's problem and the other is
// entirely ours. Saying which, at startup, is the difference between a five-minute answer and
// a day of support.
//
// It is louder when a key was actually supplied, because then it is not a warning about a
// future release, it is a paying customer being refused right now.
func (s *LicenseService) warnIfNoTrustedKey() {
	if s.logger == nil || s.hasTrustedKey == nil || s.hasTrustedKey() {
		return
	}

	s.mu.RLock()
	keySource := s.keySource
	s.mu.RUnlock()

	const msg = "This build carries a PLACEHOLDER licence signing key and can never accept a licence key. Every licence will be refused and this deployment will run on the free tier until a release is built with a real key compiled into pkg/license/pubkey_prod.go."
	entry := s.logger.WithFields(map[string]interface{}{
		"placeholder_signing_key": true,
		"key_source":              keySource,
	})
	if keySource == licenseSourceNone {
		entry.Warn(msg)
		return
	}
	entry.Error(msg)
}

// warn emits the single degradation line. Nil-tolerant because a service that cannot log is
// still a service that must answer Entitlements.
func (s *LicenseService) warn(msg string, fields map[string]interface{}) {
	if s.logger == nil {
		return
	}
	s.logger.WithFields(fields).Warn(msg)
}

// entitlementsFrom reconciles verified claims against the clock. Pure, so the boundaries it
// draws can be tested to the second without a signing key or a settings table.
func entitlementsFrom(claims *license.Claims, now time.Time) domain.Entitlements {
	if claims == nil {
		return domain.CommunityEntitlements()
	}

	expiresAt := time.Unix(claims.Exp, 0).UTC()
	state := deriveLicenseState(expiresAt, now)

	// Expired is the same tier as unlicensed — that is the whole point of there being no
	// intermediate frozen state — so the grant is Community. The licensee's name, tier and
	// expiry are kept anyway: the console has to be able to tell a customer whose key ran
	// out apart from an installation that never had one, and those three fields are the
	// only way it can.
	if state == domain.LicenseStateExpired {
		ent := domain.CommunityEntitlements()
		ent.State = domain.LicenseStateExpired
		ent.Tier = claims.Tier
		ent.Org = claims.Org
		ent.Sub = claims.Sub
		ent.ExpiresAt = expiresAt
		return ent
	}

	return domain.Entitlements{
		Tier:          claims.Tier,
		Org:           claims.Org,
		Sub:           claims.Sub,
		MaxWorkspaces: normalizeWorkspaceQuota(claims.MaxWS),
		Features:      knownFeatures(claims.Feat),
		State:         state,
		ExpiresAt:     expiresAt,
	}
}

// deriveLicenseState places now on the timeline the key draws.
//
//	now <= exp                  active
//	exp < now <= exp+GracePeriod  grace — the key still grants everything it ever granted
//	now > exp+GracePeriod       expired — identical to holding no key at all
//
// Both boundaries are inclusive of the earlier state. A key checked in the same second it
// expires is active, not expired: a strict comparison would mean the outcome depends on
// which side of a tick the request landed, which is not a distinction anybody would want to
// debug from a support ticket.
func deriveLicenseState(expiresAt, now time.Time) domain.LicenseState {
	switch {
	case !now.After(expiresAt):
		return domain.LicenseStateActive
	case !now.After(expiresAt.Add(GracePeriod)):
		return domain.LicenseStateGrace
	default:
		return domain.LicenseStateExpired
	}
}

// knownFeatures filters a key's feature list down to the ones this build knows how to gate.
//
// The list is an allow-list, and an unrecognised string is dropped rather than honoured: a
// key minted today must not unlock a capability invented three releases from now, and a
// build that granted anything it did not recognise would do exactly that. The result is
// always non-nil so it marshals to [] for the console rather than to null.
func knownFeatures(raw []string) []domain.Feature {
	features := make([]domain.Feature, 0, len(raw))
	for _, name := range raw {
		f := domain.Feature(name)
		if f.IsValid() {
			features = append(features, f)
		}
	}
	return features
}

// normalizeWorkspaceQuota maps a minted ceiling onto something a gate can safely compare
// against.
//
// Any negative value becomes UnlimitedWorkspaces. Only -1 is ever minted, but a gate that
// received -5 would compare against it and behave in a way nobody designed, so the intent
// ("no ceiling") is normalised here where it can be read, rather than at four call sites
// where it cannot.
//
// And anything below domain.CommunityMaxWorkspaces is raised to it. This is the invariant
// the whole scheme rests on: a licensed deployment must never be allowed fewer workspaces
// than an unlicensed one. A key carrying max_ws 0 — mintable until licensegen learned to
// refuse it, and still mintable by any other signer holding the private key — otherwise
// resolves to a literal ceiling of zero, and the G1 gate's `count >= limit` is then true on
// an installation with no workspaces at all: the customer who just paid cannot create their
// first workspace, while the customer who paid nothing creates three. Keys are immutable and
// there is no revocation, so a key like that cannot be recalled; the floor is what makes it
// harmless.
func normalizeWorkspaceQuota(maxWS int) int {
	if maxWS < 0 {
		return domain.UnlimitedWorkspaces
	}
	if maxWS < domain.CommunityMaxWorkspaces {
		return domain.CommunityMaxWorkspaces
	}
	return maxWS
}

// formatLicenseExpiry renders an expiry for the startup log, where the zero time means there
// is no key rather than the year 1.
func formatLicenseExpiry(t time.Time) string {
	if t.IsZero() {
		return "n/a"
	}
	return t.UTC().Format(time.RFC3339)
}

// formatWorkspaceQuota renders a workspace ceiling for the startup log, mirroring
// formatPlanLimit's vocabulary for the neighbouring "Plan limits resolved" line.
func formatWorkspaceQuota(limit int) string {
	if limit == domain.UnlimitedWorkspaces {
		return "unlimited"
	}
	return fmt.Sprintf("%d", limit)
}
