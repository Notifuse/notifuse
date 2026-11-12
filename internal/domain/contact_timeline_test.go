package domain

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimelineListRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &TimelineListRequest{
			WorkspaceID: "workspace123",
			Email:       "test@example.com",
			Limit:       10,
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing workspace_id", func(t *testing.T) {
		req := &TimelineListRequest{
			Email: "test@example.com",
			Limit: 10,
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id is required")
	})

	t.Run("missing email", func(t *testing.T) {
		req := &TimelineListRequest{
			WorkspaceID: "workspace123",
			Limit:       10,
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "email is required")
	})

	t.Run("limit <0", func(t *testing.T) {
		req := &TimelineListRequest{
			WorkspaceID: "workspace123",
			Email:       "test@example.com",
			Limit:       -1,
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "limit must be non-negative")
	})

	t.Run("limit =0", func(t *testing.T) {
		req := &TimelineListRequest{
			WorkspaceID: "workspace123",
			Email:       "test@example.com",
			Limit:       0,
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("limit =100", func(t *testing.T) {
		req := &TimelineListRequest{
			WorkspaceID: "workspace123",
			Email:       "test@example.com",
			Limit:       100,
		}

		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("limit >100", func(t *testing.T) {
		req := &TimelineListRequest{
			WorkspaceID: "workspace123",
			Email:       "test@example.com",
			Limit:       101,
		}

		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "limit cannot exceed 100")
	})
}

func TestTimelineListRequest_FromQuery(t *testing.T) {
	t.Run("valid query with all params", func(t *testing.T) {
		query := url.Values{
			"workspace_id": {"workspace123"},
			"email":        {"test@example.com"},
			"limit":        {"25"},
			"cursor":       {"cursor123"},
		}

		req := &TimelineListRequest{}
		err := req.FromQuery(query)

		require.NoError(t, err)
		assert.Equal(t, "workspace123", req.WorkspaceID)
		assert.Equal(t, "test@example.com", req.Email)
		assert.Equal(t, 25, req.Limit)
		require.NotNil(t, req.Cursor)
		assert.Equal(t, "cursor123", *req.Cursor)
	})

	t.Run("valid query with defaults", func(t *testing.T) {
		query := url.Values{
			"workspace_id": {"workspace123"},
			"email":        {"test@example.com"},
		}

		req := &TimelineListRequest{}
		err := req.FromQuery(query)

		require.NoError(t, err)
		assert.Equal(t, "workspace123", req.WorkspaceID)
		assert.Equal(t, "test@example.com", req.Email)
		assert.Equal(t, 50, req.Limit) // Default limit
		assert.Nil(t, req.Cursor)
	})

	t.Run("invalid limit", func(t *testing.T) {
		query := url.Values{
			"workspace_id": {"workspace123"},
			"email":        {"test@example.com"},
			"limit":        {"not-a-number"},
		}

		req := &TimelineListRequest{}
		err := req.FromQuery(query)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid limit parameter: must be an integer")
	})

	t.Run("cursor present", func(t *testing.T) {
		query := url.Values{
			"workspace_id": {"workspace123"},
			"email":        {"test@example.com"},
			"cursor":       {"cursor123"},
		}

		req := &TimelineListRequest{}
		err := req.FromQuery(query)

		require.NoError(t, err)
		require.NotNil(t, req.Cursor)
		assert.Equal(t, "cursor123", *req.Cursor)
	})

	t.Run("missing workspace_id", func(t *testing.T) {
		query := url.Values{
			"email": {"test@example.com"},
		}

		req := &TimelineListRequest{}
		err := req.FromQuery(query)

		require.Error(t, err) // Error from Validate()
		assert.Contains(t, err.Error(), "workspace_id is required")
	})

	t.Run("missing email", func(t *testing.T) {
		query := url.Values{
			"workspace_id": {"workspace123"},
		}

		req := &TimelineListRequest{}
		err := req.FromQuery(query)

		require.Error(t, err) // Error from Validate()
		assert.Contains(t, err.Error(), "email is required")
	})
}

func TestTimelineListRequest_EntityTypes(t *testing.T) {
	parse := func(entityTypes string) (*TimelineListRequest, error) {
		req := &TimelineListRequest{}
		err := req.FromQuery(url.Values{"workspace_id": {"ws1"}, "email": {"a@example.com"}, "entity_types": {entityTypes}})
		return req, err
	}

	t.Run("comma-separated values, trimmed, in order", func(t *testing.T) {
		req, err := parse(" message_history, contact ")
		require.NoError(t, err)
		assert.Equal(t, []string{"message_history", "contact"}, req.EntityTypes)
	})

	t.Run("duplicates removed keeping the first position", func(t *testing.T) {
		req, err := parse("contact,web_page,contact")
		require.NoError(t, err)
		assert.Equal(t, []string{"contact", "web_page"}, req.EntityTypes)
	})

	t.Run("blank items dropped, all blank means no filter", func(t *testing.T) {
		req, err := parse("contact,, ,")
		require.NoError(t, err)
		assert.Equal(t, []string{"contact"}, req.EntityTypes)

		req, err = parse(" , ")
		require.NoError(t, err)
		assert.Empty(t, req.EntityTypes)
	})

	t.Run("absent means no filter", func(t *testing.T) {
		req := &TimelineListRequest{}
		require.NoError(t, req.FromQuery(url.Values{"workspace_id": {"ws1"}, "email": {"a@example.com"}}))
		assert.Empty(t, req.EntityTypes)
	})

	t.Run("an unknown value is refused", func(t *testing.T) {
		_, err := parse("contact,bogus")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bogus")
	})

	t.Run("every known type is accepted", func(t *testing.T) {
		for _, typ := range []string{"contact", "contact_list", "contact_segment", "message_history", "custom_event",
			"inbound_webhook_event", "webhook_event", "automation", "web_session", "web_page"} {
			_, err := parse(typ)
			assert.NoError(t, err, typ)
		}
	})
}
