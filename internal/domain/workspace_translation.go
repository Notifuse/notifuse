package domain

import (
	"context"
	"fmt"
	"time"
)

type WorkspaceTranslation struct {
	Locale    string    `json:"locale"`
	Content   MapOfAny  `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (wt *WorkspaceTranslation) Validate() error {
	if wt.Locale == "" {
		return fmt.Errorf("locale is required")
	}
	if len(wt.Locale) > 10 {
		return fmt.Errorf("locale exceeds max length of 10")
	}
	if wt.Content == nil {
		return fmt.Errorf("content is required")
	}
	return nil
}

type WorkspaceTranslationRepository interface {
	Upsert(ctx context.Context, workspaceID string, translation *WorkspaceTranslation) error
	GetByLocale(ctx context.Context, workspaceID string, locale string) (*WorkspaceTranslation, error)
	List(ctx context.Context, workspaceID string) ([]*WorkspaceTranslation, error)
	Delete(ctx context.Context, workspaceID string, locale string) error
}

type UpsertWorkspaceTranslationRequest struct {
	WorkspaceID string   `json:"workspace_id"`
	Locale      string   `json:"locale"`
	Content     MapOfAny `json:"content"`
}

func (r *UpsertWorkspaceTranslationRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.Locale == "" {
		return fmt.Errorf("locale is required")
	}
	if len(r.Locale) > 10 {
		return fmt.Errorf("locale exceeds max length of 10")
	}
	if r.Content == nil {
		return fmt.Errorf("content is required")
	}
	return nil
}

type ListWorkspaceTranslationsRequest struct {
	WorkspaceID string `json:"workspace_id"`
}

type DeleteWorkspaceTranslationRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Locale      string `json:"locale"`
}
