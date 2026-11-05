package emailerror

import (
	"regexp"
	"strconv"
	"strings"
)

// SMTP error classification
//
// RECIPIENT ERRORS (5xx permanent failures - should NOT trigger circuit breaker):
// - 550: Mailbox unavailable (recipient doesn't exist)
// - 551: User not local (routing issue)
// - 552: Storage exceeded (mailbox full)
// - 553: Mailbox name not allowed (invalid format)
//
// PROVIDER ERRORS (4xx temporary failures - SHOULD trigger circuit breaker):
// - 421: Service temporarily unavailable
// - 450: Mailbox busy
// - 451: Local error in processing
// - 452: Insufficient storage
// - Connection timeouts, TLS failures

// SMTP recipient error patterns (5xx permanent failures)
var smtpRecipientPatterns = []string{
	"550 ",
	"550:",
	"551 ",
	"551:",
	"552 ",
	"552:",
	"553 ",
	"553:",
	"5.1.1", // Mailbox does not exist
	"5.1.2", // Bad destination mailbox
	"5.1.3", // Bad destination mailbox syntax
	"5.2.1", // Mailbox disabled
	"5.2.2", // Mailbox full
	"5.7.1", // Delivery not authorized (often recipient policy)
	"mailbox unavailable",
	"mailbox not found",
	"user unknown",
	"no such user",
	"recipient rejected",
	"does not exist",
	"mailbox full",
	"over quota",
}

// SMTP provider error patterns (4xx temporary failures, connection issues)
var smtpProviderPatterns = []string{
	"421 ",
	"421:",
	"450 ",
	"450:",
	"451 ",
	"451:",
	"452 ",
	"452:",
	"4.7.1", // Delivery not authorized (server policy)
	"connection refused",
	"connection reset",
	"connection timeout",
	"timed out",
	"timeout",
	"tls handshake",
	"tls error",
	"ssl error",
	"authentication failed",
	"auth failed",
	"login failed",
	"service unavailable",
	"try again later",
	"temporary failure",
	"greylisted",
	"greylist",
}

// smtpCodeRegex finds a three-digit reply code either at the start of the message —
// the shape a server's own response line has — or just after the word "code", which
// is how the built-in SMTP client reports one ("message rejected with code 452: ...",
// "RCPT TO rejected for x@y with code 550: ...").
var smtpCodeRegex = regexp.MustCompile(`(?i)(?:^|\bcode[:\s]\s*)([45]\d{2})\b`)

// extractSMTPCode returns the reply code carried by an error message, if any.
func extractSMTPCode(errStr string) (int, bool) {
	m := smtpCodeRegex.FindStringSubmatch(errStr)
	if len(m) < 2 {
		return 0, false
	}
	code, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return code, true
}

// smtpThrottlePhrases are what servers say when the sender goes too fast: SES's SMTP
// endpoint ("Throttling failure: Maximum sending rate exceeded"), Postfix ("too much mail
// from"), Microsoft 365 ("Message submission rate for this client has exceeded"), and
// the rate limit policy services of self-hosted relays.
var smtpThrottlePhrases = []string{
	"rate limit",
	"ratelimit",
	"throttl",
	"too many messages",
	"too many emails",
	"too much mail",
	"sending rate",
	"submission rate",
	"rate exceeded",
}

// smtpMailboxPhrases mark a refusal at RCPT TO that is about the recipient's mailbox,
// such as Gmail's "4.2.1 The user you are trying to contact is receiving mail at a rate
// that prevents additional messages from being delivered". Sending more slowly to
// everyone else would not help that one recipient.
var smtpMailboxPhrases = []string{"4.2.", "mailbox", "receiving"}

// smtpReplyText returns the server's own text after the reply code, without the rest of
// the message, which can hold the recipient address.
func smtpReplyText(errStr string) (string, bool) {
	loc := smtpCodeRegex.FindStringSubmatchIndex(errStr)
	if loc == nil {
		return "", false
	}
	text := strings.TrimLeft(errStr[loc[1]:], ": ")
	return text, true
}

// isSMTPThrottle reports whether a server's reply text asks the sender to slow down.
func isSMTPThrottle(replyText string) bool {
	return containsAny(replyText, smtpThrottlePhrases) && !isQuotaSignal(replyText)
}

// smtpCodeIsRecipient reports whether a reply code blames the recipient rather than
// the connection: the mailbox is unavailable, not local, full, misnamed, or the
// transaction was refused for that address. Every other 5xx — a syntax error, a 53x
// authentication failure — is a fault of the session or the credentials and stays
// provider-class, so it keeps counting toward the circuit breaker and keeps its
// retries. Being wrong in that direction costs three cheap attempts; being wrong the
// other way would mark every message permanently failed the moment a password broke.
func smtpCodeIsRecipient(code int) bool {
	return code >= 550 && code <= 554
}

func (c *Classifier) classifySMTPError(err error, errStr string, httpStatus int) *ClassifiedError {
	result := &ClassifiedError{
		Original:   err,
		Provider:   "smtp",
		HTTPStatus: httpStatus,
		Retryable:  true,
	}

	// Decide on the reply code whenever the message carries one, before consulting
	// the phrase lists. Those lists only ever matched a code followed by a space or a
	// colon, and the client's own wording puts it elsewhere — so every generic-SMTP
	// failure fell through to unknown, which IsProviderError treats as a provider
	// fault. A dead mailbox was retried three times and pushed the workspace's
	// circuit breaker toward opening.
	if code, ok := extractSMTPCode(errStr); ok {
		if smtpCodeIsRecipient(code) {
			result.Type = ErrorTypeRecipient
			result.Retryable = false
			return result
		}
		result.Type = ErrorTypeProvider
		result.Retryable = true
		if code < 500 {
			if text, ok := smtpReplyText(errStr); ok && isSMTPThrottle(text) {
				atRecipient := strings.Contains(errStr, "RCPT TO rejected")
				result.RateLimited = !atRecipient || !containsAny(text, smtpMailboxPhrases)
			}
		}
		return result
	}

	// Check for recipient-specific errors (5xx permanent failures)
	if containsAny(errStr, smtpRecipientPatterns) {
		result.Type = ErrorTypeRecipient
		result.Retryable = false
		return result
	}

	// Check for provider errors (4xx temporary failures, connection issues)
	if containsAny(errStr, smtpProviderPatterns) {
		result.Type = ErrorTypeProvider
		// Most SMTP temporary errors are retryable
		result.Retryable = true
		return result
	}

	// Fallback to HTTP status classification (if applicable)
	if httpStatus > 0 {
		result.Type = classifyByHTTPStatus(httpStatus)
		result.Retryable = httpStatus >= 500 || httpStatus == 429
		return result
	}

	// Unknown error - treat as provider error for safety
	result.Type = ErrorTypeUnknown
	result.Retryable = true
	return result
}
