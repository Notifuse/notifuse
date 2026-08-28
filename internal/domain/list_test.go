package domain

import (
	"database/sql"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			name: "valid with double opt-in template",
			list: List{
				ID:            "list123",
				Name:          "My List",
				IsDoubleOptin: true,
				DoubleOptInTemplate: &TemplateReference{
					ID:      "template1",
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

func TestUnsubscribeFromListsRequest_FromOneClickURLParams(t *testing.T) {
	// The keys here mirror exactly what BuildTemplateData writes into the one-click
	// List-Unsubscribe URL: wid, email, email_hmac, lids, mid.
	t.Run("parses all params from the query string", func(t *testing.T) {
		vals := url.Values{}
		vals.Set("wid", "ws123")
		vals.Set("email", "user@example.com")
		vals.Set("email_hmac", "deadbeef")
		vals.Set("lids", "list1")
		vals.Set("mid", "msg-1")

		var req UnsubscribeFromListsRequest
		err := req.FromOneClickURLParams(vals)
		assert.NoError(t, err)
		assert.Equal(t, "ws123", req.WorkspaceID)
		assert.Equal(t, "user@example.com", req.Email)
		assert.Equal(t, "deadbeef", req.EmailHMAC)
		assert.Equal(t, []string{"list1"}, req.ListIDs)
		assert.Equal(t, "msg-1", req.MessageID)
	})

	t.Run("supports multiple lids", func(t *testing.T) {
		vals := url.Values{}
		vals.Set("wid", "ws123")
		vals.Set("email", "user@example.com")
		vals["lids"] = []string{"list1", "list2"}

		var req UnsubscribeFromListsRequest
		err := req.FromOneClickURLParams(vals)
		assert.NoError(t, err)
		assert.Equal(t, []string{"list1", "list2"}, req.ListIDs)
	})

	t.Run("email_hmac and mid are optional at parse time", func(t *testing.T) {
		// Authentication (email_hmac) is verified by the service, and mid is only used
		// for analytics; the parser only requires enough to identify contact + lists.
		vals := url.Values{}
		vals.Set("wid", "ws123")
		vals.Set("email", "user@example.com")
		vals.Set("lids", "list1")

		var req UnsubscribeFromListsRequest
		err := req.FromOneClickURLParams(vals)
		assert.NoError(t, err)
		assert.Empty(t, req.EmailHMAC)
		assert.Empty(t, req.MessageID)
	})

	t.Run("does not accept the legacy snake_case keys", func(t *testing.T) {
		// Regression guard: the legacy FromURLParams reads workspace_id/list_ids, which
		// silently produced an empty request when fed a one-click URL. The one-click
		// parser must reject those keys.
		vals := url.Values{}
		vals.Set("workspace_id", "ws123")
		vals.Set("email", "user@example.com")
		vals["list_ids"] = []string{"list1"}

		var req UnsubscribeFromListsRequest
		assert.Error(t, req.FromOneClickURLParams(vals))
	})

	errorCases := []struct {
		name string
		vals url.Values
	}{
		{"missing wid", func() url.Values {
			v := url.Values{}
			v.Set("email", "user@example.com")
			v.Set("lids", "list1")
			return v
		}()},
		{"missing email", func() url.Values {
			v := url.Values{}
			v.Set("wid", "ws123")
			v.Set("lids", "list1")
			return v
		}()},
		{"missing lids", func() url.Values {
			v := url.Values{}
			v.Set("wid", "ws123")
			v.Set("email", "user@example.com")
			return v
		}()},
		{"empty query", url.Values{}},
	}
	for _, tc := range errorCases {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			var req UnsubscribeFromListsRequest
			assert.Error(t, req.FromOneClickURLParams(tc.vals))
		})
	}
}

func TestUnsubscribeFromListsRequest_FromJSONBody(t *testing.T) {
	// The struct's json tags map the SPA body keys: wid, email, email_hmac, lids, mid.
	t.Run("parses all fields from the JSON body", func(t *testing.T) {
		body := `{"wid":"ws123","email":"user@example.com","email_hmac":"deadbeef","lids":["list1","list2"],"mid":"msg-1"}`
		var req UnsubscribeFromListsRequest
		err := req.FromJSONBody(strings.NewReader(body))
		assert.NoError(t, err)
		assert.Equal(t, "ws123", req.WorkspaceID)
		assert.Equal(t, "user@example.com", req.Email)
		assert.Equal(t, "deadbeef", req.EmailHMAC)
		assert.Equal(t, []string{"list1", "list2"}, req.ListIDs)
		assert.Equal(t, "msg-1", req.MessageID)
	})

	t.Run("email_hmac and mid are optional at parse time", func(t *testing.T) {
		// Authentication (email_hmac) is verified by the service, and mid is only used
		// for analytics; the parser only requires enough to identify contact + lists.
		body := `{"wid":"ws123","email":"user@example.com","lids":["list1"]}`
		var req UnsubscribeFromListsRequest
		err := req.FromJSONBody(strings.NewReader(body))
		assert.NoError(t, err)
		assert.Empty(t, req.EmailHMAC)
		assert.Empty(t, req.MessageID)
	})

	t.Run("ignores unknown fields", func(t *testing.T) {
		body := `{"wid":"ws123","email":"user@example.com","lids":["list1"],"foo":"bar"}`
		var req UnsubscribeFromListsRequest
		assert.NoError(t, req.FromJSONBody(strings.NewReader(body)))
	})

	errorCases := []struct {
		name string
		body string
	}{
		{"missing wid", `{"email":"user@example.com","lids":["list1"]}`},
		{"missing email", `{"wid":"ws123","lids":["list1"]}`},
		{"missing lids", `{"wid":"ws123","email":"user@example.com"}`},
		{"empty lids array", `{"wid":"ws123","email":"user@example.com","lids":[]}`},
		{"empty object", `{}`},
		{"malformed json", `{"wid":`},
		{"empty body", ``},
	}
	for _, tc := range errorCases {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			var req UnsubscribeFromListsRequest
			assert.Error(t, req.FromJSONBody(strings.NewReader(tc.body)))
		})
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

// lists.update rewrites every column of the row, so a flag the body left out is
// stored as false. These cases go through json.Unmarshal rather than a struct
// literal on purpose: a Go literal cannot express a missing key, which is exactly
// how the field's absence became a silent "off".
func TestUpdateListRequest_OmittedFlagsAreRefusedNotStoredAsFalse(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "is_double_optin omitted",
			body:    `{"workspace_id":"workspace123","id":"list123","name":"My List","is_public":true}`,
			wantErr: "is_double_optin is required",
		},
		{
			name:    "is_public omitted",
			body:    `{"workspace_id":"workspace123","id":"list123","name":"My List","is_double_optin":false}`,
			wantErr: "is_public is required",
		},
		{
			name:    "both flags omitted",
			body:    `{"workspace_id":"workspace123","id":"list123","name":"My List"}`,
			wantErr: "is_double_optin is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req UpdateListRequest
			require.NoError(t, json.Unmarshal([]byte(tt.body), &req))

			list, workspaceID, err := req.Validate()

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, list)
			assert.Empty(t, workspaceID)
		})
	}
}

// A null is a serializer writing an absent optional out, not a caller asking for false:
// there is no bool a null could have meant. Reading it as present let a client that
// serializes absent fields as null drop a list from the public subscription pages, or
// activate subscribers without a confirmation email, without ever being told — the very
// silence the refusal below exists to break. Every other decoder in this package folds
// null into "omitted"; this one now does too.
func TestUpdateListRequest_NullFlagsAreReadAsOmitted(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "is_double_optin null",
			body:    `{"workspace_id":"workspace123","id":"list123","name":"My List","is_double_optin":null,"is_public":true}`,
			wantErr: "is_double_optin is required",
		},
		{
			name:    "is_public null",
			body:    `{"workspace_id":"workspace123","id":"list123","name":"My List","is_double_optin":false,"is_public":null}`,
			wantErr: "is_public is required",
		},
		{
			name:    "both flags null",
			body:    `{"workspace_id":"workspace123","id":"list123","name":"My List","is_double_optin":null,"is_public":null}`,
			wantErr: "is_double_optin is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req UpdateListRequest
			require.NoError(t, json.Unmarshal([]byte(tt.body), &req))

			list, workspaceID, err := req.Validate()

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, list)
			assert.Empty(t, workspaceID)
		})
	}
}

// Refusing an omitted flag must not cost the caller the ability to turn one off:
// an explicit false is a legitimate instruction and still lands.
func TestUpdateListRequest_ExplicitFalseFlagsStillApply(t *testing.T) {
	body := `{"workspace_id":"workspace123","id":"list123","name":"My List","is_double_optin":false,"is_public":false}`

	var req UpdateListRequest
	require.NoError(t, json.Unmarshal([]byte(body), &req))

	list, workspaceID, err := req.Validate()

	require.NoError(t, err)
	assert.Equal(t, "workspace123", workspaceID)
	assert.False(t, list.IsDoubleOptin)
	assert.False(t, list.IsPublic)
}

// The presence record is written by the decoder, so its zero value has to mean
// "present": a request assembled in Go has no body to read presence from, and a
// caller filling every field in by hand has stated all of them.
func TestUpdateListRequest_StructLiteralNeedsNoPresenceRecord(t *testing.T) {
	req := UpdateListRequest{
		WorkspaceID: "workspace123",
		ID:          "list123",
		Name:        "My List",
	}

	list, _, err := req.Validate()

	require.NoError(t, err)
	assert.False(t, list.IsDoubleOptin)
	assert.False(t, list.IsPublic)
}
