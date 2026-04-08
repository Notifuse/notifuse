package domain_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestContactList_Validate(t *testing.T) {
	tests := []struct {
		name        string
		contactList domain.ContactList
		wantErr     bool
	}{
		{
			name: "valid contact list",
			contactList: domain.ContactList{
				Email:  "test@example.com",
				ListID: "list123",
				Status: domain.ContactListStatusActive,
			},
			wantErr: false,
		},
		{
			name: "valid contact list with pending status",
			contactList: domain.ContactList{
				Email:  "test@example.com",
				ListID: "list123",
				Status: domain.ContactListStatusPending,
			},
			wantErr: false,
		},
		{
			name: "missing contact ID",
			contactList: domain.ContactList{
				ListID: "list123",
				Status: domain.ContactListStatusActive,
			},
			wantErr: true,
		},
		{
			name: "invalid contact ID format",
			contactList: domain.ContactList{
				Email:  "not-an-email",
				ListID: "list123",
				Status: domain.ContactListStatusActive,
			},
			wantErr: true,
		},
		{
			name: "missing list ID",
			contactList: domain.ContactList{
				Email:  "test@example.com",
				Status: domain.ContactListStatusActive,
			},
			wantErr: true,
		},
		{
			name: "invalid list ID format",
			contactList: domain.ContactList{
				Email:  "test@example.com",
				ListID: "invalid@list&id",
				Status: domain.ContactListStatusActive,
			},
			wantErr: false,
		},
		{
			name: "missing status",
			contactList: domain.ContactList{
				Email:  "test@example.com",
				ListID: "list123",
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			contactList: domain.ContactList{
				Email:  "test@example.com",
				ListID: "list123",
				Status: "invalid-status",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.contactList.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestScanContactList(t *testing.T) {
	now := time.Now()

	// Test cases for different status values
	statuses := []string{
		string(domain.ContactListStatusActive),
		string(domain.ContactListStatusPending),
		string(domain.ContactListStatusUnsubscribed),
		string(domain.ContactListStatusBounced),
		string(domain.ContactListStatusComplained),
	}

	for _, status := range statuses {
		t.Run("scan with "+status+" status", func(t *testing.T) {
			// Create mock scanner
			scanner := &contactListMockScanner{
				data: []interface{}{
					"test@example.com", // Email
					"list123",          // ListID
					status,             // Status
					now,                // CreatedAt
					now,                // UpdatedAt
				},
			}

			// Test successful scan
			contactList, err := domain.ScanContactList(scanner)
			assert.NoError(t, err)
			assert.Equal(t, "test@example.com", contactList.Email)
			assert.Equal(t, "list123", contactList.ListID)
			assert.Equal(t, domain.ContactListStatus(status), contactList.Status)
			assert.Equal(t, now, contactList.CreatedAt)
			assert.Equal(t, now, contactList.UpdatedAt)
		})
	}

	// Test scan error
	t.Run("scan error", func(t *testing.T) {
		scanner := &contactListMockScanner{
			err: sql.ErrNoRows,
		}
		_, err := domain.ScanContactList(scanner)
		assert.Error(t, err)
	})
}

// ContactListStatus constants test
func TestContactListStatusConstants(t *testing.T) {
	assert.Equal(t, domain.ContactListStatus("active"), domain.ContactListStatusActive)
	assert.Equal(t, domain.ContactListStatus("pending"), domain.ContactListStatusPending)
	assert.Equal(t, domain.ContactListStatus("unsubscribed"), domain.ContactListStatusUnsubscribed)
	assert.Equal(t, domain.ContactListStatus("bounced"), domain.ContactListStatusBounced)
	assert.Equal(t, domain.ContactListStatus("complained"), domain.ContactListStatusComplained)
}

func TestIsTerminalContactListStatus(t *testing.T) {
	assert.True(t, domain.IsTerminalContactListStatus(domain.ContactListStatusBounced))
	assert.True(t, domain.IsTerminalContactListStatus(domain.ContactListStatusComplained))
	assert.False(t, domain.IsTerminalContactListStatus(domain.ContactListStatusActive))
	assert.False(t, domain.IsTerminalContactListStatus(domain.ContactListStatusPending))
	assert.False(t, domain.IsTerminalContactListStatus(domain.ContactListStatusUnsubscribed))
}

// Mock scanner for testing
type contactListMockScanner struct {
	data []interface{}
	err  error
}

func (m *contactListMockScanner) Scan(dest ...interface{}) error {
	if m.err != nil {
		return m.err
	}

	for i, d := range dest {
		switch v := d.(type) {
		case *string:
			if s, ok := m.data[i].(string); ok {
				*v = s
			}
		case *time.Time:
			if t, ok := m.data[i].(time.Time); ok {
				*v = t
			}
		}
	}

	return nil
}

func TestErrContactListNotFound_Error(t *testing.T) {
	err := &domain.ErrContactListNotFound{Message: "test error message"}
	assert.Equal(t, "test error message", err.Error())
}

func TestSubscribeToListsRequest_Validate(t *testing.T) {
	tests := []struct {
		name        string
		request     domain.SubscribeToListsRequest
		wantErr     bool
		wantContact *domain.ContactList
	}{
		{
			name: "valid request",
			request: domain.SubscribeToListsRequest{
				WorkspaceID: "workspace123",
				Contact: domain.Contact{
					Email: "test@example.com",
				},
				ListIDs: []string{"list123"},
			},
			wantErr: false,
			wantContact: &domain.ContactList{
				Email:  "test@example.com",
				ListID: "list123",
				Status: domain.ContactListStatusActive,
			},
		},
		{
			name: "missing workspace ID",
			request: domain.SubscribeToListsRequest{
				Contact: domain.Contact{
					Email: "test@example.com",
				},
				ListIDs: []string{"list123"},
			},
			wantErr: true,
		},
		{
			name: "missing email",
			request: domain.SubscribeToListsRequest{
				WorkspaceID: "workspace123",
				ListIDs:     []string{"list123"},
			},
			wantErr: true,
		},
		{
			name: "invalid email",
			request: domain.SubscribeToListsRequest{
				WorkspaceID: "workspace123",
				Contact: domain.Contact{
					Email: "not-an-email",
				},
				ListIDs: []string{"list123"},
			},
			wantErr: true,
		},
		{
			name: "missing list IDs",
			request: domain.SubscribeToListsRequest{
				WorkspaceID: "workspace123",
				Contact: domain.Contact{
					Email: "test@example.com",
				},
			},
			wantErr: true,
		},
		{
			name: "empty list IDs",
			request: domain.SubscribeToListsRequest{
				WorkspaceID: "workspace123",
				Contact: domain.Contact{
					Email: "test@example.com",
				},
				ListIDs: []string{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetContactListRequest_FromURLParams(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string][]string
		wantErr bool
		want    domain.GetContactListRequest
	}{
		{
			name: "valid params",
			params: map[string][]string{
				"workspace_id": {"workspace123"},
				"email":        {"test@example.com"},
				"list_id":      {"list123"},
			},
			wantErr: false,
			want: domain.GetContactListRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
				ListID:      "list123",
			},
		},
		{
			name: "missing workspace ID",
			params: map[string][]string{
				"email":   {"test@example.com"},
				"list_id": {"list123"},
			},
			wantErr: true,
		},
		{
			name: "missing email",
			params: map[string][]string{
				"workspace_id": {"workspace123"},
				"list_id":      {"list123"},
			},
			wantErr: true,
		},
		{
			name: "invalid email",
			params: map[string][]string{
				"workspace_id": {"workspace123"},
				"email":        {"not-an-email"},
				"list_id":      {"list123"},
			},
			wantErr: true,
		},
		{
			name: "missing list ID",
			params: map[string][]string{
				"workspace_id": {"workspace123"},
				"email":        {"test@example.com"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &domain.GetContactListRequest{}
			err := req.FromURLParams(tt.params)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want.WorkspaceID, req.WorkspaceID)
				assert.Equal(t, tt.want.Email, req.Email)
				assert.Equal(t, tt.want.ListID, req.ListID)
			}
		})
	}
}

func TestGetContactsByListRequest_FromURLParams(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string][]string
		wantErr bool
		want    domain.GetContactsByListRequest
	}{
		{
			name: "valid params",
			params: map[string][]string{
				"workspace_id": {"workspace123"},
				"list_id":      {"list123"},
			},
			wantErr: false,
			want: domain.GetContactsByListRequest{
				WorkspaceID: "workspace123",
				ListID:      "list123",
			},
		},
		{
			name: "missing workspace ID",
			params: map[string][]string{
				"list_id": {"list123"},
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
			name: "invalid list ID format",
			params: map[string][]string{
				"workspace_id": {"workspace123"},
				"list_id":      {"invalid@list"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &domain.GetContactsByListRequest{}
			err := req.FromURLParams(tt.params)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want.WorkspaceID, req.WorkspaceID)
				assert.Equal(t, tt.want.ListID, req.ListID)
			}
		})
	}
}

func TestGetListsByContactRequest_FromURLParams(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string][]string
		wantErr bool
		want    domain.GetListsByContactRequest
	}{
		{
			name: "valid params",
			params: map[string][]string{
				"workspace_id": {"workspace123"},
				"email":        {"test@example.com"},
			},
			wantErr: false,
			want: domain.GetListsByContactRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
			},
		},
		{
			name: "missing workspace ID",
			params: map[string][]string{
				"email": {"test@example.com"},
			},
			wantErr: true,
		},
		{
			name: "missing email",
			params: map[string][]string{
				"workspace_id": {"workspace123"},
			},
			wantErr: true,
		},
		{
			name: "invalid email",
			params: map[string][]string{
				"workspace_id": {"workspace123"},
				"email":        {"not-an-email"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &domain.GetListsByContactRequest{}
			err := req.FromURLParams(tt.params)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want.WorkspaceID, req.WorkspaceID)
				assert.Equal(t, tt.want.Email, req.Email)
			}
		})
	}
}

func TestUpdateContactListStatusRequest_Validate(t *testing.T) {
	tests := []struct {
		name        string
		request     domain.UpdateContactListStatusRequest
		wantErr     bool
		wantContact *domain.ContactList
	}{
		{
			name: "valid request",
			request: domain.UpdateContactListStatusRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
				ListID:      "list123",
				Status:      "active",
			},
			wantErr: false,
			wantContact: &domain.ContactList{
				Email:  "test@example.com",
				ListID: "list123",
				Status: domain.ContactListStatusActive,
			},
		},
		{
			name: "missing workspace ID",
			request: domain.UpdateContactListStatusRequest{
				Email:  "test@example.com",
				ListID: "list123",
				Status: "active",
			},
			wantErr: true,
		},
		{
			name: "missing email",
			request: domain.UpdateContactListStatusRequest{
				WorkspaceID: "workspace123",
				ListID:      "list123",
				Status:      "active",
			},
			wantErr: true,
		},
		{
			name: "invalid email",
			request: domain.UpdateContactListStatusRequest{
				WorkspaceID: "workspace123",
				Email:       "not-an-email",
				ListID:      "list123",
				Status:      "active",
			},
			wantErr: true,
		},
		{
			name: "missing list ID",
			request: domain.UpdateContactListStatusRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
				Status:      "active",
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			request: domain.UpdateContactListStatusRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
				ListID:      "list123",
				Status:      "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspaceID, contact, err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, workspaceID)
				assert.Nil(t, contact)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.request.WorkspaceID, workspaceID)
				assert.Equal(t, tt.wantContact.Email, contact.Email)
				assert.Equal(t, tt.wantContact.ListID, contact.ListID)
				assert.Equal(t, tt.wantContact.Status, contact.Status)
			}
		})
	}
}

func TestRemoveContactFromListRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request domain.RemoveContactFromListRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: domain.RemoveContactFromListRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
				ListID:      "list123",
			},
			wantErr: false,
		},
		{
			name: "missing workspace ID",
			request: domain.RemoveContactFromListRequest{
				Email:  "test@example.com",
				ListID: "list123",
			},
			wantErr: true,
		},
		{
			name: "missing email",
			request: domain.RemoveContactFromListRequest{
				WorkspaceID: "workspace123",
				ListID:      "list123",
			},
			wantErr: true,
		},
		{
			name: "invalid email",
			request: domain.RemoveContactFromListRequest{
				WorkspaceID: "workspace123",
				Email:       "not-an-email",
				ListID:      "list123",
			},
			wantErr: true,
		},
		{
			name: "missing list ID",
			request: domain.RemoveContactFromListRequest{
				WorkspaceID: "workspace123",
				Email:       "test@example.com",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
