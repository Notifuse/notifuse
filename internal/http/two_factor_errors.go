package http

import (
	"errors"
	"math"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
)

// Error codes of two-factor authentication, in the "error" key of the body. The console
// switches on them; "message" is for people.
const (
	twoFactorRequiredCode           = "two_factor_required"
	reauthenticationRequiredCode    = "reauthentication_required"
	twoFactorInvalidCode            = "two_factor_invalid_code"
	twoFactorChallengeInvalidCode   = "two_factor_challenge_invalid"
	twoFactorSetupExpiredCode       = "two_factor_setup_expired"
	twoFactorLockedCode             = "two_factor_locked"
	twoFactorTOTPLockedCode         = "two_factor_totp_locked"
	twoFactorRequiredByInstanceCode = "two_factor_required_by_instance"
)

// writeTwoFactorError answers the errors of two-factor authentication and reports whether
// it did. None of them answers 401: the console discards its token on any 401, which would
// sign a person out at their first typo, or throw them off the invitation page.
func writeTwoFactorError(w http.ResponseWriter, err error) bool {
	var locked *domain.TwoFactorLockedError
	switch {
	case errors.Is(err, domain.ErrTwoFactorRequired):
		writeTwoFactorBody(w, http.StatusForbidden, twoFactorRequiredCode, err)
	case errors.Is(err, domain.ErrReauthenticationRequired):
		writeTwoFactorBody(w, http.StatusForbidden, reauthenticationRequiredCode, err)
	case errors.Is(err, domain.ErrTwoFactorInvalidCode), errors.Is(err, domain.ErrInvalidRecoveryCode):
		writeTwoFactorBody(w, http.StatusUnprocessableEntity, twoFactorInvalidCode, err)
	case errors.Is(err, domain.ErrTwoFactorChallengeInvalid):
		writeTwoFactorBody(w, http.StatusGone, twoFactorChallengeInvalidCode, err)
	case errors.Is(err, domain.ErrTwoFactorSetupExpired):
		writeTwoFactorBody(w, http.StatusGone, twoFactorSetupExpiredCode, err)
	case errors.Is(err, domain.ErrTwoFactorTOTPLocked):
		writeTwoFactorBody(w, http.StatusTooManyRequests, twoFactorTOTPLockedCode, err)
	case errors.As(err, &locked):
		retryAfter := int(math.Ceil(locked.RetryAfter.Seconds()))
		writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
			"error":       twoFactorLockedCode,
			"message":     err.Error(),
			"retry_after": retryAfter,
		})
	case errors.Is(err, domain.ErrTwoFactorRequiredByInstance):
		writeTwoFactorBody(w, http.StatusConflict, twoFactorRequiredByInstanceCode, err)
	default:
		return false
	}
	return true
}

func writeTwoFactorBody(w http.ResponseWriter, status int, code string, err error) {
	writeJSON(w, status, map[string]string{"error": code, "message": err.Error()})
}
