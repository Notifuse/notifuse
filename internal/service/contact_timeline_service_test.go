package service

import (
	"context"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContactTimelineService_List(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockContactTimelineRepository(ctrl)
	service := NewContactTimelineService(mockRepo)

	ctx := context.Background()
	workspaceID := "ws123"
	email := "user@example.com"
	limit := 50

	t.Run("Success - List timeline entries", func(t *testing.T) {
		cursor := "cursor123"
		now := time.Now()
		expectedEntries := []*domain.ContactTimelineEntry{
			{
				ID:          "entry1",
				Email:       email,
				Operation:   "insert",
				EntityType:  "contact",
				Kind:        "insert_contact",
				Changes:     nil,
				CreatedAt:   now,
				DBCreatedAt: now,
			},
			{
				ID:         "entry2",
				Email:      email,
				Operation:  "update",
				EntityType: "contact",
				Kind:       "update_contact",
				Changes: map[string]interface{}{
					"first_name": map[string]interface{}{
						"old": "John",
						"new": "Jane",
					},
				},
				CreatedAt:   now.Add(-1 * time.Hour),
				DBCreatedAt: now.Add(-1 * time.Hour),
			},
		}

		mockRepo.EXPECT().
			List(ctx, workspaceID, email, limit, (*string)(nil)).
			Return(expectedEntries, &cursor, nil)

		entries, nextCursor, err := service.List(ctx, workspaceID, email, limit, nil)

		require.NoError(t, err)
		assert.Len(t, entries, 2)
		assert.NotNil(t, nextCursor)
		assert.Equal(t, "cursor123", *nextCursor)
		assert.Equal(t, "entry1", entries[0].ID)
		assert.Equal(t, "insert", entries[0].Operation)
		assert.Equal(t, "entry2", entries[1].ID)
		assert.Equal(t, "update", entries[1].Operation)
	})

	t.Run("Success - List with cursor", func(t *testing.T) {
		cursor := "existing_cursor"
		listID := "list123"
		expectedEntries := []*domain.ContactTimelineEntry{
			{
				ID:         "entry3",
				Email:      email,
				Operation:  "update",
				EntityType: "contact_list",
				EntityID:   &listID,
				Changes: map[string]interface{}{
					"status": map[string]interface{}{
						"old": "pending",
						"new": "active",
					},
				},
				CreatedAt: time.Now().Add(-2 * time.Hour),
			},
		}

		mockRepo.EXPECT().
			List(ctx, workspaceID, email, limit, &cursor).
			Return(expectedEntries, nil, nil)

		entries, _, err := service.List(ctx, workspaceID, email, limit, &cursor)

		require.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.Equal(t, "entry3", entries[0].ID)
		assert.Equal(t, "contact_list", entries[0].EntityType)
		assert.NotNil(t, entries[0].EntityID)
		assert.Equal(t, "list123", *entries[0].EntityID)
	})

	t.Run("Success - Empty result", func(t *testing.T) {
		expectedEntries := []*domain.ContactTimelineEntry{}

		mockRepo.EXPECT().
			List(ctx, workspaceID, email, limit, (*string)(nil)).
			Return(expectedEntries, nil, nil)

		entries, _, err := service.List(ctx, workspaceID, email, limit, nil)

		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("Success - Message history entry", func(t *testing.T) {
		messageID := "msg123"
		expectedEntries := []*domain.ContactTimelineEntry{
			{
				ID:         "entry4",
				Email:      email,
				Operation:  "insert",
				EntityType: "message_history",
				EntityID:   &messageID,
				Changes: map[string]interface{}{
					"template_id": map[string]interface{}{
						"new": "tpl123",
					},
					"channel": map[string]interface{}{
						"new": "email",
					},
				},
				CreatedAt: time.Now(),
			},
		}

		mockRepo.EXPECT().
			List(ctx, workspaceID, email, limit, (*string)(nil)).
			Return(expectedEntries, nil, nil)

		entries, _, err := service.List(ctx, workspaceID, email, limit, nil)

		require.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.Equal(t, "message_history", entries[0].EntityType)
		assert.NotNil(t, entries[0].EntityID)
		assert.Equal(t, "msg123", *entries[0].EntityID)
	})

	t.Run("Error - Repository error", func(t *testing.T) {
		mockRepo.EXPECT().
			List(ctx, workspaceID, email, limit, (*string)(nil)).
			Return(nil, nil, assert.AnError)

		entries, nextCursor, err := service.List(ctx, workspaceID, email, limit, nil)

		assert.Error(t, err)
		assert.Nil(t, entries)
		assert.Nil(t, nextCursor)
	})

	t.Run("Success - With different limit", func(t *testing.T) {
		customLimit := 10
		expectedEntries := []*domain.ContactTimelineEntry{
			{
				ID:         "entry5",
				Email:      email,
				Operation:  "delete",
				EntityType: "contact",
				CreatedAt:  time.Now(),
			},
		}

		mockRepo.EXPECT().
			List(ctx, workspaceID, email, customLimit, (*string)(nil)).
			Return(expectedEntries, nil, nil)

		entries, _, err := service.List(ctx, workspaceID, email, customLimit, nil)

		require.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.Equal(t, "delete", entries[0].Operation)
	})
}

func TestNewContactTimelineService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockContactTimelineRepository(ctrl)

	service := NewContactTimelineService(mockRepo)

	assert.NotNil(t, service)
	assert.IsType(t, &ContactTimelineService{}, service)
}
