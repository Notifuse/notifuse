package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test helper: creates a simple valid tree for testing
func validTestTree() *TreeNode {
	return &TreeNode{
		Kind: "leaf",
		Leaf: &TreeNodeLeaf{
			Table: "contacts",
			Contact: &ContactCondition{
				Filters: []*DimensionFilter{
					{
						FieldName:    "email",
						FieldType:    "string",
						Operator:     "equals",
						StringValues: []string{"test@example.com"},
					},
				},
			},
		},
	}
}

func TestSegment_Validate(t *testing.T) {

	tests := []struct {
		name    string
		segment Segment
		wantErr bool
	}{
		{
			name: "valid segment",
			segment: Segment{
				ID:       "segment123",
				Name:     "My Segment",
				Color:    "#FF5733",
				Tree:     validTestTree(),
				Timezone: "America/New_York",
				Version:  1,
				Status:   string(SegmentStatusActive),
			},
			wantErr: false,
		},
		{
			name: "invalid ID - empty",
			segment: Segment{
				ID:       "",
				Name:     "My Segment",
				Color:    "#FF5733",
				Tree:     validTestTree(),
				Timezone: "America/New_York",
				Version:  1,
				Status:   string(SegmentStatusActive),
			},
			wantErr: true,
		},
		{
			name: "invalid ID - non-alphanumeric",
			segment: Segment{
				ID:       "segment-123",
				Name:     "My Segment",
				Color:    "#FF5733",
				Tree:     validTestTree(),
				Timezone: "America/New_York",
				Version:  1,
				Status:   string(SegmentStatusActive),
			},
			wantErr: true,
		},
		{
			name: "invalid ID - too long",
			segment: Segment{
				ID:       "segment1234567890123456789012345678901234567890",
				Name:     "My Segment",
				Color:    "#FF5733",
				Tree:     validTestTree(),
				Timezone: "America/New_York",
				Version:  1,
				Status:   string(SegmentStatusActive),
			},
			wantErr: true,
		},
		{
			name: "invalid name - empty",
			segment: Segment{
				ID:       "segment123",
				Name:     "",
				Color:    "#FF5733",
				Tree:     validTestTree(),
				Timezone: "America/New_York",
				Version:  1,
				Status:   string(SegmentStatusActive),
			},
			wantErr: true,
		},
		{
			name: "invalid name - too long",
			segment: Segment{
				ID:       "segment123",
				Name:     string(make([]byte, 256)),
				Color:    "#FF5733",
				Tree:     validTestTree(),
				Timezone: "America/New_York",
				Version:  1,
				Status:   string(SegmentStatusActive),
			},
			wantErr: true,
		},
		{
			name: "invalid color - empty",
			segment: Segment{
				ID:       "segment123",
				Name:     "My Segment",
				Color:    "",
				Tree:     validTestTree(),
				Timezone: "America/New_York",
				Version:  1,
				Status:   string(SegmentStatusActive),
			},
			wantErr: true,
		},
		{
			name: "invalid color - too long",
			segment: Segment{
				ID:       "segment123",
				Name:     "My Segment",
				Color:    string(make([]byte, 51)),
				Tree:     validTestTree(),
				Timezone: "America/New_York",
				Version:  1,
				Status:   string(SegmentStatusActive),
			},
			wantErr: true,
		},
		{
			name: "invalid timezone - empty",
			segment: Segment{
				ID:       "segment123",
				Name:     "My Segment",
				Color:    "#FF5733",
				Tree:     validTestTree(),
				Timezone: "",
				Version:  1,
				Status:   string(SegmentStatusActive),
			},
			wantErr: true,
		},
		{
			name: "invalid timezone - too long",
			segment: Segment{
				ID:       "segment123",
				Name:     "My Segment",
				Color:    "#FF5733",
				Tree:     validTestTree(),
				Timezone: string(make([]byte, 101)),
				Version:  1,
				Status:   string(SegmentStatusActive),
			},
			wantErr: true,
		},
		{
			name: "invalid version - zero",
			segment: Segment{
				ID:       "segment123",
				Name:     "My Segment",
				Color:    "#FF5733",
				Tree:     validTestTree(),
				Timezone: "America/New_York",
				Version:  0,
				Status:   string(SegmentStatusActive),
			},
			wantErr: true,
		},
		{
			name: "invalid version - negative",
			segment: Segment{
				ID:       "segment123",
				Name:     "My Segment",
				Color:    "#FF5733",
				Tree:     validTestTree(),
				Timezone: "America/New_York",
				Version:  -1,
				Status:   string(SegmentStatusActive),
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			segment: Segment{
				ID:       "segment123",
				Name:     "My Segment",
				Color:    "#FF5733",
				Tree:     validTestTree(),
				Timezone: "America/New_York",
				Version:  1,
				Status:   "invalid_status",
			},
			wantErr: true,
		},
		{
			name: "invalid tree - nil",
			segment: Segment{
				ID:       "segment123",
				Name:     "My Segment",
				Color:    "#FF5733",
				Tree:     nil,
				Timezone: "America/New_York",
				Version:  1,
				Status:   string(SegmentStatusActive),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.segment.Validate()
			if tt.wantErr {
				assert.Error(t, err, "expected validation error")
			} else {
				assert.NoError(t, err, "expected no validation error")
			}
		})
	}
}

func TestSegmentStatus_Validate(t *testing.T) {
	tests := []struct {
		name    string
		status  SegmentStatus
		wantErr bool
	}{
		{
			name:    "valid status - active",
			status:  SegmentStatusActive,
			wantErr: false,
		},
		{
			name:    "valid status - deleted",
			status:  SegmentStatusDeleted,
			wantErr: false,
		},
		{
			name:    "valid status - building",
			status:  SegmentStatusBuilding,
			wantErr: false,
		},
		{
			name:    "invalid status",
			status:  "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.status.Validate()
			if tt.wantErr {
				assert.Error(t, err, "expected validation error")
			} else {
				assert.NoError(t, err, "expected no validation error")
			}
		})
	}
}

func TestCreateSegmentRequest_Validate(t *testing.T) {

	tests := []struct {
		name    string
		request CreateSegmentRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: CreateSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "segment123",
				Name:        "My Segment",
				Color:       "#FF5733",
				Tree:        validTestTree(),
				Timezone:    "America/New_York",
			},
			wantErr: false,
		},
		{
			name: "invalid request - empty workspace ID",
			request: CreateSegmentRequest{
				WorkspaceID: "",
				ID:          "segment123",
				Name:        "My Segment",
				Color:       "#FF5733",
				Tree:        validTestTree(),
				Timezone:    "America/New_York",
			},
			wantErr: true,
		},
		{
			name: "invalid request - empty ID",
			request: CreateSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "",
				Name:        "My Segment",
				Color:       "#FF5733",
				Tree:        validTestTree(),
				Timezone:    "America/New_York",
			},
			wantErr: true,
		},
		{
			name: "invalid request - empty name",
			request: CreateSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "segment123",
				Name:        "",
				Color:       "#FF5733",
				Tree:        validTestTree(),
				Timezone:    "America/New_York",
			},
			wantErr: true,
		},
		{
			name: "invalid request - empty color",
			request: CreateSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "segment123",
				Name:        "My Segment",
				Color:       "",
				Tree:        validTestTree(),
				Timezone:    "America/New_York",
			},
			wantErr: true,
		},
		{
			name: "invalid request - empty timezone",
			request: CreateSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "segment123",
				Name:        "My Segment",
				Color:       "#FF5733",
				Tree:        validTestTree(),
				Timezone:    "",
			},
			wantErr: true,
		},
		{
			name: "invalid request - empty tree",
			request: CreateSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "segment123",
				Name:        "My Segment",
				Color:       "#FF5733",
				Tree:        nil,
				Timezone:    "America/New_York",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segment, workspaceID, err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err, "expected validation error")
				assert.Nil(t, segment)
				assert.Empty(t, workspaceID)
			} else {
				assert.NoError(t, err, "expected no validation error")
				assert.NotNil(t, segment)
				assert.Equal(t, tt.request.WorkspaceID, workspaceID)
				assert.Equal(t, tt.request.ID, segment.ID)
				assert.Equal(t, tt.request.Name, segment.Name)
				assert.Equal(t, tt.request.Color, segment.Color)
				assert.Equal(t, tt.request.Timezone, segment.Timezone)
				assert.Equal(t, int64(1), segment.Version)
				assert.Equal(t, string(SegmentStatusBuilding), segment.Status)
			}
		})
	}
}

func TestUpdateSegmentRequest_Validate(t *testing.T) {

	tests := []struct {
		name    string
		request UpdateSegmentRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: UpdateSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "segment123",
				Name:        "Updated Segment",
				Color:       "#FF5733",
				Tree:        validTestTree(),
				Timezone:    "America/New_York",
			},
			wantErr: false,
		},
		{
			name: "invalid request - empty workspace ID",
			request: UpdateSegmentRequest{
				WorkspaceID: "",
				ID:          "segment123",
				Name:        "Updated Segment",
				Color:       "#FF5733",
				Tree:        validTestTree(),
				Timezone:    "America/New_York",
			},
			wantErr: true,
		},
		{
			name: "invalid request - empty ID",
			request: UpdateSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "",
				Name:        "Updated Segment",
				Color:       "#FF5733",
				Tree:        validTestTree(),
				Timezone:    "America/New_York",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segment, workspaceID, err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err, "expected validation error")
				assert.Nil(t, segment)
				assert.Empty(t, workspaceID)
			} else {
				assert.NoError(t, err, "expected no validation error")
				assert.NotNil(t, segment)
				assert.Equal(t, tt.request.WorkspaceID, workspaceID)
				assert.Equal(t, tt.request.ID, segment.ID)
				assert.Equal(t, tt.request.Name, segment.Name)
			}
		})
	}
}

func TestDeleteSegmentRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request DeleteSegmentRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: DeleteSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "segment123",
			},
			wantErr: false,
		},
		{
			name: "invalid request - empty workspace ID",
			request: DeleteSegmentRequest{
				WorkspaceID: "",
				ID:          "segment123",
			},
			wantErr: true,
		},
		{
			name: "invalid request - empty ID",
			request: DeleteSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "",
			},
			wantErr: true,
		},
		{
			name: "invalid request - non-alphanumeric ID",
			request: DeleteSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "segment-123",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspaceID, id, err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err, "expected validation error")
				assert.Empty(t, workspaceID)
				assert.Empty(t, id)
			} else {
				assert.NoError(t, err, "expected no validation error")
				assert.Equal(t, tt.request.WorkspaceID, workspaceID)
				assert.Equal(t, tt.request.ID, id)
			}
		})
	}
}
