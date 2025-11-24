package domain

import (
	"encoding/json"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// segmentMockScanner implements the scanner interface for testing ScanSegment
type segmentMockScanner struct {
	values []interface{}
	err    error
}

func (m *segmentMockScanner) Scan(dest ...interface{}) error {
	if m.err != nil {
		return m.err
	}
	if len(m.values) != len(dest) {
		return errors.New("value count mismatch")
	}
	for i, val := range m.values {
		switch d := dest[i].(type) {
		case *string:
			if s, ok := val.(string); ok {
				*d = s
			}
		case *int:
			if i, ok := val.(int); ok {
				*d = i
			}
		case *int64:
			if i, ok := val.(int64); ok {
				*d = i
			}
		case *MapOfAny:
			if m, ok := val.(MapOfAny); ok {
				*d = m
			} else if b, ok := val.([]byte); ok {
				// Simulate JSON bytes
				*d = make(MapOfAny)
				if err := (*d).Scan(b); err != nil {
					return err
				}
			}
		case *JSONArray:
			if a, ok := val.(JSONArray); ok {
				*d = a
			} else if b, ok := val.([]byte); ok {
				// Simulate JSON bytes
				*d = make(JSONArray, 0)
				if err := (*d).Scan(b); err != nil {
					return err
				}
			}
		case **string:
			if s, ok := val.(string); ok {
				*d = &s
			} else if val == nil {
				*d = nil
			} else if sp, ok := val.(*string); ok {
				*d = sp
			}
		case **time.Time:
			if t, ok := val.(time.Time); ok {
				*d = &t
			} else if val == nil {
				*d = nil
			} else if tp, ok := val.(*time.Time); ok {
				*d = tp
			}
		case *time.Time:
			if t, ok := val.(time.Time); ok {
				*d = t
			}
		default:
			// Try to use reflection or direct assignment for other types
			// This handles interface{} types that might be passed
		}
	}
	return nil
}

func TestScanSegment(t *testing.T) {
	t.Run("successful scan", func(t *testing.T) {
		now := time.Now()
		treeData := MapOfAny{
			"kind": "leaf",
			"leaf": map[string]interface{}{
				"table": "contacts",
				"contact": map[string]interface{}{
					"filters": []interface{}{
						map[string]interface{}{
							"field_name":    "email",
							"field_type":    "string",
							"operator":      "equals",
							"string_values": []interface{}{"test@example.com"},
						},
					},
				},
			},
		}
		treeJSON, _ := json.Marshal(treeData)
		generatedSQL := "SELECT * FROM contacts"
		args := JSONArray{1, "test"}
		argsJSON, _ := json.Marshal(args)

		scanner := &segmentMockScanner{
			values: []interface{}{
				"segment123",           // ID
				"My Segment",           // Name
				"#FF5733",              // Color
				treeJSON,                // Tree (as JSON bytes)
				"America/New_York",     // Timezone
				int64(1),                // Version
				"active",                // Status
				&generatedSQL,           // GeneratedSQL
				argsJSON,                // GeneratedArgs (as JSON bytes)
				&now,                    // RecomputeAfter
				now,                     // DBCreatedAt
				now,                     // DBUpdatedAt
				42,                      // UsersCount
			},
		}

		segment, err := ScanSegment(scanner)
		require.NoError(t, err)
		assert.NotNil(t, segment)
		assert.Equal(t, "segment123", segment.ID)
		assert.Equal(t, "My Segment", segment.Name)
		assert.Equal(t, "#FF5733", segment.Color)
		assert.Equal(t, "America/New_York", segment.Timezone)
		assert.Equal(t, int64(1), segment.Version)
		assert.Equal(t, "active", segment.Status)
		assert.NotNil(t, segment.GeneratedSQL)
		assert.Equal(t, "SELECT * FROM contacts", *segment.GeneratedSQL)
		assert.NotNil(t, segment.GeneratedArgs)
		assert.Equal(t, 2, len(segment.GeneratedArgs))
		assert.NotNil(t, segment.RecomputeAfter)
		assert.Equal(t, now, *segment.RecomputeAfter)
		assert.Equal(t, 42, segment.UsersCount)
		assert.NotNil(t, segment.Tree)
		assert.Equal(t, "leaf", segment.Tree.Kind)
	})

	t.Run("scan error", func(t *testing.T) {
		scanner := &segmentMockScanner{
			err: errors.New("database error"),
		}

		segment, err := ScanSegment(scanner)
		assert.Error(t, err)
		assert.Nil(t, segment)
		assert.Equal(t, "database error", err.Error())
	})

	t.Run("invalid tree data - unmarshalable value", func(t *testing.T) {
		now := time.Now()
		generatedSQL := "SELECT * FROM contacts"
		
		// Create a MapOfAny with a channel value, which can't be marshaled to JSON
		// This will cause TreeNodeFromMapOfAny to fail when trying to marshal
		invalidMap := MapOfAny{
			"kind": make(chan int), // Channels can't be marshaled to JSON
		}
		// We need to pass this as bytes that will fail during unmarshal in TreeNodeFromMapOfAny
		// Actually, the MapOfAny.Scan will fail first when trying to unmarshal invalid JSON
		// Let's use a different approach - create a tree that will cause json.Marshal to fail
		// But we can't easily do that. Instead, let's test the error path by making
		// the scanner return an error that gets handled properly.
		// Actually, the simplest is to test with malformed JSON bytes that MapOfAny.Scan can handle
		// but TreeNodeFromMapOfAny can't process. But MapOfAny.Scan will fail first.
		
		// Let's try a different approach: use valid JSON but with a structure that causes
		// json.Marshal in TreeNodeFromMapOfAny to fail. But json.Marshal handles most things.
		// The only way json.Marshal fails is with circular references or unsupported types.
		// Since we're using MapOfAny which is map[string]any, we can put a channel in it.
		// But MapOfAny.Scan will try to unmarshal JSON bytes first, so we need to pass
		// the MapOfAny directly, not as bytes.
		
		scanner := &segmentMockScanner{
			values: []interface{}{
				"segment123",
				"My Segment",
				"#FF5733",
				invalidMap, // Direct MapOfAny with unmarshalable value
				"America/New_York",
				int64(1),
				"active",
				&generatedSQL,
				[]byte("[]"),
				nil,
				now,
				now,
				0,
			},
		}

		segment, err := ScanSegment(scanner)
		// This should fail when TreeNodeFromMapOfAny tries to marshal the MapOfAny
		// because channels can't be marshaled to JSON
		assert.Error(t, err)
		assert.Nil(t, segment)
		assert.Contains(t, err.Error(), "failed to parse segment tree")
	})

	t.Run("nil optional fields", func(t *testing.T) {
		now := time.Now()
		treeData := MapOfAny{
			"kind": "leaf",
			"leaf": map[string]interface{}{
				"table": "contacts",
				"contact": map[string]interface{}{
					"filters": []interface{}{},
				},
			},
		}
		treeJSON, _ := json.Marshal(treeData)

		scanner := &segmentMockScanner{
			values: []interface{}{
				"segment123",
				"My Segment",
				"#FF5733",
				treeJSON,
				"America/New_York",
				int64(1),
				"active",
				nil, // GeneratedSQL is nil
				[]byte("[]"), // GeneratedArgs
				nil, // RecomputeAfter is nil
				now,
				now,
				0,
			},
		}

		segment, err := ScanSegment(scanner)
		require.NoError(t, err)
		assert.NotNil(t, segment)
		assert.Nil(t, segment.GeneratedSQL)
		assert.Nil(t, segment.RecomputeAfter)
	})
}

func TestGetSegmentsRequest_FromURLParams(t *testing.T) {
	tests := []struct {
		name          string
		urlValues     url.Values
		wantWorkspace string
		wantWithCount bool
		wantErr       bool
	}{
		{
			name: "valid params with with_count=true",
			urlValues: url.Values{
				"workspace_id": []string{"workspace123"},
				"with_count":   []string{"true"},
			},
			wantWorkspace: "workspace123",
			wantWithCount: true,
			wantErr:       false,
		},
		{
			name: "valid params with with_count=1",
			urlValues: url.Values{
				"workspace_id": []string{"workspace123"},
				"with_count":   []string{"1"},
			},
			wantWorkspace: "workspace123",
			wantWithCount: true,
			wantErr:       false,
		},
		{
			name: "valid params with with_count=false",
			urlValues: url.Values{
				"workspace_id": []string{"workspace123"},
				"with_count":   []string{"false"},
			},
			wantWorkspace: "workspace123",
			wantWithCount: false,
			wantErr:       false,
		},
		{
			name: "valid params without with_count",
			urlValues: url.Values{
				"workspace_id": []string{"workspace123"},
			},
			wantWorkspace: "workspace123",
			wantWithCount: false,
			wantErr:       false,
		},
		{
			name: "missing workspace_id",
			urlValues: url.Values{
				"with_count": []string{"true"},
			},
			wantWorkspace: "",
			wantWithCount: false,
			wantErr:       true,
		},
		{
			name: "empty workspace_id",
			urlValues: url.Values{
				"workspace_id": []string{""},
				"with_count":   []string{"true"},
			},
			wantWorkspace: "",
			wantWithCount: false,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &GetSegmentsRequest{}
			err := req.FromURLParams(tt.urlValues)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "workspace_id is required")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantWorkspace, req.WorkspaceID)
				assert.Equal(t, tt.wantWithCount, req.WithCount)
			}
		})
	}
}

func TestGetSegmentRequest_FromURLParams(t *testing.T) {
	tests := []struct {
		name          string
		urlValues     url.Values
		wantWorkspace string
		wantID        string
		wantErr       bool
		errContains   string
	}{
		{
			name: "valid params",
			urlValues: url.Values{
				"workspace_id": []string{"workspace123"},
				"id":           []string{"segment123"},
			},
			wantWorkspace: "workspace123",
			wantID:        "segment123",
			wantErr:       false,
		},
		{
			name: "missing workspace_id",
			urlValues: url.Values{
				"id": []string{"segment123"},
			},
			wantWorkspace: "",
			wantID:        "",
			wantErr:       true,
			errContains:   "workspace_id is required",
		},
		{
			name: "empty workspace_id",
			urlValues: url.Values{
				"workspace_id": []string{""},
				"id":           []string{"segment123"},
			},
			wantWorkspace: "",
			wantID:        "",
			wantErr:       true,
			errContains:   "workspace_id is required",
		},
		{
			name: "missing id",
			urlValues: url.Values{
				"workspace_id": []string{"workspace123"},
			},
			wantWorkspace: "",
			wantID:        "",
			wantErr:       true,
			errContains:   "id is required",
		},
		{
			name: "empty id",
			urlValues: url.Values{
				"workspace_id": []string{"workspace123"},
				"id":           []string{""},
			},
			wantWorkspace: "",
			wantID:        "",
			wantErr:       true,
			errContains:   "id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &GetSegmentRequest{}
			err := req.FromURLParams(tt.urlValues)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantWorkspace, req.WorkspaceID)
				assert.Equal(t, tt.wantID, req.ID)
			}
		})
	}
}

func TestGetSegmentRequest_Validate(t *testing.T) {
	tests := []struct {
		name          string
		request       GetSegmentRequest
		wantWorkspace string
		wantID        string
		wantErr       bool
		errContains   string
	}{
		{
			name: "valid request",
			request: GetSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "segment123",
			},
			wantWorkspace: "workspace123",
			wantID:        "segment123",
			wantErr:       false,
		},
		{
			name: "missing workspace_id",
			request: GetSegmentRequest{
				WorkspaceID: "",
				ID:          "segment123",
			},
			wantWorkspace: "",
			wantID:        "",
			wantErr:       true,
			errContains:   "workspace_id is required",
		},
		{
			name: "missing id",
			request: GetSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "",
			},
			wantWorkspace: "",
			wantID:        "",
			wantErr:       true,
			errContains:   "id is required",
		},
		{
			name: "invalid id - non-alphanumeric",
			request: GetSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "segment-123",
			},
			wantWorkspace: "",
			wantID:        "",
			wantErr:       true,
			errContains:   "id must contain only lowercase letters, numbers, and underscores",
		},
		{
			name: "invalid id - uppercase",
			request: GetSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "Segment123",
			},
			wantWorkspace: "",
			wantID:        "",
			wantErr:       true,
			errContains:   "id must contain only lowercase letters, numbers, and underscores",
		},
		{
			name: "invalid id - too long",
			request: GetSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "segment123456789012345678901234567890",
			},
			wantWorkspace: "",
			wantID:        "",
			wantErr:       true,
			errContains:   "id length must be between 1 and 32",
		},
		{
			name: "valid id with underscores",
			request: GetSegmentRequest{
				WorkspaceID: "workspace123",
				ID:          "segment_123_test",
			},
			wantWorkspace: "workspace123",
			wantID:        "segment_123_test",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspaceID, id, err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Empty(t, workspaceID)
				assert.Empty(t, id)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantWorkspace, workspaceID)
				assert.Equal(t, tt.wantID, id)
			}
		})
	}
}

func TestErrSegmentNotFound_Error(t *testing.T) {
	t.Run("error message", func(t *testing.T) {
		err := &ErrSegmentNotFound{
			Message: "segment not found: segment123",
		}
		assert.Equal(t, "segment not found: segment123", err.Error())
	})

	t.Run("empty error message", func(t *testing.T) {
		err := &ErrSegmentNotFound{
			Message: "",
		}
		assert.Equal(t, "", err.Error())
	})
}
