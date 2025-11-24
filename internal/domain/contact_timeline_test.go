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
