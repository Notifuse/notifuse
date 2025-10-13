package service

import (
	"context"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// LogSenderDetails logs detailed information about the sender being used
// This is useful for debugging From name issues
func logSenderDetails(ctx context.Context, log logger.Logger, sender *domain.EmailSender, context string) {
	if sender == nil {
		log.WithFields(map[string]interface{}{
			"context": context,
			"error":   "sender is nil",
		}).Error("Sender is nil")
		return
	}

	fields := map[string]interface{}{
		"context":      context,
		"sender_id":    sender.ID,
		"sender_email": sender.Email,
		"sender_name":  sender.Name,
		"is_default":   sender.IsDefault,
	}

	// Add warning if name is empty
	if sender.Name == "" {
		fields["warning"] = "SENDER NAME IS EMPTY"
		log.WithFields(fields).Warn("Sender has empty name - From header will only contain email")
	} else {
		log.WithFields(fields).Debug("Using sender with name")
	}
}

// ValidateSenderHasName checks if a sender has a name and logs a warning if not
func validateSenderHasName(log logger.Logger, sender *domain.EmailSender, context string) {
	if sender == nil {
		log.WithField("context", context).Error("Sender is nil")
		return
	}

	if sender.Name == "" {
		log.WithFields(map[string]interface{}{
			"context":      context,
			"sender_id":    sender.ID,
			"sender_email": sender.Email,
		}).Warn("⚠️ Sender has empty name - emails will be sent without From name")
	}
}

// DebugEmailProviderSenders logs all senders in an email provider
// Useful for debugging which senders are available
func debugEmailProviderSenders(log logger.Logger, provider *domain.EmailProvider, context string) {
	if provider == nil {
		log.WithField("context", context).Error("Email provider is nil")
		return
	}

	log.WithFields(map[string]interface{}{
		"context":       context,
		"provider_kind": provider.Kind,
		"sender_count":  len(provider.Senders),
	}).Debug("Email provider configuration")

	for i, sender := range provider.Senders {
		fields := map[string]interface{}{
			"context":      context,
			"index":        i,
			"sender_id":    sender.ID,
			"sender_email": sender.Email,
			"sender_name":  sender.Name,
			"is_default":   sender.IsDefault,
		}

		if sender.Name == "" {
			fields["warning"] = "EMPTY NAME"
			log.WithFields(fields).Warn("Sender has empty name")
		} else {
			log.WithFields(fields).Debug("Sender configuration")
		}
	}
}
