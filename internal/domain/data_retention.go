package domain

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RetentionCategory is one kind of personal data a retention policy can expire. The set is
// closed: a policy naming anything else is refused.
type RetentionCategory string

const (
	// RetentionInactiveContacts deletes contacts who have not engaged for the period (the D5
	// clock), with everything they own; their opt-outs are kept as hashes.
	RetentionInactiveContacts RetentionCategory = "inactive_contacts"
	// RetentionMessageHistory redacts sent messages older than the period (never deletes
	// them: broadcast statistics are computed from the rows).
	RetentionMessageHistory RetentionCategory = "message_history"
	// RetentionContactTimeline deletes timeline entries older than the period.
	RetentionContactTimeline RetentionCategory = "contact_timeline"
	// RetentionCustomEvents deletes custom events that occurred before the period.
	RetentionCustomEvents RetentionCategory = "custom_events"
	// RetentionWebAnalytics drops the web analytics months that ended before the period.
	RetentionWebAnalytics RetentionCategory = "web_analytics"
	// RetentionInboundWebhookEvents deletes provider events (deliveries, bounces, replies).
	RetentionInboundWebhookEvents RetentionCategory = "inbound_webhook_events"
	// RetentionWebhookDeliveries deletes outgoing webhook deliveries.
	RetentionWebhookDeliveries RetentionCategory = "webhook_deliveries"
	// RetentionFailedEmails deletes the email queue's failed rows.
	RetentionFailedEmails RetentionCategory = "failed_emails"
	// RetentionAutomationHistory deletes finished automation journeys (their node executions
	// go with them; the trigger log stays, so a "once" automation never enrols a contact
	// twice).
	RetentionAutomationHistory RetentionCategory = "automation_history"
)

// RetentionCategorySpec describes a category: the shortest period the product can honour,
// and the period the console suggests when the category is switched on.
type RetentionCategorySpec struct {
	Category      RetentionCategory `json:"category"`
	MinDays       int               `json:"min_days"`
	SuggestedDays int               `json:"suggested_days"`
	// DefaultDays is the period a deployment applies when root sets nothing: 0 (off) for
	// every category of personal data, which are opt-in.
	DefaultDays int `json:"default_days"`
}

// RetentionMaxDays is the longest period a category accepts (10 years).
const RetentionMaxDays = 3650

// RetentionPendingDelay is how long a change that deletes more waits before it applies:
// time to read the preview again and cancel.
const RetentionPendingDelay = 7 * 24 * time.Hour

// RetentionCategorySpecs lists the categories in the order a run processes them: contacts
// first, so the history of the contacts deleted goes in the same run.
var RetentionCategorySpecs = []RetentionCategorySpec{
	{RetentionInactiveContacts, 180, 1095, 0},
	{RetentionMessageHistory, 90, 1095, 0},
	{RetentionContactTimeline, 90, 1095, 0},
	{RetentionCustomEvents, 30, 1095, 0},
	{RetentionAutomationHistory, 90, 1095, 0},
	{RetentionWebAnalytics, 30, 761, 0},
	{RetentionInboundWebhookEvents, 30, 365, 0},
	// The two technical logs are purged after 7 days today; that stays the default.
	{RetentionWebhookDeliveries, 7, 7, 7},
	{RetentionFailedEmails, 7, 7, 7},
}

// RetentionCategorySpecOf returns a category's spec.
func RetentionCategorySpecOf(category RetentionCategory) (RetentionCategorySpec, bool) {
	for _, spec := range RetentionCategorySpecs {
		if spec.Category == category {
			return spec, true
		}
	}
	return RetentionCategorySpec{}, false
}

// ValidateRetentionDays checks one category's period: 0 (off), or from its minimum to 10
// years.
func ValidateRetentionDays(category RetentionCategory, days int) error {
	spec, ok := RetentionCategorySpecOf(category)
	if !ok {
		return fmt.Errorf("unknown retention category %q", category)
	}
	if days == 0 {
		return nil
	}
	if days < spec.MinDays || days > RetentionMaxDays {
		return fmt.Errorf("%s: the period must be 0 (off) or between %d and %d days", category, spec.MinDays, RetentionMaxDays)
	}
	return nil
}

// DataRetentionPolicy is a workspace's retention policy. Categories maps a category to its
// period in days; a category absent from the map follows the deployment default, and 0
// switches it off. The other fields only concern inactive contacts.
type DataRetentionPolicy struct {
	Categories map[RetentionCategory]int `json:"categories"`
	// ExemptListIDs and ExemptSegmentIDs: their members are never deleted as inactive.
	ExemptListIDs    []string `json:"exempt_list_ids,omitempty"`
	ExemptSegmentIDs []string `json:"exempt_segment_ids,omitempty"`
	// EngagementEventNames are the custom event names that are actions of the contact
	// (a purchase, a login): their events move the contact's clock forward.
	EngagementEventNames []string `json:"engagement_event_names,omitempty"`
}

// Validate refuses an unknown category, a period out of bounds and an empty id or name.
func (p *DataRetentionPolicy) Validate() error {
	if p == nil {
		return nil
	}
	for category, days := range p.Categories {
		if err := ValidateRetentionDays(category, days); err != nil {
			return err
		}
	}
	for _, field := range []struct {
		name   string
		values []string
	}{
		{"exempt_list_ids", p.ExemptListIDs},
		{"exempt_segment_ids", p.ExemptSegmentIDs},
		{"engagement_event_names", p.EngagementEventNames},
	} {
		for _, v := range field.values {
			if strings.TrimSpace(v) == "" {
				return fmt.Errorf("%s: an entry is empty", field.name)
			}
		}
	}
	return nil
}

// Clone returns a deep copy.
func (p *DataRetentionPolicy) Clone() *DataRetentionPolicy {
	if p == nil {
		return nil
	}
	c := &DataRetentionPolicy{Categories: make(map[RetentionCategory]int, len(p.Categories))}
	for k, v := range p.Categories {
		c.Categories[k] = v
	}
	c.ExemptListIDs = append([]string(nil), p.ExemptListIDs...)
	c.ExemptSegmentIDs = append([]string(nil), p.ExemptSegmentIDs...)
	c.EngagementEventNames = append([]string(nil), p.EngagementEventNames...)
	return c
}

// Normalize sorts and deduplicates the lists, so two policies that mean the same compare
// equal.
func (p *DataRetentionPolicy) Normalize() {
	if p == nil {
		return
	}
	if p.Categories == nil {
		p.Categories = map[RetentionCategory]int{}
	}
	p.ExemptListIDs = sortedSet(p.ExemptListIDs)
	p.ExemptSegmentIDs = sortedSet(p.ExemptSegmentIDs)
	p.EngagementEventNames = sortedSet(p.EngagementEventNames)
}

func sortedSet(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

// RetentionDefaults are a deployment's default periods (root, D6), with their own pending
// change: a default that deletes more waits 7 days, like a workspace's own change.
type RetentionDefaults struct {
	Categories map[RetentionCategory]int `json:"categories"`
	Pending    *PendingRetentionDefaults `json:"pending,omitempty"`
}

// PendingRetentionDefaults is a change of the deployment defaults that waits.
type PendingRetentionDefaults struct {
	Categories  map[RetentionCategory]int `json:"categories"`
	EffectiveAt time.Time                 `json:"effective_at"`
}

// BuiltinRetentionDefaults are the defaults of a deployment where root set nothing.
func BuiltinRetentionDefaults() map[RetentionCategory]int {
	out := make(map[RetentionCategory]int, len(RetentionCategorySpecs))
	for _, spec := range RetentionCategorySpecs {
		out[spec.Category] = spec.DefaultDays
	}
	return out
}

// ResolvedDefaults returns the deployment's default periods for every category at now: the
// built-in defaults, overridden by root's, with root's pending change applied once due.
func (d *RetentionDefaults) ResolvedDefaults(now time.Time) map[RetentionCategory]int {
	out := BuiltinRetentionDefaults()
	if d == nil {
		return out
	}
	for k, v := range d.Categories {
		out[k] = v
	}
	if d.Pending != nil && !now.Before(d.Pending.EffectiveAt) {
		for k, v := range d.Pending.Categories {
			out[k] = v
		}
	}
	return out
}

// PendingDefaults returns root's pending default periods (every category), and when they
// apply; nil when there is no pending change or it is already due.
func (d *RetentionDefaults) PendingDefaults(now time.Time) (map[RetentionCategory]int, *time.Time) {
	if d == nil || d.Pending == nil || !now.Before(d.Pending.EffectiveAt) {
		return nil, nil
	}
	out := BuiltinRetentionDefaults()
	for k, v := range d.Categories {
		out[k] = v
	}
	for k, v := range d.Pending.Categories {
		out[k] = v
	}
	at := d.Pending.EffectiveAt
	return out, &at
}

// DataRetentionSettings is what a workspace stores: the policy in force, and the change
// that waits, if any. Written only through dataRetention.set and dataRetention.cancelPending,
// and preserved across a partial workspaces.update.
type DataRetentionSettings struct {
	Policy  *DataRetentionPolicy    `json:"policy,omitempty"`
	Pending *PendingRetentionChange `json:"pending,omitempty"`
	// Revision counts the writes (WorkspaceRepository.SetDataRetentionSettings), so a write
	// made from a stale read is refused rather than lost.
	Revision int `json:"revision,omitempty"`
}

// PendingRetentionChange is a saved policy that deletes more than the one in force: it
// replaces it at EffectiveAt, unless cancelled.
type PendingRetentionChange struct {
	Policy      *DataRetentionPolicy `json:"policy"`
	EffectiveAt time.Time            `json:"effective_at"`
	RequestedBy string               `json:"requested_by,omitempty"`
	RequestedAt time.Time            `json:"requested_at"`
}

// Due applies a pending change whose date has come, and reports whether it did.
func (s *DataRetentionSettings) Due(now time.Time) bool {
	if s == nil || s.Pending == nil || now.Before(s.Pending.EffectiveAt) {
		return false
	}
	s.Policy = s.Pending.Policy
	s.Pending = nil
	return true
}

// EffectiveRetention is a workspace's policy resolved for one run: every category's period,
// and the inactive-contact settings.
type EffectiveRetention struct {
	Days                 map[RetentionCategory]int
	ExemptListIDs        []string
	ExemptSegmentIDs     []string
	EngagementEventNames []string
}

// Resolve combines a workspace's policy with the deployment defaults: a category the policy
// names takes its value, any other the default's.
func Resolve(policy *DataRetentionPolicy, defaults map[RetentionCategory]int) EffectiveRetention {
	eff := EffectiveRetention{Days: make(map[RetentionCategory]int, len(RetentionCategorySpecs))}
	for _, spec := range RetentionCategorySpecs {
		eff.Days[spec.Category] = defaults[spec.Category]
	}
	if policy == nil {
		return eff
	}
	for k, v := range policy.Categories {
		eff.Days[k] = v
	}
	eff.ExemptListIDs = append([]string(nil), policy.ExemptListIDs...)
	eff.ExemptSegmentIDs = append([]string(nil), policy.ExemptSegmentIDs...)
	eff.EngagementEventNames = append([]string(nil), policy.EngagementEventNames...)
	return eff
}

// deletesMore reports whether a period deletes more than another: switching a category on,
// or shortening it.
func deletesMore(next, current int) bool {
	if next == 0 {
		return false
	}
	return current == 0 || next < current
}

// missingFrom reports whether next lacks a value current has.
func missingFrom(next, current []string) bool {
	have := make(map[string]struct{}, len(next))
	for _, v := range next {
		have[v] = struct{}{}
	}
	for _, v := range current {
		if _, ok := have[v]; !ok {
			return true
		}
	}
	return false
}

// DeletesMore reports whether resolving next instead of current would delete anything
// current keeps: a category switched on or shortened, an exemption or an engagement event
// name removed. Such a change waits RetentionPendingDelay (D7).
func (next EffectiveRetention) DeletesMore(current EffectiveRetention) bool {
	for _, spec := range RetentionCategorySpecs {
		if deletesMore(next.Days[spec.Category], current.Days[spec.Category]) {
			return true
		}
	}
	return missingFrom(next.ExemptListIDs, current.ExemptListIDs) ||
		missingFrom(next.ExemptSegmentIDs, current.ExemptSegmentIDs) ||
		missingFrom(next.EngagementEventNames, current.EngagementEventNames)
}

// Safest combines two resolved policies into the one that deletes least of both: the longer
// period (off beats on), every exemption and every engagement event name. While a change
// waits, a run applies this combination of the policy in force and the one requested, so a
// part that deletes less takes effect at once and nothing deletes more before its date.
func Safest(a, b EffectiveRetention) EffectiveRetention {
	out := EffectiveRetention{Days: make(map[RetentionCategory]int, len(RetentionCategorySpecs))}
	for _, spec := range RetentionCategorySpecs {
		x, y := a.Days[spec.Category], b.Days[spec.Category]
		switch {
		case x == 0 || y == 0:
			out.Days[spec.Category] = 0
		case x > y:
			out.Days[spec.Category] = x
		default:
			out.Days[spec.Category] = y
		}
	}
	out.ExemptListIDs = sortedSet(append(append([]string(nil), a.ExemptListIDs...), b.ExemptListIDs...))
	out.ExemptSegmentIDs = sortedSet(append(append([]string(nil), a.ExemptSegmentIDs...), b.ExemptSegmentIDs...))
	out.EngagementEventNames = sortedSet(append(append([]string(nil), a.EngagementEventNames...), b.EngagementEventNames...))
	return out
}

// RetentionRunStatus is the outcome of a run.
type RetentionRunStatus string

const (
	RetentionRunRunning   RetentionRunStatus = "running"
	RetentionRunCompleted RetentionRunStatus = "completed"
	// RetentionRunPartial: at least one category failed; the others ran.
	RetentionRunPartial RetentionRunStatus = "partial"
	RetentionRunFailed  RetentionRunStatus = "failed"
)

// RetentionCategoryResult is what a run did for one category.
type RetentionCategoryResult struct {
	Days int `json:"days"`
	// Rows deleted, redacted or (web analytics) partitions dropped.
	Affected int64 `json:"affected"`
	// OptOutsKept: for inactive contacts, the opt-outs kept as hashes.
	OptOutsKept int64 `json:"opt_outs_kept,omitempty"`
	// Remaining: the run stopped at its budget and the rest waits for the next run.
	Remaining bool   `json:"remaining,omitempty"`
	Skipped   string `json:"skipped,omitempty"`
	Error     string `json:"error,omitempty"`
}

// RetentionRun is the report of one run on one workspace. It holds counts, never an
// address.
type RetentionRun struct {
	ID         string                                        `json:"id"`
	StartedAt  time.Time                                     `json:"started_at"`
	FinishedAt *time.Time                                    `json:"finished_at,omitempty"`
	Status     RetentionRunStatus                            `json:"status"`
	Policy     map[RetentionCategory]int                     `json:"policy"`
	Results    map[RetentionCategory]RetentionCategoryResult `json:"results"`
	Error      string                                        `json:"error,omitempty"`
}

// RetentionInForce resolves what a run applies at now, and what waits.
//
// In force is the combination that deletes least (Safest) of the policy in force and every
// change still waiting, the workspace's own and the deployment defaults': a part of a change
// that deletes less applies at once, a part that deletes more waits for its date. A change
// whose date has come is in force. Pending is what the workspace will have once every waiting
// change applies, with the latest of their dates; nil when nothing waits.
func RetentionInForce(settings *DataRetentionSettings, defaults *RetentionDefaults, now time.Time) (inForce EffectiveRetention, pending *EffectiveRetention, pendingAt *time.Time) {
	var policy *DataRetentionPolicy
	var waiting *PendingRetentionChange
	if settings != nil {
		policy = settings.Policy
		waiting = settings.Pending
		if waiting != nil && !now.Before(waiting.EffectiveAt) {
			policy, waiting = waiting.Policy, nil
		}
	}
	current := defaults.ResolvedDefaults(now)
	inForce = Resolve(policy, current)

	target := policy
	var at *time.Time
	if waiting != nil {
		target = waiting.Policy
		t := waiting.EffectiveAt
		at = &t
	}
	targetDefaults := current
	if next, nextAt := defaults.PendingDefaults(now); next != nil {
		targetDefaults = next
		if at == nil || nextAt.After(*at) {
			at = nextAt
		}
	}
	if at == nil {
		return inForce, nil, nil
	}
	final := Resolve(target, targetDefaults)
	// Every combination of what is in force and what waits, so nothing deletes more than
	// the policy in force before its own date.
	inForce = Safest(Safest(inForce, final), Safest(Resolve(target, current), Resolve(policy, targetDefaults)))
	return inForce, &final, at
}

// ExpiredContactsQuery selects the contacts the inactive-contact category deletes.
type ExpiredContactsQuery struct {
	// EngagedBefore: a contact whose clock is before it has expired.
	EngagedBefore        time.Time
	ExemptListIDs        []string
	ExemptSegmentIDs     []string
	EngagementEventNames []string
	// AfterEmail and Limit page through the contacts in email order.
	AfterEmail string
	Limit      int
}

// RetentionContactFloor: a contact is never deleted as inactive less than this long after
// it was stored, whatever its collection date says. An import carrying historical created_at
// values (a migration from another tool) is not purged on arrival.
const RetentionContactFloor = 7 * 24 * time.Hour

//go:generate mockgen -destination mocks/mock_data_retention_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain DataRetentionRepository

// DataRetentionRepository holds the purge statements of the retention engine, workspace by
// workspace. Every statement that deletes runs with notifuse.retention_purge on, in a
// transaction of a bounded number of rows.
type DataRetentionRepository interface {
	// ListExpiredContacts returns the addresses of contacts whose D5 clock is before the
	// query's date, outside the exemptions and past the floor, in email order.
	ListExpiredContacts(ctx context.Context, workspaceID string, q ExpiredContactsQuery) ([]string, error)
	// CountExpiredContacts counts them, and the opt-outs that would be kept as hashes.
	CountExpiredContacts(ctx context.Context, workspaceID string, q ExpiredContactsQuery) (contacts int64, optOuts int64, err error)
	// DeleteExpiredContacts deletes, in one transaction, those of the given contacts that are
	// still expired, with their list rows (their opt-outs kept as hashes), segment
	// memberships and queued mail, and puts each one on the erasure queue for the rest of
	// its history. It returns the addresses deleted and the opt-outs kept.
	DeleteExpiredContacts(ctx context.Context, workspaceID, runID string, q ExpiredContactsQuery, emails []string) (deleted []string, optOutsKept int64, err error)
	// ErasureQueue returns up to limit addresses waiting for their history to be erased.
	ErasureQueue(ctx context.Context, workspaceID string, limit int) ([]string, error)
	// Dequeue removes an address from the erasure queue.
	Dequeue(ctx context.Context, workspaceID, email string) error
	// Requeue moves an address whose erasure failed to the back of the queue, so it does not
	// hold up the others; the next run tries it again.
	Requeue(ctx context.Context, workspaceID, email string) error
	// ContactExists reports whether a contact with that address exists (again).
	ContactExists(ctx context.Context, workspaceID, email string) (bool, error)

	// PurgeCategory deletes or redacts up to limit rows of a category older than before
	// (inactive contacts and web analytics have their own methods). It returns the rows
	// affected; fewer than limit means the category is done for this run. For custom events,
	// the latest designated event of each contact is folded into its engagement date first
	// (D10), so deleting it does not shorten the contact's clock.
	PurgeCategory(ctx context.Context, workspaceID string, category RetentionCategory, before time.Time, limit int, engagementEventNames []string) (int64, error)
	// CountCategory counts what PurgeCategory would affect.
	CountCategory(ctx context.Context, workspaceID string, category RetentionCategory, before time.Time) (int64, error)
	// WebAnalyticsMonthsBefore lists the months (first day, UTC) whose partitions all end
	// before the given month.
	WebAnalyticsMonthsBefore(ctx context.Context, workspaceID string, month time.Time) ([]time.Time, error)
	// DropWebAnalyticsMonth drops the session, page and goal partitions of one month. It
	// refuses the current and the next month.
	DropWebAnalyticsMonth(ctx context.Context, workspaceID string, month time.Time, now time.Time) error

	// CustomEventNames returns the distinct custom event names the workspace has received,
	// sorted, at most limit: what the policy can designate as engagement.
	CustomEventNames(ctx context.Context, workspaceID string, limit int) ([]string, error)

	// BackfillCompleted reports whether the engagement backfill has run (the contact
	// category waits for it).
	BackfillCompleted(ctx context.Context, workspaceID string) (bool, error)
	// MissingExemptions returns the exempt lists and segments that do not exist (or are
	// deleted), and the exempt segments whose membership is not ready to be trusted.
	MissingExemptions(ctx context.Context, workspaceID string, listIDs, segmentIDs []string) ([]string, error)

	// TryLock takes the workspace's retention lock on a dedicated connection, held until
	// release is called, so two servers never purge the same workspace at once. ok is false
	// when another holder has it.
	TryLock(ctx context.Context, workspaceID string) (release func(), ok bool, err error)

	InsertRun(ctx context.Context, workspaceID string, run *RetentionRun) error
	FinishRun(ctx context.Context, workspaceID string, run *RetentionRun) error
	ListRuns(ctx context.Context, workspaceID string, limit int) ([]*RetentionRun, error)
}

// ErrRetentionExemptionUnavailable: an exempt list or segment is gone, or a segment is being
// rebuilt, so its members cannot be told apart: the contact category stops for this run.
var ErrRetentionExemptionUnavailable = errors.New("an exempt list or segment is unavailable")

// ErrRetentionConflict: the data retention settings changed between the read and the write
// (409). The caller reads them again.
type ErrRetentionConflict struct{ Message string }

func (e *ErrRetentionConflict) Error() string { return e.Message }
