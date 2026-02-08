package domain

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a valid EmailBlock for testing
func createTestEmailBlock() notifuse_mjml.EmailBlock {
	blockJSON := []byte(`{"id":"b1","type":"mj-text","content":"Hello","attributes":{"fontSize":"16px"}}`)
	blk, _ := notifuse_mjml.UnmarshalEmailBlock(blockJSON)
	return blk
}

// dummyEmptyTypeBlock is a test helper that implements EmailBlock but returns empty type
type dummyEmptyTypeBlock struct{}

func (d dummyEmptyTypeBlock) GetID() string                                   { return "dummy" }
func (d dummyEmptyTypeBlock) SetID(id string)                                 {}
func (d dummyEmptyTypeBlock) GetType() notifuse_mjml.MJMLComponentType        { return "" }
func (d dummyEmptyTypeBlock) GetChildren() []notifuse_mjml.EmailBlock         { return nil }
func (d dummyEmptyTypeBlock) SetChildren(children []notifuse_mjml.EmailBlock) {}
func (d dummyEmptyTypeBlock) GetAttributes() map[string]interface{}           { return nil }
func (d dummyEmptyTypeBlock) SetAttributes(attrs map[string]interface{})      {}
func (d dummyEmptyTypeBlock) GetContent() *string                             { return nil }
func (d dummyEmptyTypeBlock) SetContent(content *string)                      {}
func (d dummyEmptyTypeBlock) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"id":   d.GetID(),
		"type": d.GetType(),
	})
}

// TestTemplateBlock_MarshalJSON tests JSON marshaling of TemplateBlock
func TestTemplateBlock_MarshalJSON(t *testing.T) {
	block := createTestEmailBlock()
	tb := TemplateBlock{
		ID:      "test-id",
		Name:    "Test Block",
		Block:   block,
		Created: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		Updated: time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(tb)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "test-id", result["id"])
	assert.Equal(t, "Test Block", result["name"])
	assert.NotNil(t, result["block"])
	assert.NotNil(t, result["created"])
	assert.NotNil(t, result["updated"])
}

// TestTemplateBlock_UnmarshalJSON tests JSON unmarshaling of TemplateBlock
func TestTemplateBlock_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		check   func(*testing.T, *TemplateBlock)
	}{
		{
			name: "valid template block",
			json: `{
				"id": "test-id",
				"name": "Test Block",
				"block": {"id":"b1","type":"mj-text","content":"Hello","attributes":{"fontSize":"16px"}},
				"created": "2023-01-01T00:00:00Z",
				"updated": "2023-01-02T00:00:00Z"
			}`,
			wantErr: false,
			check: func(t *testing.T, tb *TemplateBlock) {
				assert.Equal(t, "test-id", tb.ID)
				assert.Equal(t, "Test Block", tb.Name)
				assert.NotNil(t, tb.Block)
				assert.Equal(t, notifuse_mjml.MJMLComponentMjText, tb.Block.GetType())
			},
		},
		{
			name: "null block",
			json: `{
				"id": "test-id",
				"name": "Test Block",
				"block": null,
				"created": "2023-01-01T00:00:00Z",
				"updated": "2023-01-02T00:00:00Z"
			}`,
			wantErr: false,
			check: func(t *testing.T, tb *TemplateBlock) {
				assert.Nil(t, tb.Block)
			},
		},
		{
			name: "empty string block",
			json: `{
				"id": "test-id",
				"name": "Test Block",
				"block": "",
				"created": "2023-01-01T00:00:00Z",
				"updated": "2023-01-02T00:00:00Z"
			}`,
			wantErr: false,
			check: func(t *testing.T, tb *TemplateBlock) {
				assert.Nil(t, tb.Block)
			},
		},
		{
			name: "invalid block JSON - missing type",
			json: `{
				"id": "test-id",
				"name": "Test Block",
				"block": {"id":"b1","content":"Hello"},
				"created": "2023-01-01T00:00:00Z",
				"updated": "2023-01-02T00:00:00Z"
			}`,
			wantErr: false, // UnmarshalEmailBlock may handle this gracefully
			check: func(t *testing.T, tb *TemplateBlock) {
				// Just verify it unmarshals without error
				assert.Equal(t, "test-id", tb.ID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tb TemplateBlock
			err := json.Unmarshal([]byte(tt.json), &tb)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.check != nil {
					tt.check(t, &tb)
				}
			}
		})
	}
}

// TestCreateTemplateBlockRequest_UnmarshalJSON tests JSON unmarshaling of CreateTemplateBlockRequest
func TestCreateTemplateBlockRequest_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		check   func(*testing.T, *CreateTemplateBlockRequest)
	}{
		{
			name: "valid request",
			json: `{
				"workspace_id": "workspace123",
				"name": "Test Block",
				"block": {"id":"b1","type":"mj-text","content":"Hello","attributes":{"fontSize":"16px"}}
			}`,
			wantErr: false,
			check: func(t *testing.T, req *CreateTemplateBlockRequest) {
				assert.Equal(t, "workspace123", req.WorkspaceID)
				assert.Equal(t, "Test Block", req.Name)
				assert.NotNil(t, req.Block)
			},
		},
		{
			name: "null block",
			json: `{
				"workspace_id": "workspace123",
				"name": "Test Block",
				"block": null
			}`,
			wantErr: false,
			check: func(t *testing.T, req *CreateTemplateBlockRequest) {
				assert.Nil(t, req.Block)
			},
		},
		{
			name: "invalid block JSON - missing type",
			json: `{
				"workspace_id": "workspace123",
				"name": "Test Block",
				"block": {"id":"b1","content":"Hello"}
			}`,
			wantErr: false, // UnmarshalEmailBlock may handle this gracefully
			check: func(t *testing.T, req *CreateTemplateBlockRequest) {
				// Just verify it unmarshals without error
				assert.Equal(t, "workspace123", req.WorkspaceID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req CreateTemplateBlockRequest
			err := json.Unmarshal([]byte(tt.json), &req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.check != nil {
					tt.check(t, &req)
				}
			}
		})
	}
}

func TestCreateTemplateBlockRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request *CreateTemplateBlockRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request",
			request: &CreateTemplateBlockRequest{
				WorkspaceID: "workspace123",
				Name:        "Test Block",
				Block:       createTestEmailBlock(),
			},
			wantErr: false,
		},
		{
			name: "missing workspace_id",
			request: &CreateTemplateBlockRequest{
				WorkspaceID: "",
				Name:        "Test Block",
				Block:       createTestEmailBlock(),
			},
			wantErr: true,
			errMsg:  "workspace_id is required",
		},
		{
			name: "non-alphanumeric workspace_id",
			request: &CreateTemplateBlockRequest{
				WorkspaceID: "workspace-123",
				Name:        "Test Block",
				Block:       createTestEmailBlock(),
			},
			wantErr: true,
			errMsg:  "workspace_id must be alphanumeric",
		},
		{
			name: "workspace_id too long",
			request: &CreateTemplateBlockRequest{
				WorkspaceID: strings.Repeat("a", 33),
				Name:        "Test Block",
				Block:       createTestEmailBlock(),
			},
			wantErr: true,
			errMsg:  "workspace_id length must be between 1 and 32",
		},
		{
			name: "missing name",
			request: &CreateTemplateBlockRequest{
				WorkspaceID: "workspace123",
				Name:        "",
				Block:       createTestEmailBlock(),
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "name too long",
			request: &CreateTemplateBlockRequest{
				WorkspaceID: "workspace123",
				Name:        strings.Repeat("a", 256),
				Block:       createTestEmailBlock(),
			},
			wantErr: true,
			errMsg:  "name length must be between 1 and 255",
		},
		{
			name: "missing block",
			request: &CreateTemplateBlockRequest{
				WorkspaceID: "workspace123",
				Name:        "Test Block",
				Block:       nil,
			},
			wantErr: true,
			errMsg:  "block is required",
		},
		{
			name: "block with empty type",
			request: &CreateTemplateBlockRequest{
				WorkspaceID: "workspace123",
				Name:        "Test Block",
				Block:       dummyEmptyTypeBlock{},
			},
			wantErr: true,
			errMsg:  "block kind is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block, workspaceID, err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, block)
				assert.Empty(t, workspaceID)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, block)
				assert.Equal(t, tt.request.WorkspaceID, workspaceID)
				assert.NotEmpty(t, block.ID)
				assert.Equal(t, tt.request.Name, block.Name)
			}
		})
	}
}

// TestUpdateTemplateBlockRequest_UnmarshalJSON tests JSON unmarshaling of UpdateTemplateBlockRequest
func TestUpdateTemplateBlockRequest_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		check   func(*testing.T, *UpdateTemplateBlockRequest)
	}{
		{
			name: "valid request",
			json: `{
				"workspace_id": "workspace123",
				"id": "block123",
				"name": "Updated Block",
				"block": {"id":"b1","type":"mj-text","content":"Hello","attributes":{"fontSize":"16px"}}
			}`,
			wantErr: false,
			check: func(t *testing.T, req *UpdateTemplateBlockRequest) {
				assert.Equal(t, "workspace123", req.WorkspaceID)
				assert.Equal(t, "block123", req.ID)
				assert.Equal(t, "Updated Block", req.Name)
				assert.NotNil(t, req.Block)
			},
		},
		{
			name: "null block",
			json: `{
				"workspace_id": "workspace123",
				"id": "block123",
				"name": "Updated Block",
				"block": null
			}`,
			wantErr: false,
			check: func(t *testing.T, req *UpdateTemplateBlockRequest) {
				assert.Nil(t, req.Block)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req UpdateTemplateBlockRequest
			err := json.Unmarshal([]byte(tt.json), &req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.check != nil {
					tt.check(t, &req)
				}
			}
		})
	}
}

func TestUpdateTemplateBlockRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request *UpdateTemplateBlockRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request",
			request: &UpdateTemplateBlockRequest{
				WorkspaceID: "workspace123",
				ID:          "block123",
				Name:        "Updated Block",
				Block:       createTestEmailBlock(),
			},
			wantErr: false,
		},
		{
			name: "missing workspace_id",
			request: &UpdateTemplateBlockRequest{
				WorkspaceID: "",
				ID:          "block123",
				Name:        "Updated Block",
				Block:       createTestEmailBlock(),
			},
			wantErr: true,
			errMsg:  "workspace_id is required",
		},
		{
			name: "missing id",
			request: &UpdateTemplateBlockRequest{
				WorkspaceID: "workspace123",
				ID:          "",
				Name:        "Updated Block",
				Block:       createTestEmailBlock(),
			},
			wantErr: true,
			errMsg:  "id is required",
		},
		{
			name: "missing name",
			request: &UpdateTemplateBlockRequest{
				WorkspaceID: "workspace123",
				ID:          "block123",
				Name:        "",
				Block:       createTestEmailBlock(),
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "name too long",
			request: &UpdateTemplateBlockRequest{
				WorkspaceID: "workspace123",
				ID:          "block123",
				Name:        strings.Repeat("a", 256),
				Block:       createTestEmailBlock(),
			},
			wantErr: true,
			errMsg:  "name length must be between 1 and 255",
		},
		{
			name: "missing block",
			request: &UpdateTemplateBlockRequest{
				WorkspaceID: "workspace123",
				ID:          "block123",
				Name:        "Updated Block",
				Block:       nil,
			},
			wantErr: true,
			errMsg:  "block is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block, workspaceID, err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, block)
				assert.Empty(t, workspaceID)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, block)
				assert.Equal(t, tt.request.WorkspaceID, workspaceID)
				assert.Equal(t, tt.request.ID, block.ID)
				assert.Equal(t, tt.request.Name, block.Name)
			}
		})
	}
}

func TestDeleteTemplateBlockRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request *DeleteTemplateBlockRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request",
			request: &DeleteTemplateBlockRequest{
				WorkspaceID: "workspace123",
				ID:          "block123",
			},
			wantErr: false,
		},
		{
			name: "missing workspace_id",
			request: &DeleteTemplateBlockRequest{
				WorkspaceID: "",
				ID:          "block123",
			},
			wantErr: true,
			errMsg:  "workspace_id is required",
		},
		{
			name: "non-alphanumeric workspace_id",
			request: &DeleteTemplateBlockRequest{
				WorkspaceID: "workspace-123",
				ID:          "block123",
			},
			wantErr: true,
			errMsg:  "workspace_id must be alphanumeric",
		},
		{
			name: "workspace_id too long",
			request: &DeleteTemplateBlockRequest{
				WorkspaceID: strings.Repeat("a", 33),
				ID:          "block123",
			},
			wantErr: true,
			errMsg:  "workspace_id length must be between 1 and 32",
		},
		{
			name: "missing id",
			request: &DeleteTemplateBlockRequest{
				WorkspaceID: "workspace123",
				ID:          "",
			},
			wantErr: true,
			errMsg:  "id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspaceID, id, err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Empty(t, workspaceID)
				assert.Empty(t, id)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.request.WorkspaceID, workspaceID)
				assert.Equal(t, tt.request.ID, id)
			}
		})
	}
}

func TestGetTemplateBlockRequest_FromURLParams(t *testing.T) {
	tests := []struct {
		name        string
		queryParams url.Values
		wantErr     bool
		errMsg      string
	}{
		{
			name: "valid params",
			queryParams: url.Values{
				"workspace_id": {"workspace123"},
				"id":           {"block123"},
			},
			wantErr: false,
		},
		{
			name: "missing workspace_id",
			queryParams: url.Values{
				"id": {"block123"},
			},
			wantErr: true,
			errMsg:  "workspace_id is required",
		},
		{
			name: "missing id",
			queryParams: url.Values{
				"workspace_id": {"workspace123"},
			},
			wantErr: true,
			errMsg:  "id is required",
		},
		{
			name: "workspace_id too long",
			queryParams: url.Values{
				"workspace_id": {strings.Repeat("a", 33)},
				"id":           {"block123"},
			},
			wantErr: true,
			errMsg:  "workspace_id length must be between 1 and 32",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &GetTemplateBlockRequest{}
			err := req.FromURLParams(tt.queryParams)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.queryParams.Get("workspace_id"), req.WorkspaceID)
				assert.Equal(t, tt.queryParams.Get("id"), req.ID)
			}
		})
	}
}

func TestListTemplateBlocksRequest_FromURLParams(t *testing.T) {
	tests := []struct {
		name        string
		queryParams url.Values
		wantErr     bool
		errMsg      string
	}{
		{
			name: "valid params",
			queryParams: url.Values{
				"workspace_id": {"workspace123"},
			},
			wantErr: false,
		},
		{
			name:        "missing workspace_id",
			queryParams: url.Values{},
			wantErr:     true,
			errMsg:      "workspace_id is required",
		},
		{
			name: "workspace_id too long",
			queryParams: url.Values{
				"workspace_id": {strings.Repeat("a", 33)},
			},
			wantErr: true,
			errMsg:  "workspace_id length must be between 1 and 32",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ListTemplateBlocksRequest{}
			err := req.FromURLParams(tt.queryParams)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.queryParams.Get("workspace_id"), req.WorkspaceID)
			}
		})
	}
}

// TestErrTemplateBlockNotFound tests the error type
func TestErrTemplateBlockNotFound(t *testing.T) {
	err := &ErrTemplateBlockNotFound{
		Message: "template block not found",
	}

	assert.Equal(t, "template block not found", err.Error())
	assert.Error(t, err)
}
