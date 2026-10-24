package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validAutomationForRequest builds the smallest Automation that passes
// Automation.Validate, so request-level tests fail only for request-level
// reasons. A regression in Automation.Validate itself is pinned by
// TestAutomation_Validate, not here.
func validAutomationForRequest() *Automation {
	return &Automation{
		ID:          "auto-1",
		WorkspaceID: "ws-1",
		Name:        "Welcome flow",
		Status:      AutomationStatusDraft,
		Trigger:     &TimelineTriggerConfig{EventKind: "contact.created", Frequency: TriggerFrequencyOnce},
	}
}

func TestCreateAutomationRequest_Validate(t *testing.T) {
	t.Run("missing workspace_id is rejected", func(t *testing.T) {
		err := (&CreateAutomationRequest{Automation: validAutomationForRequest()}).Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "workspace_id is required")
	})

	t.Run("missing automation is rejected", func(t *testing.T) {
		err := (&CreateAutomationRequest{WorkspaceID: "ws-1"}).Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "automation is required")
	})

	t.Run("automation validation errors propagate unchanged", func(t *testing.T) {
		bad := validAutomationForRequest()
		bad.Name = ""
		err := (&CreateAutomationRequest{WorkspaceID: "ws-1", Automation: bad}).Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "name is required")
	})

	t.Run("workspace_id is copied onto an automation that lacks one", func(t *testing.T) {
		auto := validAutomationForRequest()
		auto.WorkspaceID = ""
		req := &CreateAutomationRequest{WorkspaceID: "ws-9", Automation: auto}
		require.NoError(t, req.Validate())
		// The copy is the point of the request type: downstream code reads the
		// workspace from the automation, so a dropped assignment would file
		// the automation under no workspace at all.
		assert.Equal(t, "ws-9", auto.WorkspaceID)
	})

	t.Run("an explicit automation workspace_id is not overwritten", func(t *testing.T) {
		auto := validAutomationForRequest()
		req := &CreateAutomationRequest{WorkspaceID: "ws-other", Automation: auto}
		require.NoError(t, req.Validate())
		assert.Equal(t, "ws-1", auto.WorkspaceID)
	})
}

func TestUpdateAutomationRequest_Validate(t *testing.T) {
	t.Run("missing workspace_id is rejected", func(t *testing.T) {
		err := (&UpdateAutomationRequest{Automation: validAutomationForRequest()}).Validate()
		require.EqualError(t, err, "workspace_id is required")
	})

	t.Run("missing automation is rejected", func(t *testing.T) {
		err := (&UpdateAutomationRequest{WorkspaceID: "ws-1"}).Validate()
		require.EqualError(t, err, "automation is required")
	})

	t.Run("missing automation id is rejected before full validation", func(t *testing.T) {
		auto := validAutomationForRequest()
		auto.ID = ""
		err := (&UpdateAutomationRequest{WorkspaceID: "ws-1", Automation: auto}).Validate()
		require.EqualError(t, err, "automation id is required")
	})

	t.Run("valid update passes and fills the workspace_id", func(t *testing.T) {
		auto := validAutomationForRequest()
		auto.WorkspaceID = ""
		req := &UpdateAutomationRequest{WorkspaceID: "ws-7", Automation: auto}
		require.NoError(t, req.Validate())
		assert.Equal(t, "ws-7", auto.WorkspaceID)
	})
}

func TestGetAutomationRequest_FromURLParams(t *testing.T) {
	t.Run("both params are parsed", func(t *testing.T) {
		var req GetAutomationRequest
		require.NoError(t, req.FromURLParams(map[string][]string{
			"workspace_id":  {"ws-1"},
			"automation_id": {"auto-1"},
		}))
		assert.Equal(t, "ws-1", req.WorkspaceID)
		assert.Equal(t, "auto-1", req.AutomationID)
	})

	t.Run("missing workspace_id fails validation", func(t *testing.T) {
		var req GetAutomationRequest
		err := req.FromURLParams(map[string][]string{"automation_id": {"auto-1"}})
		require.EqualError(t, err, "workspace_id is required")
	})

	t.Run("missing automation_id fails validation", func(t *testing.T) {
		var req GetAutomationRequest
		err := req.FromURLParams(map[string][]string{"workspace_id": {"ws-1"}})
		require.EqualError(t, err, "automation_id is required")
	})

	t.Run("only the first value of a repeated param is used", func(t *testing.T) {
		var req GetAutomationRequest
		require.NoError(t, req.FromURLParams(map[string][]string{
			"workspace_id":  {"ws-first", "ws-second"},
			"automation_id": {"auto-1"},
		}))
		assert.Equal(t, "ws-first", req.WorkspaceID)
	})

	t.Run("empty params map fails instead of returning a zero request", func(t *testing.T) {
		var req GetAutomationRequest
		require.Error(t, req.FromURLParams(map[string][]string{}))
	})
}

func TestListAutomationsRequest_FromURLParams(t *testing.T) {
	t.Run("all params are parsed including repeated statuses", func(t *testing.T) {
		var req ListAutomationsRequest
		require.NoError(t, req.FromURLParams(map[string][]string{
			"workspace_id": {"ws-1"},
			"status":       {"draft", "live"},
			"list_id":      {"list-1"},
			"limit":        {"25"},
			"offset":       {"50"},
		}))
		assert.Equal(t, "ws-1", req.WorkspaceID)
		assert.Equal(t, []AutomationStatus{AutomationStatusDraft, AutomationStatusLive}, req.Status)
		assert.Equal(t, "list-1", req.ListID)
		assert.Equal(t, 25, req.Limit)
		assert.Equal(t, 50, req.Offset)
	})

	t.Run("missing workspace_id fails validation", func(t *testing.T) {
		var req ListAutomationsRequest
		err := req.FromURLParams(map[string][]string{"limit": {"10"}})
		require.EqualError(t, err, "workspace_id is required")
	})

	t.Run("a non-numeric limit degrades to zero, not an error", func(t *testing.T) {
		// The parser deliberately ignores Sscanf errors: a garbage limit must
		// not 400 the listing, it just falls back to the default page size.
		var req ListAutomationsRequest
		require.NoError(t, req.FromURLParams(map[string][]string{
			"workspace_id": {"ws-1"},
			"limit":        {"abc"},
			"offset":       {"1x"},
		}))
		assert.Equal(t, 0, req.Limit)
		assert.Equal(t, 1, req.Offset, "Sscanf parses the numeric prefix of offset")
	})

	t.Run("absent pagination stays zero", func(t *testing.T) {
		var req ListAutomationsRequest
		require.NoError(t, req.FromURLParams(map[string][]string{"workspace_id": {"ws-1"}}))
		assert.Equal(t, 0, req.Limit)
		assert.Equal(t, 0, req.Offset)
		assert.Empty(t, req.Status)
	})
}

func TestListAutomationsRequest_ToFilter(t *testing.T) {
	req := &ListAutomationsRequest{
		WorkspaceID: "ws-1",
		Status:      []AutomationStatus{AutomationStatusLive},
		ListID:      "list-1",
		Limit:       10,
		Offset:      20,
	}
	filter := req.ToFilter()
	// Field-by-field: a swapped Limit/Offset or a dropped Status would
	// silently page or filter wrong, so compare the whole struct.
	assert.Equal(t, AutomationFilter{
		Status: []AutomationStatus{AutomationStatusLive},
		ListID: "list-1",
		Limit:  10,
		Offset: 20,
	}, filter)
}

func TestAutomationIDOnlyRequests_Validate(t *testing.T) {
	cases := []struct {
		name     string
		validate func(workspaceID, automationID string) error
	}{
		{"delete", func(ws, id string) error {
			return (&DeleteAutomationRequest{WorkspaceID: ws, AutomationID: id}).Validate()
		}},
		{"activate", func(ws, id string) error {
			return (&ActivateAutomationRequest{WorkspaceID: ws, AutomationID: id}).Validate()
		}},
		{"pause", func(ws, id string) error {
			return (&PauseAutomationRequest{WorkspaceID: ws, AutomationID: id}).Validate()
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name+" requires workspace_id", func(t *testing.T) {
			require.EqualError(t, tc.validate("", "auto-1"), "workspace_id is required")
		})
		t.Run(tc.name+" requires automation_id", func(t *testing.T) {
			require.EqualError(t, tc.validate("ws-1", ""), "automation_id is required")
		})
		t.Run(tc.name+" accepts both ids", func(t *testing.T) {
			require.NoError(t, tc.validate("ws-1", "auto-1"))
		})
	}
}

func TestGetContactNodeExecutionsRequest_FromURLParams(t *testing.T) {
	t.Run("all params are parsed", func(t *testing.T) {
		var req GetContactNodeExecutionsRequest
		require.NoError(t, req.FromURLParams(map[string][]string{
			"workspace_id":  {"ws-1"},
			"automation_id": {"auto-1"},
			"email":         {"a@example.com"},
		}))
		assert.Equal(t, "ws-1", req.WorkspaceID)
		assert.Equal(t, "auto-1", req.AutomationID)
		assert.Equal(t, "a@example.com", req.Email)
	})

	t.Run("each missing field is reported in field order", func(t *testing.T) {
		var req GetContactNodeExecutionsRequest
		require.EqualError(t, req.FromURLParams(map[string][]string{}), "workspace_id is required")

		req = GetContactNodeExecutionsRequest{}
		require.EqualError(t, req.FromURLParams(map[string][]string{
			"workspace_id": {"ws-1"},
		}), "automation_id is required")

		req = GetContactNodeExecutionsRequest{}
		require.EqualError(t, req.FromURLParams(map[string][]string{
			"workspace_id":  {"ws-1"},
			"automation_id": {"auto-1"},
		}), "email is required")
	})
}

func TestWebhookNodeConfig_Validate(t *testing.T) {
	assert.EqualError(t, WebhookNodeConfig{}.Validate(), "url is required")
	assert.EqualError(t, WebhookNodeConfig{URL: "ftp://example.com/hook"}.Validate(),
		"url must start with http:// or https://")
	assert.EqualError(t, WebhookNodeConfig{URL: "example.com/hook"}.Validate(),
		"url must start with http:// or https://")
	require.NoError(t, WebhookNodeConfig{URL: "http://example.com/hook"}.Validate())
	require.NoError(t, WebhookNodeConfig{URL: "https://example.com/hook"}.Validate())
}

func TestAutomation_Validate_Boundaries(t *testing.T) {
	t.Run("id at exactly 36 chars passes, 37 fails", func(t *testing.T) {
		auto := validAutomationForRequest()
		auto.ID = strings.Repeat("a", 36)
		require.NoError(t, auto.Validate())

		auto.ID = strings.Repeat("a", 37)
		require.EqualError(t, auto.Validate(), "id cannot exceed 36 characters")
	})

	t.Run("name at exactly 255 chars passes, 256 fails", func(t *testing.T) {
		auto := validAutomationForRequest()
		auto.Name = strings.Repeat("n", 255)
		require.NoError(t, auto.Validate())

		auto.Name = strings.Repeat("n", 256)
		require.EqualError(t, auto.Validate(), "name cannot exceed 255 characters")
	})

	t.Run("nodes present without a root node id fails", func(t *testing.T) {
		auto := validAutomationForRequest()
		auto.Nodes = []*AutomationNode{
			{ID: "n1", AutomationID: auto.ID, Type: NodeTypeTrigger, Config: map[string]interface{}{}},
		}
		require.EqualError(t, auto.Validate(), "root_node_id is required when nodes are present")
	})

	t.Run("root node id pointing at a missing node fails and names the id", func(t *testing.T) {
		auto := validAutomationForRequest()
		auto.Nodes = []*AutomationNode{
			{ID: "n1", AutomationID: auto.ID, Type: NodeTypeTrigger, Config: map[string]interface{}{}},
		}
		auto.RootNodeID = "ghost"
		require.EqualError(t, auto.Validate(), "root_node_id ghost does not reference a valid node")
	})

	t.Run("a nil node is reported with its index", func(t *testing.T) {
		auto := validAutomationForRequest()
		auto.Nodes = []*AutomationNode{
			{ID: "n1", AutomationID: auto.ID, Type: NodeTypeTrigger, Config: map[string]interface{}{}},
			nil,
		}
		auto.RootNodeID = "n1"
		require.EqualError(t, auto.Validate(), "node at index 1 is nil")
	})
}
