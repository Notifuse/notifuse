// Package domain — bounce classification.
//
// ClassifyBounce maps a provider-specific (bounceType, bounceSubType, diagnostic)
// tuple to one of three outcomes used by the inbound webhook pipeline:
//
//   - BounceClassificationHard       — escalate immediately (mark contact_lists
//     as bounced and flip message_history to bounced).
//   - BounceClassificationSoftCount  — temporary failure attributable to the
//     recipient (mailbox full, transient, undetermined, …). Counts toward the
//     workspace-configured soft-bounce threshold; the caller decides whether
//     repeated occurrences should escalate to Hard.
//   - BounceClassificationSoftIgnore — temporary failure attributable to the
//     message itself (too large, content rejected, attachment rejected, …).
//     Never counts, never escalates.
//
// Per-provider mapping summary:
//
//   - SES        — `Permanent/*` → Hard. `Transient/MessageTooLarge|ContentRejected|
//     AttachmentRejected` → SoftIgnore. `Transient/General` → Hard if diagnostic
//     matches retry exhaustion (SMTP `4.4.7` or "message expired"); otherwise
//     SoftCount. Other `Transient/*` and `Undetermined` → SoftCount.
//   - Mailgun    — `hardbounce`/`permanent` → Hard. `softbounce`/`temporary` →
//     SoftCount.
//   - Mailjet    — `hardbounce` → Hard. `softbounce` → SoftCount.
//   - Postmark   — `HardBounce`/`Blocked`, or any value containing "hard" → Hard.
//     `MessageContent`/`RuleBlocked` (message-level rejections) → SoftIgnore.
//     `SoftBounce` and other "soft" values → SoftCount.
//   - SparkPost  — bounce class codes 10/30/90 → Hard. 1/20-25/40/50-54/60/70/
//     80/100 → SoftCount.
//   - SendGrid   — `bounce` or category "invalid address" → Hard.
//     `blocked`/`dropped` and the documented temporary categories → SoftCount.
//   - SMTP       — `hardbounce` → Hard, otherwise SoftCount.
//
// Unknown/empty inputs default to SoftCount: the threshold check still gives
// repeated failures a chance to escalate, but we do not unilaterally mark a
// list as bounced from a single ambiguous event.
package domain

import (
	"regexp"
	"strings"
)

// BounceClassification is the outcome of classifying an inbound bounce event.
type BounceClassification int

const (
	// BounceClassificationHard means the bounce is permanent or otherwise
	// should immediately mark the recipient as bounced.
	BounceClassificationHard BounceClassification = iota
	// BounceClassificationSoftCount means the bounce is temporary but
	// attributable to the recipient and counts toward the soft-bounce
	// threshold.
	BounceClassificationSoftCount
	// BounceClassificationSoftIgnore means the bounce describes a problem
	// with the message (too large, content rejected, …) and must not count
	// against the recipient.
	BounceClassificationSoftIgnore
)

// DefaultSoftBounceThreshold is the fallback consecutive-soft-bounce count
// after which a contact is escalated to Hard. Workspace settings may override
// this value in the range [1, 20].
const DefaultSoftBounceThreshold = 5

// BounceInput carries the raw, provider-specific signal that ClassifyBounce
// inspects.
type BounceInput struct {
	Provider   EmailProviderKind
	Type       string
	Subtype    string
	Diagnostic string
}

// retryExhaustedRe matches the SMTP enhanced status code SES emits after it
// has retried for 840 minutes and given up.
var retryExhaustedRe = regexp.MustCompile(`\b4\.4\.7\b`)

// isRetryExhaustedDiagnostic reports whether the SMTP diagnostic indicates
// SES retry exhaustion (effectively undeliverable).
func isRetryExhaustedDiagnostic(diagnostic string) bool {
	if diagnostic == "" {
		return false
	}
	lower := strings.ToLower(diagnostic)
	if strings.Contains(lower, "message expired") {
		return true
	}
	return retryExhaustedRe.MatchString(lower)
}

// ClassifyBounce decides how a bounce event should affect downstream state.
// The function is pure and safe for concurrent use.
func ClassifyBounce(in BounceInput) BounceClassification {
	bounceType := strings.ToLower(strings.TrimSpace(in.Type))
	subtype := strings.ToLower(strings.TrimSpace(in.Subtype))

	switch in.Provider {
	case EmailProviderKindSES:
		switch bounceType {
		case "permanent":
			return BounceClassificationHard
		case "transient":
			switch subtype {
			case "messagetoolarge", "contentrejected", "attachmentrejected":
				return BounceClassificationSoftIgnore
			case "general":
				if isRetryExhaustedDiagnostic(in.Diagnostic) {
					return BounceClassificationHard
				}
				return BounceClassificationSoftCount
			default:
				return BounceClassificationSoftCount
			}
		case "undetermined":
			return BounceClassificationSoftCount
		}

	case EmailProviderKindMailgun:
		if bounceType == "hardbounce" || bounceType == "permanent" ||
			subtype == "hardbounce" || subtype == "permanent" {
			return BounceClassificationHard
		}
		if bounceType == "softbounce" || bounceType == "temporary" ||
			subtype == "softbounce" || subtype == "temporary" {
			return BounceClassificationSoftCount
		}

	case EmailProviderKindMailjet:
		if bounceType == "hardbounce" {
			return BounceClassificationHard
		}
		if bounceType == "softbounce" {
			return BounceClassificationSoftCount
		}

	case EmailProviderKindPostmark:
		// Message-level rejections — the recipient is fine, the payload isn't.
		switch bounceType {
		case "messagecontent", "ruleblocked":
			return BounceClassificationSoftIgnore
		}
		if bounceType == "blocked" || subtype == "blocked" {
			return BounceClassificationHard
		}
		if strings.Contains(bounceType, "hard") || strings.Contains(subtype, "hard") {
			return BounceClassificationHard
		}
		if strings.Contains(bounceType, "soft") || strings.Contains(subtype, "soft") {
			return BounceClassificationSoftCount
		}

	case EmailProviderKindSparkPost:
		switch subtype {
		case "10", "30", "90":
			return BounceClassificationHard
		case "1", "20", "21", "22", "23", "24", "25",
			"40", "50", "51", "52", "53", "54",
			"60", "70", "80", "100":
			return BounceClassificationSoftCount
		}

	case EmailProviderKindSendGrid:
		switch bounceType {
		case "bounce":
			return BounceClassificationHard
		case "blocked", "dropped":
			return BounceClassificationSoftCount
		}
		switch subtype {
		case "invalid address":
			return BounceClassificationHard
		case "technical", "content", "reputation",
			"frequency/volume", "mailbox unavailable", "unclassified":
			return BounceClassificationSoftCount
		}

	case EmailProviderKindSMTP:
		if bounceType == "hardbounce" {
			return BounceClassificationHard
		}
		if bounceType == "softbounce" {
			return BounceClassificationSoftCount
		}
	}

	// Safe default — repeated occurrences can still escalate via the
	// threshold path, but a single ambiguous bounce never marks a list.
	return BounceClassificationSoftCount
}
