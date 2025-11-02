package service

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

// CreateDefaultSupabaseNotifications creates the default Supabase auth email transactional notifications
func (s *SupabaseService) CreateDefaultSupabaseNotifications(ctx context.Context, workspaceID, integrationID string, mappings *domain.SupabaseTemplateMappings) error {
	// Use system context to bypass authentication
	systemCtx := context.WithValue(ctx, domain.SystemCallKey, true)

	// Create signup notification
	signupNotificationID := fmt.Sprintf("supabase_signup_%06d", rand.Intn(1000000))
	integrationIDPtr := &integrationID

	signupNotification := &domain.TransactionalNotification{
		ID:            signupNotificationID,
		Name:          "Supabase Signup Confirmation",
		Description:   "Sends signup confirmation emails via Supabase integration",
		IntegrationID: integrationIDPtr,
		Channels: domain.ChannelTemplates{
			domain.TransactionalChannelEmail: domain.ChannelTemplate{
				TemplateID: mappings.Signup,
			},
		},
		TrackingSettings: notifuse_mjml.TrackingSettings{
			EnableTracking: false,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.transactionalRepo.Create(systemCtx, workspaceID, signupNotification); err != nil {
		return fmt.Errorf("failed to create signup notification: %w", err)
	}

	// Create magic link notification
	magicLinkNotificationID := fmt.Sprintf("supabase_magiclink_%06d", rand.Intn(1000000))
	magicLinkNotification := &domain.TransactionalNotification{
		ID:            magicLinkNotificationID,
		Name:          "Supabase Magic Link",
		Description:   "Sends magic link authentication emails via Supabase integration",
		IntegrationID: integrationIDPtr,
		Channels: domain.ChannelTemplates{
			domain.TransactionalChannelEmail: domain.ChannelTemplate{
				TemplateID: mappings.MagicLink,
			},
		},
		TrackingSettings: notifuse_mjml.TrackingSettings{
			EnableTracking: false,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.transactionalRepo.Create(systemCtx, workspaceID, magicLinkNotification); err != nil {
		return fmt.Errorf("failed to create magic link notification: %w", err)
	}

	// Create recovery notification
	recoveryNotificationID := fmt.Sprintf("supabase_recovery_%06d", rand.Intn(1000000))
	recoveryNotification := &domain.TransactionalNotification{
		ID:            recoveryNotificationID,
		Name:          "Supabase Password Recovery",
		Description:   "Sends password recovery emails via Supabase integration",
		IntegrationID: integrationIDPtr,
		Channels: domain.ChannelTemplates{
			domain.TransactionalChannelEmail: domain.ChannelTemplate{
				TemplateID: mappings.Recovery,
			},
		},
		TrackingSettings: notifuse_mjml.TrackingSettings{
			EnableTracking: false,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.transactionalRepo.Create(systemCtx, workspaceID, recoveryNotification); err != nil {
		return fmt.Errorf("failed to create recovery notification: %w", err)
	}

	// Create email change notification (single notification for both current and new email addresses)
	emailChangeNotificationID := fmt.Sprintf("supabase_email_change_%06d", rand.Intn(1000000))
	emailChangeNotification := &domain.TransactionalNotification{
		ID:            emailChangeNotificationID,
		Name:          "Supabase Email Change",
		Description:   "Sends email change confirmation via Supabase integration",
		IntegrationID: integrationIDPtr,
		Channels: domain.ChannelTemplates{
			domain.TransactionalChannelEmail: domain.ChannelTemplate{
				TemplateID: mappings.EmailChange,
			},
		},
		TrackingSettings: notifuse_mjml.TrackingSettings{
			EnableTracking: false,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.transactionalRepo.Create(systemCtx, workspaceID, emailChangeNotification); err != nil {
		return fmt.Errorf("failed to create email change notification: %w", err)
	}

	// Create invite notification
	inviteNotificationID := fmt.Sprintf("supabase_invite_%06d", rand.Intn(1000000))
	inviteNotification := &domain.TransactionalNotification{
		ID:            inviteNotificationID,
		Name:          "Supabase User Invitation",
		Description:   "Sends user invitation emails via Supabase integration",
		IntegrationID: integrationIDPtr,
		Channels: domain.ChannelTemplates{
			domain.TransactionalChannelEmail: domain.ChannelTemplate{
				TemplateID: mappings.Invite,
			},
		},
		TrackingSettings: notifuse_mjml.TrackingSettings{
			EnableTracking: false,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.transactionalRepo.Create(systemCtx, workspaceID, inviteNotification); err != nil {
		return fmt.Errorf("failed to create invite notification: %w", err)
	}

	// Create reauthentication notification
	reauthNotificationID := fmt.Sprintf("supabase_reauth_%06d", rand.Intn(1000000))
	reauthNotification := &domain.TransactionalNotification{
		ID:            reauthNotificationID,
		Name:          "Supabase Reauthentication",
		Description:   "Sends reauthentication verification via Supabase integration",
		IntegrationID: integrationIDPtr,
		Channels: domain.ChannelTemplates{
			domain.TransactionalChannelEmail: domain.ChannelTemplate{
				TemplateID: mappings.Reauthentication,
			},
		},
		TrackingSettings: notifuse_mjml.TrackingSettings{
			EnableTracking: false,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.transactionalRepo.Create(systemCtx, workspaceID, reauthNotification); err != nil {
		return fmt.Errorf("failed to create reauthentication notification: %w", err)
	}

	return nil
}
