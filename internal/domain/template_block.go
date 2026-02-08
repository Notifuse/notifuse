package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/asaskevich/govalidator"
	"github.com/google/uuid"
)

//go:generate mockgen -destination mocks/mock_template_block_service.go -package mocks github.com/Notifuse/notifuse/internal/domain TemplateBlockService

// TemplateBlock represents a reusable email block template
type TemplateBlock struct {
	ID      string                   `json:"id"`
	Name    string                   `json:"name"`
	Block   notifuse_mjml.EmailBlock `json:"block"`
	Created time.Time                `json:"created"`
	Updated time.Time                `json:"updated"`
}

// MarshalJSON implements custom JSON marshaling for TemplateBlock
func (tb TemplateBlock) MarshalJSON() ([]byte, error) {
	// Create a temporary struct with the same fields but Block as interface{}
	temp := struct {
		ID      string      `json:"id"`
		Name    string      `json:"name"`
		Block   interface{} `json:"block"`
		Created time.Time   `json:"created"`
		Updated time.Time   `json:"updated"`
	}{
		ID:      tb.ID,
		Name:    tb.Name,
		Block:   tb.Block,
		Created: tb.Created,
		Updated: tb.Updated,
	}
	return json.Marshal(temp)
}

// UnmarshalJSON implements custom JSON unmarshaling for TemplateBlock
func (tb *TemplateBlock) UnmarshalJSON(data []byte) error {
	// First unmarshal into a temporary struct
	temp := struct {
		ID      string          `json:"id"`
		Name    string          `json:"name"`
		Block   json.RawMessage `json:"block"`
		Created time.Time       `json:"created"`
		Updated time.Time       `json:"updated"`
	}{}

	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// Set the simple fields
	tb.ID = temp.ID
	tb.Name = temp.Name
	tb.Created = temp.Created
	tb.Updated = temp.Updated

	// Unmarshal the Block using the existing EmailBlock unmarshaling logic
	if len(temp.Block) > 0 {
		// Skip if it's just an empty string or null - these are not valid EmailBlock JSON
		blockStr := string(temp.Block)
		if blockStr == `""` || blockStr == `null` {
			tb.Block = nil
		} else {
			block, err := notifuse_mjml.UnmarshalEmailBlock(temp.Block)
			if err != nil {
				return fmt.Errorf("failed to unmarshal template block: %w", err)
			}
			tb.Block = block
		}
	}

	return nil
}

// Request Types

// CreateTemplateBlockRequest defines the request structure for creating a template block
type CreateTemplateBlockRequest struct {
	WorkspaceID string                   `json:"workspace_id"`
	Name        string                   `json:"name"`
	Block       notifuse_mjml.EmailBlock `json:"block"`
}

// UnmarshalJSON implements custom JSON unmarshaling for CreateTemplateBlockRequest
func (r *CreateTemplateBlockRequest) UnmarshalJSON(data []byte) error {
	// First unmarshal into a temporary struct
	temp := struct {
		WorkspaceID string          `json:"workspace_id"`
		Name        string          `json:"name"`
		Block       json.RawMessage `json:"block"`
	}{}

	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	r.WorkspaceID = temp.WorkspaceID
	r.Name = temp.Name

	// Unmarshal the Block using the existing EmailBlock unmarshaling logic
	if len(temp.Block) > 0 {
		blockStr := string(temp.Block)
		if blockStr == `""` || blockStr == `null` {
			r.Block = nil
		} else {
			block, err := notifuse_mjml.UnmarshalEmailBlock(temp.Block)
			if err != nil {
				return fmt.Errorf("failed to unmarshal template block: %w", err)
			}
			r.Block = block
		}
	}

	return nil
}

// Validate validates the create template block request
func (r *CreateTemplateBlockRequest) Validate() (block *TemplateBlock, workspaceID string, err error) {
	if r.WorkspaceID == "" {
		return nil, "", fmt.Errorf("invalid create template block request: workspace_id is required")
	}
	if !govalidator.IsAlphanumeric(r.WorkspaceID) {
		return nil, "", fmt.Errorf("invalid create template block request: workspace_id must be alphanumeric")
	}
	if len(r.WorkspaceID) > 32 {
		return nil, "", fmt.Errorf("invalid create template block request: workspace_id length must be between 1 and 32")
	}

	if r.Name == "" {
		return nil, "", fmt.Errorf("invalid create template block request: name is required")
	}
	if len(r.Name) > 255 {
		return nil, "", fmt.Errorf("invalid create template block request: name length must be between 1 and 255")
	}

	if r.Block == nil {
		return nil, "", fmt.Errorf("invalid create template block request: block is required")
	}
	if r.Block.GetType() == "" {
		return nil, "", fmt.Errorf("invalid create template block request: block kind is required")
	}

	now := time.Now().UTC()
	return &TemplateBlock{
		ID:      uuid.New().String(),
		Name:    r.Name,
		Block:   r.Block,
		Created: now,
		Updated: now,
	}, r.WorkspaceID, nil
}

// UpdateTemplateBlockRequest defines the request structure for updating a template block
type UpdateTemplateBlockRequest struct {
	WorkspaceID string                   `json:"workspace_id"`
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Block       notifuse_mjml.EmailBlock `json:"block"`
}

// UnmarshalJSON implements custom JSON unmarshaling for UpdateTemplateBlockRequest
func (r *UpdateTemplateBlockRequest) UnmarshalJSON(data []byte) error {
	// First unmarshal into a temporary struct
	temp := struct {
		WorkspaceID string          `json:"workspace_id"`
		ID          string          `json:"id"`
		Name        string          `json:"name"`
		Block       json.RawMessage `json:"block"`
	}{}

	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	r.WorkspaceID = temp.WorkspaceID
	r.ID = temp.ID
	r.Name = temp.Name

	// Unmarshal the Block using the existing EmailBlock unmarshaling logic
	if len(temp.Block) > 0 {
		blockStr := string(temp.Block)
		if blockStr == `""` || blockStr == `null` {
			r.Block = nil
		} else {
			block, err := notifuse_mjml.UnmarshalEmailBlock(temp.Block)
			if err != nil {
				return fmt.Errorf("failed to unmarshal template block: %w", err)
			}
			r.Block = block
		}
	}

	return nil
}

// Validate validates the update template block request
func (r *UpdateTemplateBlockRequest) Validate() (block *TemplateBlock, workspaceID string, err error) {
	if r.WorkspaceID == "" {
		return nil, "", fmt.Errorf("invalid update template block request: workspace_id is required")
	}
	if !govalidator.IsAlphanumeric(r.WorkspaceID) {
		return nil, "", fmt.Errorf("invalid update template block request: workspace_id must be alphanumeric")
	}
	if len(r.WorkspaceID) > 32 {
		return nil, "", fmt.Errorf("invalid update template block request: workspace_id length must be between 1 and 32")
	}

	if r.ID == "" {
		return nil, "", fmt.Errorf("invalid update template block request: id is required")
	}

	if r.Name == "" {
		return nil, "", fmt.Errorf("invalid update template block request: name is required")
	}
	if len(r.Name) > 255 {
		return nil, "", fmt.Errorf("invalid update template block request: name length must be between 1 and 255")
	}

	if r.Block == nil {
		return nil, "", fmt.Errorf("invalid update template block request: block is required")
	}
	if r.Block.GetType() == "" {
		return nil, "", fmt.Errorf("invalid update template block request: block kind is required")
	}

	return &TemplateBlock{
		ID:      r.ID,
		Name:    r.Name,
		Block:   r.Block,
		Updated: time.Now().UTC(),
	}, r.WorkspaceID, nil
}

// DeleteTemplateBlockRequest defines the request structure for deleting a template block
type DeleteTemplateBlockRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

// Validate validates the delete template block request
func (r *DeleteTemplateBlockRequest) Validate() (workspaceID string, id string, err error) {
	if r.WorkspaceID == "" {
		return "", "", fmt.Errorf("invalid delete template block request: workspace_id is required")
	}
	if !govalidator.IsAlphanumeric(r.WorkspaceID) {
		return "", "", fmt.Errorf("invalid delete template block request: workspace_id must be alphanumeric")
	}
	if len(r.WorkspaceID) > 32 {
		return "", "", fmt.Errorf("invalid delete template block request: workspace_id length must be between 1 and 32")
	}

	if r.ID == "" {
		return "", "", fmt.Errorf("invalid delete template block request: id is required")
	}

	return r.WorkspaceID, r.ID, nil
}

// GetTemplateBlockRequest defines the request structure for getting a template block
type GetTemplateBlockRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ID          string `json:"id"`
}

// FromURLParams parses the request from URL query parameters
func (r *GetTemplateBlockRequest) FromURLParams(queryParams url.Values) error {
	r.WorkspaceID = queryParams.Get("workspace_id")
	r.ID = queryParams.Get("id")

	if r.WorkspaceID == "" {
		return fmt.Errorf("invalid get template block request: workspace_id is required")
	}
	if len(r.WorkspaceID) > 32 {
		return fmt.Errorf("invalid get template block request: workspace_id length must be between 1 and 32")
	}

	if r.ID == "" {
		return fmt.Errorf("invalid get template block request: id is required")
	}

	return nil
}

// ListTemplateBlocksRequest defines the request structure for listing template blocks
type ListTemplateBlocksRequest struct {
	WorkspaceID string `json:"workspace_id"`
}

// FromURLParams parses the request from URL query parameters
func (r *ListTemplateBlocksRequest) FromURLParams(queryParams url.Values) error {
	r.WorkspaceID = queryParams.Get("workspace_id")

	if r.WorkspaceID == "" {
		return fmt.Errorf("invalid list template blocks request: workspace_id is required")
	}
	if len(r.WorkspaceID) > 32 {
		return fmt.Errorf("invalid list template blocks request: workspace_id length must be between 1 and 32")
	}

	return nil
}

// ErrTemplateBlockNotFound is returned when a template block is not found
type ErrTemplateBlockNotFound struct {
	Message string
}

func (e *ErrTemplateBlockNotFound) Error() string {
	return e.Message
}

// TemplateBlockService provides operations for managing template blocks
type TemplateBlockService interface {
	// CreateTemplateBlock creates a new template block
	CreateTemplateBlock(ctx context.Context, workspaceID string, block *TemplateBlock) error

	// GetTemplateBlock retrieves a template block by ID
	GetTemplateBlock(ctx context.Context, workspaceID string, id string) (*TemplateBlock, error)

	// ListTemplateBlocks retrieves all template blocks for a workspace
	ListTemplateBlocks(ctx context.Context, workspaceID string) ([]*TemplateBlock, error)

	// UpdateTemplateBlock updates an existing template block
	UpdateTemplateBlock(ctx context.Context, workspaceID string, block *TemplateBlock) error

	// DeleteTemplateBlock deletes a template block by ID
	DeleteTemplateBlock(ctx context.Context, workspaceID string, id string) error
}
