package domain

import (
	"database/sql"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestList_Validate(t *testing.T) {
	tests := []struct {
		name    string
		list    List
		wantErr bool
	}{
		{
			name: "valid list",
			list: List{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				Description:   "This is a description",
			},
			wantErr: false,
		},
		{
			name: "valid list without description",
			list: List{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: false,
			},
			wantErr: false,
		},
		{
			name: "invalid ID",
			list: List{
				ID:            "",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "non-alphanumeric ID",
			list: List{
				ID:            "list-123",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "ID too long",
			list: List{
				ID:            "list1234567890123456789012345678901234567890",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "invalid name",
			list: List{
				ID:            "list123",
				Name:          "",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "name too long",
			list: List{
				ID:            "list123",
				Name:          string(make([]byte, 256)),
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "invalid double opt-in template",
			list: List{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				DoubleOptInTemplate: &TemplateReference{
					ID:      "",
					Version: 0,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid welcome template",
			list: List{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: false,
				WelcomeTemplate: &TemplateReference{
					ID:      "",
					Version: 0,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid unsubscribe template",
			list: List{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: false,
				UnsubscribeTemplate: &TemplateReference{
					ID:      "",
					Version: 0,
				},
			},
			wantErr: true,
		},
		{
			name: "valid with all template references",
			list: List{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				DoubleOptInTemplate: &TemplateReference{
					ID:      "template1",
					Version: 1,
				},
				WelcomeTemplate: &TemplateReference{
					ID:      "template2",
					Version: 1,
				},
				UnsubscribeTemplate: &TemplateReference{
					ID:      "template3",
					Version: 1,
				},
			},
			wantErr: false,
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

	// Test 1: Basic list without templates
	scanner := &listMockScanner{
		data: []interface{}{
			"list123",        // ID
			"My List",        // Name
			true,             // IsDoubleOptin
			true,             // IsPublic
			"This is a list", // Description
			nil,              // DoubleOptInTemplate
			nil,              // WelcomeTemplate
			nil,              // UnsubscribeTemplate
			nil,              // Slug
			false,            // WebPublicationEnabled
			nil,              // WebPublicationSettings
			now,              // CreatedAt
			now,              // UpdatedAt
			nil,              // DeletedAt
		},
	}

	// Test successful scan without templates
	list, err := ScanList(scanner)
	assert.NoError(t, err)
	assert.Equal(t, "list123", list.ID)
	assert.Equal(t, "My List", list.Name)
	assert.Equal(t, true, list.IsDoubleOptin)
	assert.Equal(t, true, list.IsPublic)
	assert.Equal(t, "This is a list", list.Description)
	assert.Nil(t, list.DoubleOptInTemplate)
	assert.Nil(t, list.WelcomeTemplate)
	assert.Nil(t, list.UnsubscribeTemplate)
	assert.Nil(t, list.Slug)
	assert.Equal(t, false, list.WebPublicationEnabled)
	assert.Nil(t, list.WebPublicationSettings)
	assert.Equal(t, now, list.CreatedAt)
	assert.Equal(t, now, list.UpdatedAt)
	assert.Nil(t, list.DeletedAt)

	// Test scan error
	scanner.err = sql.ErrNoRows
	_, err = ScanList(scanner)
	assert.Error(t, err)
}

// Mock scanner for testing
type listMockScanner struct {
	data []interface{}
	err  error
}

func (m *listMockScanner) Scan(dest ...interface{}) error {
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
		case **TemplateReference:
			if tr, ok := m.data[i].(*TemplateReference); ok {
				*v = tr
			}
		case **string:
			if m.data[i] == nil {
				*v = nil
			} else if s, ok := m.data[i].(string); ok {
				*v = &s
			}
		case **WebPublicationSettings:
			if wps, ok := m.data[i].(*WebPublicationSettings); ok {
				*v = wps
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
	err := &ErrListNotFound{Message: "test error message"}
	assert.Equal(t, "test error message", err.Error())
}

func TestUnsubscribeFromListsRequest_FromURLParams_ExistingFile(t *testing.T) {
	vals := url.Values{}
	vals.Set("workspace_id", "wid")
	vals.Set("email", "user@example.com")
	vals.Set("email_hmac", "hmac")
	vals.Set("mid", "msg-1")
	vals["list_ids"] = []string{"l1", "l2"}

	var req UnsubscribeFromListsRequest
	if err := req.FromURLParams(vals); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.WorkspaceID != "wid" || req.Email != "user@example.com" || req.MessageID != "msg-1" {
		t.Fatalf("parsed fields mismatch: %+v", req)
	}
	if len(req.ListIDs) != 2 || req.ListIDs[0] != "l1" || req.ListIDs[1] != "l2" {
		t.Fatalf("expected list ids [l1 l2], got %#v", req.ListIDs)
	}

	// missing required field
	bad := url.Values{}
	if err := req.FromURLParams(bad); err == nil {
		t.Fatalf("expected error on missing fields")
	}
}

func TestCreateListRequest_Validate(t *testing.T) {
	tests := []struct {
		name     string
		request  CreateListRequest
		wantErr  bool
		wantList *List
	}{
		{
			name: "valid request",
			request: CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "Test description",
				DoubleOptInTemplate: &TemplateReference{
					ID:      "template123",
					Version: 1,
				},
			},
			wantErr: false,
			wantList: &List{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "Test description",
				DoubleOptInTemplate: &TemplateReference{
					ID:      "template123",
					Version: 1,
				},
			},
		},
		{
			name: "valid request with all templates",
			request: CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "Test description",
				DoubleOptInTemplate: &TemplateReference{
					ID:      "template1",
					Version: 1,
				},
				WelcomeTemplate: &TemplateReference{
					ID:      "template2",
					Version: 1,
				},
				UnsubscribeTemplate: &TemplateReference{
					ID:      "template3",
					Version: 1,
				},
			},
			wantErr: false,
			wantList: &List{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "Test description",
				DoubleOptInTemplate: &TemplateReference{
					ID:      "template1",
					Version: 1,
				},
				WelcomeTemplate: &TemplateReference{
					ID:      "template2",
					Version: 1,
				},
				UnsubscribeTemplate: &TemplateReference{
					ID:      "template3",
					Version: 1,
				},
			},
		},
		{
			name: "valid request with no double opt-in",
			request: CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: false,
				IsPublic:      true,
				Description:   "Test description",
			},
			wantErr: false,
			wantList: &List{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: false,
				IsPublic:      true,
				Description:   "Test description",
			},
		},
		{
			name: "missing workspace ID",
			request: CreateListRequest{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "missing ID",
			request: CreateListRequest{
				WorkspaceID:   "workspace123",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "invalid ID format",
			request: CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "invalid@id",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "ID too long",
			request: CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list1234567890123456789012345678901234567890",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "missing name",
			request: CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "name too long",
			request: CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          string(make([]byte, 256)),
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "double opt-in without template",
			request: CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "invalid double opt-in template",
			request: CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				DoubleOptInTemplate: &TemplateReference{
					ID:      "",
					Version: 0,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid welcome template",
			request: CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: false,
				WelcomeTemplate: &TemplateReference{
					ID:      "",
					Version: 0,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid unsubscribe template",
			request: CreateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: false,
				UnsubscribeTemplate: &TemplateReference{
					ID:      "",
					Version: 0,
				},
			},
			wantErr: true,
		},
		{
			name: "valid request with web publication",
			request: CreateListRequest{
				WorkspaceID:           "workspace123",
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				IsPublic:              true,
				Description:           "Test description",
				WebPublicationEnabled: true,
				WebPublicationSettings: &WebPublicationSettings{
					Slug:            "my-newsletter",
					MetaTitle:       "My Newsletter",
					MetaDescription: "Subscribe to my newsletter",
				},
			},
			wantErr: false,
			wantList: &List{
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				IsPublic:              true,
				Description:           "Test description",
				WebPublicationEnabled: true,
				WebPublicationSettings: &WebPublicationSettings{
					Slug:            "my-newsletter",
					MetaTitle:       "My Newsletter",
					MetaDescription: "Subscribe to my newsletter",
				},
			},
		},
		{
			name: "web publication enabled without slug",
			request: CreateListRequest{
				WorkspaceID:            "workspace123",
				ID:                     "list123",
				Name:                   "My List",
				IsDoubleOptin:          false,
				WebPublicationEnabled:  true,
				WebPublicationSettings: &WebPublicationSettings{},
			},
			wantErr: true,
		},
		{
			name: "web publication enabled with nil settings",
			request: CreateListRequest{
				WorkspaceID:           "workspace123",
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				WebPublicationEnabled: true,
			},
			wantErr: true,
		},
		{
			name: "web publication with reserved slug - console",
			request: CreateListRequest{
				WorkspaceID:           "workspace123",
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				WebPublicationEnabled: true,
				WebPublicationSettings: &WebPublicationSettings{
					Slug: "console",
				},
			},
			wantErr: true,
		},
		{
			name: "web publication with reserved slug - api",
			request: CreateListRequest{
				WorkspaceID:           "workspace123",
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				WebPublicationEnabled: true,
				WebPublicationSettings: &WebPublicationSettings{
					Slug: "api",
				},
			},
			wantErr: true,
		},
		{
			name: "web publication with reserved slug - notification-center",
			request: CreateListRequest{
				WorkspaceID:           "workspace123",
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				WebPublicationEnabled: true,
				WebPublicationSettings: &WebPublicationSettings{
					Slug: "notification-center",
				},
			},
			wantErr: true,
		},
		{
			name: "web publication with invalid slug format - uppercase",
			request: CreateListRequest{
				WorkspaceID:           "workspace123",
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				WebPublicationEnabled: true,
				WebPublicationSettings: &WebPublicationSettings{
					Slug: "MyNewsletter",
				},
			},
			wantErr: true,
		},
		{
			name: "web publication with invalid slug format - special chars",
			request: CreateListRequest{
				WorkspaceID:           "workspace123",
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				WebPublicationEnabled: true,
				WebPublicationSettings: &WebPublicationSettings{
					Slug: "my_newsletter",
				},
			},
			wantErr: true,
		},
		{
			name: "web publication with slug too long",
			request: CreateListRequest{
				WorkspaceID:           "workspace123",
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				WebPublicationEnabled: true,
				WebPublicationSettings: &WebPublicationSettings{
					Slug: "this-is-a-very-long-slug-that-exceeds-the-maximum-length-of-one-hundred-characters-and-should-fail-validation",
				},
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

				if tt.wantList.DoubleOptInTemplate != nil {
					assert.NotNil(t, list.DoubleOptInTemplate)
					assert.Equal(t, tt.wantList.DoubleOptInTemplate.ID, list.DoubleOptInTemplate.ID)
					assert.Equal(t, tt.wantList.DoubleOptInTemplate.Version, list.DoubleOptInTemplate.Version)
				} else {
					assert.Nil(t, list.DoubleOptInTemplate)
				}

				if tt.wantList.WelcomeTemplate != nil {
					assert.NotNil(t, list.WelcomeTemplate)
					assert.Equal(t, tt.wantList.WelcomeTemplate.ID, list.WelcomeTemplate.ID)
					assert.Equal(t, tt.wantList.WelcomeTemplate.Version, list.WelcomeTemplate.Version)
				} else {
					assert.Nil(t, list.WelcomeTemplate)
				}

				if tt.wantList.UnsubscribeTemplate != nil {
					assert.NotNil(t, list.UnsubscribeTemplate)
					assert.Equal(t, tt.wantList.UnsubscribeTemplate.ID, list.UnsubscribeTemplate.ID)
					assert.Equal(t, tt.wantList.UnsubscribeTemplate.Version, list.UnsubscribeTemplate.Version)
				} else {
					assert.Nil(t, list.UnsubscribeTemplate)
				}
			}
		})
	}
}

func TestGetListsRequest_FromURLParams(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string][]string
		wantErr bool
		want    GetListsRequest
	}{
		{
			name: "valid params",
			params: map[string][]string{
				"workspace_id": {"workspace123"},
			},
			wantErr: false,
			want: GetListsRequest{
				WorkspaceID: "workspace123",
			},
		},
		{
			name:    "missing workspace ID",
			params:  map[string][]string{},
			wantErr: true,
		},
		{
			name: "empty workspace ID",
			params: map[string][]string{
				"workspace_id": {""},
			},
			wantErr: true,
		},
		{
			name: "workspace ID too long",
			params: map[string][]string{
				"workspace_id": {"workspace12345678901234567890123456789"},
			},
			wantErr: true,
		},
		{
			name: "invalid workspace ID format",
			params: map[string][]string{
				"workspace_id": {"invalid@workspace"},
			},
			wantErr: false,
			want: GetListsRequest{
				WorkspaceID: "invalid@workspace",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &GetListsRequest{}
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
		want    GetListRequest
	}{
		{
			name: "valid params",
			params: map[string][]string{
				"workspace_id": {"workspace123"},
				"id":           {"list123"},
			},
			wantErr: false,
			want: GetListRequest{
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
			name: "empty workspace ID",
			params: map[string][]string{
				"workspace_id": {""},
				"id":           {"list123"},
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
			name: "empty list ID",
			params: map[string][]string{
				"workspace_id": {"workspace123"},
				"id":           {""},
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
			want: GetListRequest{
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
		{
			name: "list ID too long",
			params: map[string][]string{
				"workspace_id": {"workspace123"},
				"id":           {"list12345678901234567890123456789012345"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &GetListRequest{}
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
		request  UpdateListRequest
		wantErr  bool
		wantList *List
	}{
		{
			name: "valid request",
			request: UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "Test description",
				DoubleOptInTemplate: &TemplateReference{
					ID:      "template123",
					Version: 1,
				},
			},
			wantErr: false,
			wantList: &List{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "Test description",
				DoubleOptInTemplate: &TemplateReference{
					ID:      "template123",
					Version: 1,
				},
			},
		},
		{
			name: "valid request with all templates",
			request: UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "Test description",
				DoubleOptInTemplate: &TemplateReference{
					ID:      "template1",
					Version: 1,
				},
				WelcomeTemplate: &TemplateReference{
					ID:      "template2",
					Version: 1,
				},
				UnsubscribeTemplate: &TemplateReference{
					ID:      "template3",
					Version: 1,
				},
			},
			wantErr: false,
			wantList: &List{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				IsPublic:      true,
				Description:   "Test description",
				DoubleOptInTemplate: &TemplateReference{
					ID:      "template1",
					Version: 1,
				},
				WelcomeTemplate: &TemplateReference{
					ID:      "template2",
					Version: 1,
				},
				UnsubscribeTemplate: &TemplateReference{
					ID:      "template3",
					Version: 1,
				},
			},
		},
		{
			name: "valid request with no double opt-in",
			request: UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: false,
				IsPublic:      true,
				Description:   "Test description",
			},
			wantErr: false,
			wantList: &List{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: false,
				IsPublic:      true,
				Description:   "Test description",
			},
		},
		{
			name: "missing workspace ID",
			request: UpdateListRequest{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "missing ID",
			request: UpdateListRequest{
				WorkspaceID:   "workspace123",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "invalid ID format",
			request: UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "invalid@id",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "ID too long",
			request: UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list1234567890123456789012345678901234567890",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "missing name",
			request: UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "name too long",
			request: UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          string(make([]byte, 256)),
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "double opt-in without template",
			request: UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
			},
			wantErr: true,
		},
		{
			name: "invalid double opt-in template",
			request: UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				DoubleOptInTemplate: &TemplateReference{
					ID:      "",
					Version: 0,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid welcome template",
			request: UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: false,
				WelcomeTemplate: &TemplateReference{
					ID:      "",
					Version: 0,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid unsubscribe template",
			request: UpdateListRequest{
				WorkspaceID:   "workspace123",
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: false,
				UnsubscribeTemplate: &TemplateReference{
					ID:      "",
					Version: 0,
				},
			},
			wantErr: true,
		},
		{
			name: "valid request with web publication",
			request: UpdateListRequest{
				WorkspaceID:           "workspace123",
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				IsPublic:              true,
				Description:           "Test description",
				WebPublicationEnabled: true,
				WebPublicationSettings: &WebPublicationSettings{
					Slug:            "my-newsletter",
					MetaTitle:       "My Newsletter",
					MetaDescription: "Subscribe to my newsletter",
				},
			},
			wantErr: false,
			wantList: &List{
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				IsPublic:              true,
				Description:           "Test description",
				WebPublicationEnabled: true,
				WebPublicationSettings: &WebPublicationSettings{
					Slug:            "my-newsletter",
					MetaTitle:       "My Newsletter",
					MetaDescription: "Subscribe to my newsletter",
				},
			},
		},
		{
			name: "web publication enabled without slug",
			request: UpdateListRequest{
				WorkspaceID:            "workspace123",
				ID:                     "list123",
				Name:                   "My List",
				IsDoubleOptin:          false,
				WebPublicationEnabled:  true,
				WebPublicationSettings: &WebPublicationSettings{},
			},
			wantErr: true,
		},
		{
			name: "web publication enabled with nil settings",
			request: UpdateListRequest{
				WorkspaceID:           "workspace123",
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				WebPublicationEnabled: true,
			},
			wantErr: true,
		},
		{
			name: "web publication with reserved slug - console",
			request: UpdateListRequest{
				WorkspaceID:           "workspace123",
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				WebPublicationEnabled: true,
				WebPublicationSettings: &WebPublicationSettings{
					Slug: "console",
				},
			},
			wantErr: true,
		},
		{
			name: "web publication with reserved slug - api",
			request: UpdateListRequest{
				WorkspaceID:           "workspace123",
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				WebPublicationEnabled: true,
				WebPublicationSettings: &WebPublicationSettings{
					Slug: "api",
				},
			},
			wantErr: true,
		},
		{
			name: "web publication with reserved slug - preferences",
			request: UpdateListRequest{
				WorkspaceID:           "workspace123",
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				WebPublicationEnabled: true,
				WebPublicationSettings: &WebPublicationSettings{
					Slug: "preferences",
				},
			},
			wantErr: true,
		},
		{
			name: "web publication with invalid slug format - uppercase",
			request: UpdateListRequest{
				WorkspaceID:           "workspace123",
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				WebPublicationEnabled: true,
				WebPublicationSettings: &WebPublicationSettings{
					Slug: "MyNewsletter",
				},
			},
			wantErr: true,
		},
		{
			name: "web publication with invalid slug format - spaces",
			request: UpdateListRequest{
				WorkspaceID:           "workspace123",
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				WebPublicationEnabled: true,
				WebPublicationSettings: &WebPublicationSettings{
					Slug: "my newsletter",
				},
			},
			wantErr: true,
		},
		{
			name: "web publication with slug too long",
			request: UpdateListRequest{
				WorkspaceID:           "workspace123",
				ID:                    "list123",
				Name:                  "My List",
				IsDoubleOptin:         false,
				WebPublicationEnabled: true,
				WebPublicationSettings: &WebPublicationSettings{
					Slug: "this-is-a-very-long-slug-that-exceeds-the-maximum-length-of-one-hundred-characters-and-should-fail-validation",
				},
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

				if tt.wantList.DoubleOptInTemplate != nil {
					assert.NotNil(t, list.DoubleOptInTemplate)
					assert.Equal(t, tt.wantList.DoubleOptInTemplate.ID, list.DoubleOptInTemplate.ID)
					assert.Equal(t, tt.wantList.DoubleOptInTemplate.Version, list.DoubleOptInTemplate.Version)
				} else {
					assert.Nil(t, list.DoubleOptInTemplate)
				}

				if tt.wantList.WelcomeTemplate != nil {
					assert.NotNil(t, list.WelcomeTemplate)
					assert.Equal(t, tt.wantList.WelcomeTemplate.ID, list.WelcomeTemplate.ID)
					assert.Equal(t, tt.wantList.WelcomeTemplate.Version, list.WelcomeTemplate.Version)
				} else {
					assert.Nil(t, list.WelcomeTemplate)
				}

				if tt.wantList.UnsubscribeTemplate != nil {
					assert.NotNil(t, list.UnsubscribeTemplate)
					assert.Equal(t, tt.wantList.UnsubscribeTemplate.ID, list.UnsubscribeTemplate.ID)
					assert.Equal(t, tt.wantList.UnsubscribeTemplate.Version, list.UnsubscribeTemplate.Version)
				} else {
					assert.Nil(t, list.UnsubscribeTemplate)
				}
			}
		})
	}
}

func TestDeleteListRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request DeleteListRequest
		wantErr bool
		wantID  string
	}{
		{
			name: "valid request",
			request: DeleteListRequest{
				WorkspaceID: "workspace123",
				ID:          "list123",
			},
			wantErr: false,
			wantID:  "workspace123",
		},
		{
			name: "missing workspace ID",
			request: DeleteListRequest{
				ID: "list123",
			},
			wantErr: true,
			wantID:  "",
		},
		{
			name: "empty workspace ID",
			request: DeleteListRequest{
				WorkspaceID: "",
				ID:          "list123",
			},
			wantErr: true,
			wantID:  "",
		},
		{
			name: "missing list ID",
			request: DeleteListRequest{
				WorkspaceID: "workspace123",
			},
			wantErr: true,
			wantID:  "",
		},
		{
			name: "empty list ID",
			request: DeleteListRequest{
				WorkspaceID: "workspace123",
				ID:          "",
			},
			wantErr: true,
			wantID:  "",
		},
		{
			name: "invalid workspace ID format",
			request: DeleteListRequest{
				WorkspaceID: "invalid@workspace",
				ID:          "list123",
			},
			wantErr: false,
			wantID:  "invalid@workspace",
		},
		{
			name: "invalid list ID format",
			request: DeleteListRequest{
				WorkspaceID: "workspace123",
				ID:          "invalid@list",
			},
			wantErr: true,
			wantID:  "",
		},
		{
			name: "list ID too long",
			request: DeleteListRequest{
				WorkspaceID: "workspace123",
				ID:          "list12345678901234567890123456789012345",
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
