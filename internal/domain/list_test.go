package domain_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestList_Validate(t *testing.T) {
	tests := []struct {
		name    string
		list    domain.List
		wantErr bool
	}{
		{
			name: "valid list",
			list: domain.List{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				Description:   "This is a description",
			},
			wantErr: false,
		},
		{
			name: "valid list without description",
			list: domain.List{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: false,
			},
			wantErr: false,
		},
		{
			name: "invalid ID",
			list: domain.List{
				ID:            "",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "invalid name",
			list: domain.List{
				ID:            "list123",
				Name:          "",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.list.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestScanList(t *testing.T) {
	now := time.Now()
	// Create mock scanner
	scanner := &mockScanner{
		data: []interface{}{
			"list123",        // ID
			"My List",        // Name
			true,             // IsDoubleOptin
			true,             // IsPublic
			"This is a list", // Description
			10,               // TotalActive
			5,                // TotalPending
			2,                // TotalUnsubscribed
			1,                // TotalBounced
			0,                // TotalComplained
			nil,              // DoubleOptInTemplate
			nil,              // WelcomeTemplate
			nil,              // UnsubscribeTemplate
			now,              // CreatedAt
			now,              // UpdatedAt
			nil,              // DeletedAt
		},
	}

	// Test successful scan
	list, err := domain.ScanList(scanner)
	assert.NoError(t, err)
	assert.Equal(t, "list123", list.ID)
	assert.Equal(t, "My List", list.Name)
	assert.Equal(t, true, list.IsDoubleOptin)
	assert.Equal(t, true, list.IsPublic)
	assert.Equal(t, "This is a list", list.Description)
	assert.Equal(t, 10, list.TotalActive)
	assert.Equal(t, 5, list.TotalPending)
	assert.Equal(t, 2, list.TotalUnsubscribed)
	assert.Equal(t, 1, list.TotalBounced)
	assert.Equal(t, 0, list.TotalComplained)
	assert.Nil(t, list.DoubleOptInTemplate)
	assert.Nil(t, list.WelcomeTemplate)
	assert.Nil(t, list.UnsubscribeTemplate)
	assert.Equal(t, now, list.CreatedAt)
	assert.Equal(t, now, list.UpdatedAt)
	assert.Nil(t, list.DeletedAt)

	// Test scan error
	scanner.err = sql.ErrNoRows
	_, err = domain.ScanList(scanner)
	assert.Error(t, err)
}

// Mock scanner for testing
type mockScanner struct {
	data []interface{}
	err  error
}

func (m *mockScanner) Scan(dest ...interface{}) error {
	if m.err != nil {
		return m.err
	}
	for i, d := range dest {
		switch v := d.(type) {
		case *string:
			if s, ok := m.data[i].(string); ok {
				*v = s
			}
		case *bool:
			if b, ok := m.data[i].(bool); ok {
				*v = b
			}
		case *int:
			if n, ok := m.data[i].(int); ok {
				*v = n
			}
		case **domain.TemplateReference:
			if tr, ok := m.data[i].(*domain.TemplateReference); ok {
				*v = tr
			}
		case *time.Time:
			if t, ok := m.data[i].(time.Time); ok {
				*v = t
			}
		case **time.Time:
			if m.data[i] == nil {
				*v = nil
			} else if t, ok := m.data[i].(time.Time); ok {
				*v = &t
			}
		}
	}
	return nil
}

func TestErrListNotFound_Error(t *testing.T) {
	err := &domain.ErrListNotFound{Message: "test error message"}
	assert.Equal(t, "test error message", err.Error())
}

func TestCreateListRequest_Validate(t *testing.T) {
	tests := []struct {
		name     string
		request  domain.CreateListRequest
		wantErr  bool
		wantList *domain.List
	}{
		{
			name: "valid request",
			request: domain.CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "Test description",
				DoubleOptInTemplate: &domain.TemplateReference{
					ID:      "template123",
					Version: 1,
				},
			},
			wantErr: false,
			wantList: &domain.List{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "Test description",
				DoubleOptInTemplate: &domain.TemplateReference{
					ID:      "template123",
					Version: 1,
				},
			},
		},
		{
			name: "missing workspace ID",
			request: domain.CreateListRequest{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "missing ID",
			request: domain.CreateListRequest{
				WorkspaceID:   "workspace123",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "invalid ID format",
			request: domain.CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "invalid@id",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "missing name",
			request: domain.CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "double opt-in without template",
			request: domain.CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, workspaceID, err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, workspaceID)
				assert.Nil(t, list)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.request.WorkspaceID, workspaceID)
				assert.Equal(t, tt.wantList.ID, list.ID)
				assert.Equal(t, tt.wantList.Name, list.Name)
				assert.Equal(t, tt.wantList.IsDoubleOptin, list.IsDoubleOptin)
				assert.Equal(t, tt.wantList.IsPublic, list.IsPublic)
				assert.Equal(t, tt.wantList.Description, list.Description)
				assert.Equal(t, tt.wantList.DoubleOptInTemplate.ID, list.DoubleOptInTemplate.ID)
				assert.Equal(t, tt.wantList.DoubleOptInTemplate.Version, list.DoubleOptInTemplate.Version)
			}
		})
	}
}

func TestGetListsRequest_FromURLParams(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string][]string
		wantErr bool
		want    domain.GetListsRequest
	}{
		{
			name: "valid params",
			params: map[string][]string{
				"workspace_id": {"workspace123"},
			},
			wantErr: false,
			want: domain.GetListsRequest{
				WorkspaceID: "workspace123",
			},
		},
		{
			name:    "missing workspace ID",
			params:  map[string][]string{},
			wantErr: true,
		},
		{
			name: "invalid workspace ID format",
			params: map[string][]string{
				"workspace_id": {"invalid@workspace"},
			},
			wantErr: false,
			want: domain.GetListsRequest{
				WorkspaceID: "invalid@workspace",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &domain.GetListsRequest{}
			err := req.FromURLParams(tt.params)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want.WorkspaceID, req.WorkspaceID)
			}
		})
	}
}

func TestGetListRequest_FromURLParams(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string][]string
		wantErr bool
		want    domain.GetListRequest
	}{
		{
			name: "valid params",
			params: map[string][]string{
				"workspace_id": {"workspace123"},
				"id":           {"list123"},
			},
			wantErr: false,
			want: domain.GetListRequest{
				WorkspaceID: "workspace123",
				ID:          "list123",
			},
		},
		{
			name: "missing workspace ID",
			params: map[string][]string{
				"id": {"list123"},
			},
			wantErr: true,
		},
		{
			name: "missing list ID",
			params: map[string][]string{
				"workspace_id": {"workspace123"},
			},
			wantErr: true,
		},
		{
			name: "invalid workspace ID format",
			params: map[string][]string{
				"workspace_id": {"invalid@workspace"},
				"id":           {"list123"},
			},
			wantErr: false,
			want: domain.GetListRequest{
				WorkspaceID: "invalid@workspace",
				ID:          "list123",
			},
		},
		{
			name: "invalid list ID format",
			params: map[string][]string{
				"workspace_id": {"workspace123"},
				"id":           {"invalid@list"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &domain.GetListRequest{}
			err := req.FromURLParams(tt.params)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want.WorkspaceID, req.WorkspaceID)
				assert.Equal(t, tt.want.ID, req.ID)
			}
		})
	}
}

func TestUpdateListRequest_Validate(t *testing.T) {
	tests := []struct {
		name     string
		request  domain.UpdateListRequest
		wantErr  bool
		wantList *domain.List
	}{
		{
			name: "valid request",
			request: domain.UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "Test description",
				DoubleOptInTemplate: &domain.TemplateReference{
					ID:      "template123",
					Version: 1,
				},
			},
			wantErr: false,
			wantList: &domain.List{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "Test description",
				DoubleOptInTemplate: &domain.TemplateReference{
					ID:      "template123",
					Version: 1,
				},
			},
		},
		{
			name: "missing workspace ID",
			request: domain.UpdateListRequest{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "missing ID",
			request: domain.UpdateListRequest{
				WorkspaceID:   "workspace123",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "invalid ID format",
			request: domain.UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "invalid@id",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "missing name",
			request: domain.UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "double opt-in without template",
			request: domain.UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, workspaceID, err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, workspaceID)
				assert.Nil(t, list)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.request.WorkspaceID, workspaceID)
				assert.Equal(t, tt.wantList.ID, list.ID)
				assert.Equal(t, tt.wantList.Name, list.Name)
				assert.Equal(t, tt.wantList.IsDoubleOptin, list.IsDoubleOptin)
				assert.Equal(t, tt.wantList.IsPublic, list.IsPublic)
				assert.Equal(t, tt.wantList.Description, list.Description)
				assert.Equal(t, tt.wantList.DoubleOptInTemplate.ID, list.DoubleOptInTemplate.ID)
				assert.Equal(t, tt.wantList.DoubleOptInTemplate.Version, list.DoubleOptInTemplate.Version)
			}
		})
	}
}

func TestDeleteListRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request domain.DeleteListRequest
		wantErr bool
		wantID  string
	}{
		{
			name: "valid request",
			request: domain.DeleteListRequest{
				WorkspaceID: "workspace123",
				ID:          "list123",
			},
			wantErr: false,
			wantID:  "workspace123",
		},
		{
			name: "missing workspace ID",
			request: domain.DeleteListRequest{
				ID: "list123",
			},
			wantErr: true,
			wantID:  "",
		},
		{
			name: "missing list ID",
			request: domain.DeleteListRequest{
				WorkspaceID: "workspace123",
			},
			wantErr: true,
			wantID:  "",
		},
		{
			name: "invalid workspace ID format",
			request: domain.DeleteListRequest{
				WorkspaceID: "invalid@workspace",
				ID:          "list123",
			},
			wantErr: false,
			wantID:  "invalid@workspace",
		},
		{
			name: "invalid list ID format",
			request: domain.DeleteListRequest{
				WorkspaceID: "workspace123",
				ID:          "invalid@list",
			},
			wantErr: true,
			wantID:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspaceID, err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, workspaceID)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantID, workspaceID)
			}
		})
	}
}

func TestContactListTotalType_Validate(t *testing.T) {
	tests := []struct {
		name      string
		totalType domain.ContactListTotalType
		wantErr   bool
	}{
		{
			name:      "valid type - pending",
			totalType: domain.TotalTypePending,
			wantErr:   false,
		},
		{
			name:      "valid type - active",
			totalType: domain.TotalTypeActive,
			wantErr:   false,
		},
		{
			name:      "valid type - unsubscribed",
			totalType: domain.TotalTypeUnsubscribed,
			wantErr:   false,
		},
		{
			name:      "valid type - bounced",
			totalType: domain.TotalTypeBounced,
			wantErr:   false,
		},
		{
			name:      "valid type - complained",
			totalType: domain.TotalTypeComplained,
			wantErr:   false,
		},
		{
			name:      "invalid type",
			totalType: domain.ContactListTotalType("invalid"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.totalType.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
