package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// createContactServiceWithMocks creates a ContactService with all required mocks
func createContactServiceWithMocks(ctrl *gomock.Controller) (*ContactService, *mocks.MockContactRepository, *mocks.MockWorkspaceRepository, *mocks.MockAuthService, *mocks.MockMessageHistoryRepository, *mocks.MockInboundWebhookEventRepository, *mocks.MockContactListRepository, *mocks.MockContactTimelineRepository, *pkgmocks.MockLogger) {
	mockRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockInboundWebhookEventRepo := mocks.NewMockInboundWebhookEventRepository(ctrl)
	mockContactListRepo := mocks.NewMockContactListRepository(ctrl)
	mockContactTimelineRepo := mocks.NewMockContactTimelineRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewContactService(
		mockRepo,
		mockWorkspaceRepo,
		mockAuthService,
		mockMessageHistoryRepo,
		mockInboundWebhookEventRepo,
		mockContactListRepo,
		mockContactTimelineRepo,
		mockLogger,
	)

	return service, mockRepo, mockWorkspaceRepo, mockAuthService, mockMessageHistoryRepo, mockInboundWebhookEventRepo, mockContactListRepo, mockContactTimelineRepo, mockLogger
}

func TestContactService_GetContactByEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service, mockRepo, _, mockAuthService, _, _, _, _, mockLogger := createContactServiceWithMocks(ctrl)

	ctx := context.Background()
	workspaceID := "workspace123"
	email := "test@example.com"
	contact := &domain.Contact{
		Email: email,
	}

	userWorkspace := &domain.UserWorkspace{
		UserID:      "user123",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true, Write: true},
		},
	}

	t.Run("successful retrieval", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetContactByEmail(ctx, workspaceID, email).Return(contact, nil)

		result, err := service.GetContactByEmail(ctx, workspaceID, email)
		assert.NoError(t, err)
		assert.Equal(t, contact, result)
	})

	t.Run("successful retrieval with contact lists", func(t *testing.T) {
		firstName := &domain.NullableString{String: "John", IsNull: false}
		lastName := &domain.NullableString{String: "Doe", IsNull: false}
		contactWithLists := &domain.Contact{
			Email:     email,
			FirstName: firstName,
			LastName:  lastName,
			ContactLists: []*domain.ContactList{
				{
					Email:    email,
					ListID:   "newsletter",
					ListName: "Newsletter",
					Status:   domain.ContactListStatusActive,
				},
				{
					Email:    email,
					ListID:   "product_updates",
					ListName: "Product Updates",
					Status:   domain.ContactListStatusActive,
				},
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetContactByEmail(ctx, workspaceID, email).Return(contactWithLists, nil)

		result, err := service.GetContactByEmail(ctx, workspaceID, email)
		assert.NoError(t, err)
		assert.Equal(t, contactWithLists, result)
		assert.NotNil(t, result.ContactLists)
		assert.Len(t, result.ContactLists, 2)
		assert.Equal(t, "newsletter", result.ContactLists[0].ListID)
		assert.Equal(t, "Newsletter", result.ContactLists[0].ListName)
		assert.Equal(t, domain.ContactListStatusActive, result.ContactLists[0].Status)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, errors.New("auth error"))

		result, err := service.GetContactByEmail(ctx, workspaceID, email)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("insufficient permissions", func(t *testing.T) {
		userWorkspaceNoPerms := &domain.UserWorkspace{
			UserID:      "user123",
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceContacts: {Read: false, Write: false},
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspaceNoPerms, nil)

		result, err := service.GetContactByEmail(ctx, workspaceID, email)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "Insufficient permissions")
	})

	t.Run("contact not found", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetContactByEmail(ctx, workspaceID, email).Return(nil, fmt.Errorf("contact not found"))

		result, err := service.GetContactByEmail(ctx, workspaceID, email)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("repository error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetContactByEmail(ctx, workspaceID, email).Return(nil, errors.New("repo error"))
		mockLogger.EXPECT().WithField("email", email).Return(mockLogger)
		mockLogger.EXPECT().Error("Failed to get contact by email: repo error")

		result, err := service.GetContactByEmail(ctx, workspaceID, email)
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestContactService_GetContactByExternalID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service, mockRepo, _, mockAuthService, _, _, _, _, mockLogger := createContactServiceWithMocks(ctrl)

	ctx := context.Background()
	workspaceID := "workspace123"
	externalID := "ext123"
	contact := &domain.Contact{
		ExternalID: &domain.NullableString{String: externalID, IsNull: false},
	}

	userWorkspace := &domain.UserWorkspace{
		UserID:      "user123",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true, Write: true},
		},
	}

	t.Run("successful retrieval", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetContactByExternalID(ctx, workspaceID, externalID).Return(contact, nil)

		result, err := service.GetContactByExternalID(ctx, workspaceID, externalID)
		assert.NoError(t, err)
		assert.Equal(t, contact, result)
	})

	t.Run("successful retrieval with contact lists", func(t *testing.T) {
		extID := &domain.NullableString{String: externalID, IsNull: false}
		firstName := &domain.NullableString{String: "Jane", IsNull: false}
		lastName := &domain.NullableString{String: "Smith", IsNull: false}
		contactWithLists := &domain.Contact{
			Email:      "test@example.com",
			ExternalID: extID,
			FirstName:  firstName,
			LastName:   lastName,
			ContactLists: []*domain.ContactList{
				{
					Email:    "test@example.com",
					ListID:   "vip",
					ListName: "VIP Members",
					Status:   domain.ContactListStatusActive,
				},
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetContactByExternalID(ctx, workspaceID, externalID).Return(contactWithLists, nil)

		result, err := service.GetContactByExternalID(ctx, workspaceID, externalID)
		assert.NoError(t, err)
		assert.Equal(t, contactWithLists, result)
		assert.NotNil(t, result.ContactLists)
		assert.Len(t, result.ContactLists, 1)
		assert.Equal(t, "vip", result.ContactLists[0].ListID)
		assert.Equal(t, "VIP Members", result.ContactLists[0].ListName)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, errors.New("auth error"))

		result, err := service.GetContactByExternalID(ctx, workspaceID, externalID)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("contact not found", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetContactByExternalID(ctx, workspaceID, externalID).Return(nil, fmt.Errorf("contact not found"))

		result, err := service.GetContactByExternalID(ctx, workspaceID, externalID)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("repository error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetContactByExternalID(ctx, workspaceID, externalID).Return(nil, errors.New("repo error"))
		mockLogger.EXPECT().WithField("external_id", externalID).Return(mockLogger)
		mockLogger.EXPECT().Error("Failed to get contact by external ID: repo error")

		result, err := service.GetContactByExternalID(ctx, workspaceID, externalID)
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestContactService_GetContacts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service, mockRepo, _, mockAuthService, _, _, _, _, mockLogger := createContactServiceWithMocks(ctrl)

	ctx := context.Background()
	workspaceID := "workspace123"
	req := &domain.GetContactsRequest{
		WorkspaceID: workspaceID,
	}
	response := &domain.GetContactsResponse{
		Contacts: []*domain.Contact{
			{Email: "test1@example.com"},
			{Email: "test2@example.com"},
		},
	}

	userWorkspace := &domain.UserWorkspace{
		UserID:      "user123",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true, Write: true},
		},
	}

	t.Run("successful retrieval", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetContacts(ctx, req).Return(response, nil)

		result, err := service.GetContacts(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, response, result)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, errors.New("auth error"))

		result, err := service.GetContacts(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("repository error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetContacts(ctx, req).Return(nil, errors.New("repo error"))
		mockLogger.EXPECT().Error("Failed to get contacts: repo error")

		result, err := service.GetContacts(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestContactService_DeleteContact(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service, mockContactRepo, _, mockAuthService, mockMessageHistoryRepo, mockInboundWebhookEventRepo, mockContactListRepo, mockContactTimelineRepo, mockLogger := createContactServiceWithMocks(ctrl)

	ctx := context.Background()
	workspaceID := "test-workspace"
	email := "test@example.com"

	userWorkspace := &domain.UserWorkspace{
		UserID:      "user123",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true, Write: true},
		},
	}

	t.Run("successful deletion", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockMessageHistoryRepo.EXPECT().DeleteForEmail(ctx, workspaceID, email).Return(nil)
		mockInboundWebhookEventRepo.EXPECT().DeleteForEmail(ctx, workspaceID, email).Return(nil)
		mockContactListRepo.EXPECT().DeleteForEmail(ctx, workspaceID, email).Return(nil)
		mockContactTimelineRepo.EXPECT().DeleteForEmail(ctx, workspaceID, email).Return(nil)
		mockContactRepo.EXPECT().DeleteContact(ctx, workspaceID, email).Return(nil)

		err := service.DeleteContact(ctx, workspaceID, email)
		assert.NoError(t, err)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, fmt.Errorf("auth error"))

		err := service.DeleteContact(ctx, workspaceID, email)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("contact not found", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockMessageHistoryRepo.EXPECT().DeleteForEmail(ctx, workspaceID, email).Return(nil)
		mockInboundWebhookEventRepo.EXPECT().DeleteForEmail(ctx, workspaceID, email).Return(nil)
		mockContactListRepo.EXPECT().DeleteForEmail(ctx, workspaceID, email).Return(nil)
		mockContactTimelineRepo.EXPECT().DeleteForEmail(ctx, workspaceID, email).Return(nil)
		mockLogger.EXPECT().WithField("email", email).Return(mockLogger)
		mockContactRepo.EXPECT().DeleteContact(ctx, workspaceID, email).Return(fmt.Errorf("contact not found"))
		mockLogger.EXPECT().Error(fmt.Sprintf("Failed to delete contact: %v", fmt.Errorf("contact not found")))

		err := service.DeleteContact(ctx, workspaceID, email)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete contact")
	})
}

func TestContactService_UpsertContact(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service, mockRepo, _, mockAuthService, _, _, _, _, mockLogger := createContactServiceWithMocks(ctrl)

	ctx := context.Background()
	workspaceID := "workspace123"
	contact := &domain.Contact{
		Email: "test@example.com",
	}

	userWorkspace := &domain.UserWorkspace{
		UserID:      "user123",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true, Write: true},
		},
	}

	t.Run("successful create", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().UpsertContact(ctx, workspaceID, contact).Return(true, nil)

		result := service.UpsertContact(ctx, workspaceID, contact)
		assert.Equal(t, domain.UpsertContactOperationCreate, result.Action)
		assert.Empty(t, result.Error)
	})

	t.Run("successful update", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().UpsertContact(ctx, workspaceID, contact).Return(false, nil)

		result := service.UpsertContact(ctx, workspaceID, contact)
		assert.Equal(t, domain.UpsertContactOperationUpdate, result.Action)
		assert.Empty(t, result.Error)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, errors.New("auth error"))
		mockLogger.EXPECT().WithField("email", contact.Email).Return(mockLogger)
		mockLogger.EXPECT().Error("Failed to authenticate user: auth error")

		result := service.UpsertContact(ctx, workspaceID, contact)
		assert.Equal(t, domain.UpsertContactOperationError, result.Action)
		assert.Contains(t, result.Error, "auth error")
	})

	t.Run("repository error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().UpsertContact(ctx, workspaceID, contact).Return(false, errors.New("repo error"))
		mockLogger.EXPECT().WithField("email", contact.Email).Return(mockLogger)
		mockLogger.EXPECT().Error("Failed to upsert contact: repo error")

		result := service.UpsertContact(ctx, workspaceID, contact)
		assert.Equal(t, domain.UpsertContactOperationError, result.Action)
		assert.Contains(t, result.Error, "repo error")
	})

	t.Run("validation error", func(t *testing.T) {
		invalidContact := &domain.Contact{
			Email: "", // Empty email should fail validation
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockLogger.EXPECT().WithField("email", invalidContact.Email).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any()) // Any validation error message

		result := service.UpsertContact(ctx, workspaceID, invalidContact)
		assert.Equal(t, domain.UpsertContactOperationError, result.Action)
		assert.NotEmpty(t, result.Error)
	})
}

func TestContactService_UpsertContactWithPartialUpdates(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service, mockRepo, _, mockAuthService, _, _, _, _, _ := createContactServiceWithMocks(ctrl)

	ctx := context.Background()
	workspaceID := "workspace123"

	userWorkspace := &domain.UserWorkspace{
		UserID:      "user123",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true, Write: true},
		},
	}

	t.Run("upsert with only email", func(t *testing.T) {
		// Create a contact with only email
		minimalContact := &domain.Contact{
			Email: "minimal@example.com",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().UpsertContact(ctx, workspaceID, gomock.Any()).DoAndReturn(
			func(ctx context.Context, workspaceID string, contact *domain.Contact) (bool, error) {
				// Verify that contact has CreatedAt and UpdatedAt set
				assert.NotZero(t, contact.CreatedAt.Unix())
				assert.NotZero(t, contact.UpdatedAt.Unix())
				assert.Equal(t, "minimal@example.com", contact.Email)
				return true, nil
			})

		result := service.UpsertContact(ctx, workspaceID, minimalContact)
		assert.Equal(t, domain.UpsertContactOperationCreate, result.Action)
		assert.Empty(t, result.Error)
	})

	t.Run("upsert with partial fields", func(t *testing.T) {
		// Create a contact with partial fields
		partialContact := &domain.Contact{
			Email:     "partial@example.com",
			FirstName: &domain.NullableString{String: "Jane", IsNull: false},
			LastName:  &domain.NullableString{String: "Smith", IsNull: false},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().UpsertContact(ctx, workspaceID, gomock.Any()).DoAndReturn(
			func(ctx context.Context, workspaceID string, contact *domain.Contact) (bool, error) {
				// Verify that only the specified fields are set
				assert.NotZero(t, contact.CreatedAt.Unix())
				assert.NotZero(t, contact.UpdatedAt.Unix())
				assert.Equal(t, "partial@example.com", contact.Email)
				assert.Equal(t, "Jane", contact.FirstName.String)
				assert.Equal(t, "Smith", contact.LastName.String)
				assert.False(t, contact.FirstName.IsNull)
				assert.False(t, contact.LastName.IsNull)
				// Other fields should be nil
				assert.Nil(t, contact.ExternalID)
				assert.Nil(t, contact.Phone)
				assert.Nil(t, contact.CustomJSON1)
				return false, nil
			})

		result := service.UpsertContact(ctx, workspaceID, partialContact)
		assert.Equal(t, domain.UpsertContactOperationUpdate, result.Action)
		assert.Empty(t, result.Error)
	})

	t.Run("upsert with custom JSON", func(t *testing.T) {
		// Create a contact with custom JSON fields
		jsonData := map[string]interface{}{
			"preference": "email",
			"frequency":  "weekly",
		}
		jsonContact := &domain.Contact{
			Email:       "json@example.com",
			CustomJSON1: &domain.NullableJSON{Data: jsonData, IsNull: false},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().UpsertContact(ctx, workspaceID, gomock.Any()).DoAndReturn(
			func(ctx context.Context, workspaceID string, contact *domain.Contact) (bool, error) {
				// Verify that JSON field is properly set
				assert.NotZero(t, contact.CreatedAt.Unix())
				assert.NotZero(t, contact.UpdatedAt.Unix())
				assert.Equal(t, "json@example.com", contact.Email)
				assert.NotNil(t, contact.CustomJSON1)
				assert.Equal(t, jsonData, contact.CustomJSON1.Data)
				assert.False(t, contact.CustomJSON1.IsNull)
				// Other fields should be nil
				assert.Nil(t, contact.FirstName)
				assert.Nil(t, contact.LastName)
				return true, nil
			})

		result := service.UpsertContact(ctx, workspaceID, jsonContact)
		assert.Equal(t, domain.UpsertContactOperationCreate, result.Action)
		assert.Empty(t, result.Error)
	})

	t.Run("upsert with explicit null field", func(t *testing.T) {
		// Create a contact with some fields explicitly set to null
		contactWithNulls := &domain.Contact{
			Email:       "null@example.com",
			FirstName:   &domain.NullableString{String: "", IsNull: true},
			CustomJSON1: &domain.NullableJSON{Data: nil, IsNull: true},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().UpsertContact(ctx, workspaceID, gomock.Any()).DoAndReturn(
			func(ctx context.Context, workspaceID string, contact *domain.Contact) (bool, error) {
				// Verify that null fields are properly set
				assert.NotZero(t, contact.CreatedAt.Unix())
				assert.NotZero(t, contact.UpdatedAt.Unix())
				assert.Equal(t, "null@example.com", contact.Email)
				assert.NotNil(t, contact.FirstName)
				assert.True(t, contact.FirstName.IsNull)
				assert.NotNil(t, contact.CustomJSON1)
				assert.True(t, contact.CustomJSON1.IsNull)
				assert.Nil(t, contact.CustomJSON1.Data)
				// Other fields should be nil
				assert.Nil(t, contact.LastName)
				return false, nil
			})

		result := service.UpsertContact(ctx, workspaceID, contactWithNulls)
		assert.Equal(t, domain.UpsertContactOperationUpdate, result.Action)
		assert.Empty(t, result.Error)
	})
}

func TestContactService_BatchImportContacts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service, mockRepo, _, mockAuthService, _, _, _, _, mockLogger := createContactServiceWithMocks(ctrl)

	ctx := context.Background()
	workspaceID := "workspace123"

	userWorkspace := &domain.UserWorkspace{
		UserID:      "user123",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true, Write: true},
		},
	}

	t.Run("authentication error", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: "contact1@example.com"},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, errors.New("auth error"))

		response := service.BatchImportContacts(ctx, workspaceID, contacts, nil)
		assert.NotNil(t, response)
		assert.Contains(t, response.Error, "failed to authenticate user")
	})

	t.Run("validation error", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: ""}, // Invalid email
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)

		response := service.BatchImportContacts(ctx, workspaceID, contacts, nil)
		assert.NotNil(t, response)

		// Find the error operation in the response
		var foundErrorOp bool
		for _, op := range response.Operations {
			if op != nil && op.Action == domain.UpsertContactOperationError {
				foundErrorOp = true
				assert.Equal(t, "", op.Email)
				assert.Contains(t, op.Error, "invalid contact")
				break
			}
		}
		assert.True(t, foundErrorOp, "No error operation found in response")
	})

	t.Run("repository error", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: "valid@example.com"},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().BulkUpsertContacts(ctx, workspaceID, gomock.Any()).Return(nil, errors.New("repo error"))
		mockLogger.EXPECT().Error(gomock.Any())

		response := service.BatchImportContacts(ctx, workspaceID, contacts, nil)
		assert.NotNil(t, response)

		// Find the error operation in the response
		var foundErrorOp bool
		for _, op := range response.Operations {
			if op != nil && op.Action == domain.UpsertContactOperationError {
				foundErrorOp = true
				assert.Equal(t, "valid@example.com", op.Email)
				assert.Contains(t, op.Error, "failed to upsert contact")
				break
			}
		}
		assert.True(t, foundErrorOp, "No error operation found in response")
	})

	t.Run("successful mixed operations", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: "new@example.com"},
			{Email: "existing@example.com"},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)

		// Expect bulk upsert with both contacts
		mockRepo.EXPECT().BulkUpsertContacts(ctx, workspaceID, contacts).Return([]domain.BulkUpsertResult{
			{Email: "new@example.com", IsNew: true},
			{Email: "existing@example.com", IsNew: false},
		}, nil)

		response := service.BatchImportContacts(ctx, workspaceID, contacts, nil)
		assert.NotNil(t, response)
		assert.Empty(t, response.Error)

		// Verify operations
		assert.Len(t, response.Operations, 2)

		// Find the operations by email
		var newOp, existingOp *domain.UpsertContactOperation
		for _, op := range response.Operations {
			switch op.Email {
			case "new@example.com":
				newOp = op
			case "existing@example.com":
				existingOp = op
			}
		}

		assert.NotNil(t, newOp)
		assert.Equal(t, domain.UpsertContactOperationCreate, newOp.Action)
		assert.NotNil(t, existingOp)
		assert.Equal(t, domain.UpsertContactOperationUpdate, existingOp.Action)
	})
}

func TestContactService_BatchImportContacts_WithBulkOperations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service, mockRepo, _, mockAuthService, _, _, mockContactListRepo, _, mockLogger := createContactServiceWithMocks(ctrl)

	ctx := context.Background()
	workspaceID := "workspace123"

	userWorkspace := &domain.UserWorkspace{
		UserID:      "user123",
		WorkspaceID: workspaceID,
		Role:        "admin",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true, Write: true},
			domain.PermissionResourceLists:    {Read: true, Write: true},
		},
	}

	t.Run("successful bulk import without lists", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: "test1@example.com"},
			{Email: "test2@example.com"},
			{Email: "test3@example.com"},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)

		// Expect bulk upsert to be called with all valid contacts
		mockRepo.EXPECT().BulkUpsertContacts(ctx, workspaceID, contacts).Return([]domain.BulkUpsertResult{
			{Email: "test1@example.com", IsNew: true},
			{Email: "test2@example.com", IsNew: true},
			{Email: "test3@example.com", IsNew: false},
		}, nil)

		response := service.BatchImportContacts(ctx, workspaceID, contacts, nil)

		assert.NotNil(t, response)
		assert.Empty(t, response.Error)
		assert.Len(t, response.Operations, 3)
		assert.Equal(t, domain.UpsertContactOperationCreate, response.Operations[0].Action)
		assert.Equal(t, domain.UpsertContactOperationCreate, response.Operations[1].Action)
		assert.Equal(t, domain.UpsertContactOperationUpdate, response.Operations[2].Action)
	})

	t.Run("successful bulk import with lists", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: "test1@example.com"},
			{Email: "test2@example.com"},
		}
		listIDs := []string{"list1", "list2"}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)

		mockRepo.EXPECT().BulkUpsertContacts(ctx, workspaceID, contacts).Return([]domain.BulkUpsertResult{
			{Email: "test1@example.com", IsNew: true},
			{Email: "test2@example.com", IsNew: true},
		}, nil)

		// Expect bulk list subscription
		mockContactListRepo.EXPECT().BulkAddContactsToLists(
			ctx,
			workspaceID,
			[]string{"test1@example.com", "test2@example.com"},
			listIDs,
			domain.ContactListStatusActive,
		).Return(nil)

		response := service.BatchImportContacts(ctx, workspaceID, contacts, listIDs)

		assert.NotNil(t, response)
		assert.Empty(t, response.Error)
		assert.Len(t, response.Operations, 2)
	})

	t.Run("bulk import with validation errors", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: "valid@example.com"},
			{Email: ""}, // Invalid - no email
			{Email: "another@example.com"},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)

		// Only valid contacts should be passed to bulk upsert
		validContacts := []*domain.Contact{
			{Email: "valid@example.com"},
			{Email: "another@example.com"},
		}

		mockRepo.EXPECT().BulkUpsertContacts(ctx, workspaceID, validContacts).Return([]domain.BulkUpsertResult{
			{Email: "valid@example.com", IsNew: true},
			{Email: "another@example.com", IsNew: true},
		}, nil)

		response := service.BatchImportContacts(ctx, workspaceID, contacts, nil)

		assert.NotNil(t, response)
		assert.Empty(t, response.Error)
		assert.Len(t, response.Operations, 3) // 2 success + 1 validation error

		// Check that one operation is an error
		errorCount := 0
		for _, op := range response.Operations {
			if op.Action == domain.UpsertContactOperationError {
				errorCount++
			}
		}
		assert.Equal(t, 1, errorCount)
	})

	t.Run("bulk upsert fails", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: "test1@example.com"},
			{Email: "test2@example.com"},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)

		mockRepo.EXPECT().BulkUpsertContacts(ctx, workspaceID, contacts).Return(nil, errors.New("database error"))
		mockLogger.EXPECT().Error(gomock.Any())

		response := service.BatchImportContacts(ctx, workspaceID, contacts, nil)

		assert.NotNil(t, response)
		assert.Empty(t, response.Error)
		assert.Len(t, response.Operations, 2)
		// All operations should be marked as errors
		for _, op := range response.Operations {
			assert.Equal(t, domain.UpsertContactOperationError, op.Action)
		}
	})

	t.Run("list subscription fails (non-critical)", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: "test1@example.com"},
		}
		listIDs := []string{"list1"}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)

		mockRepo.EXPECT().BulkUpsertContacts(ctx, workspaceID, contacts).Return([]domain.BulkUpsertResult{
			{Email: "test1@example.com", IsNew: true},
		}, nil)

		// List subscription fails, but shouldn't fail the entire operation
		mockContactListRepo.EXPECT().BulkAddContactsToLists(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
		).Return(errors.New("list error"))
		mockLogger.EXPECT().Error(gomock.Any())

		response := service.BatchImportContacts(ctx, workspaceID, contacts, listIDs)

		// Contact should still be created successfully
		assert.NotNil(t, response)
		assert.Empty(t, response.Error)
		assert.Len(t, response.Operations, 1)
		assert.Equal(t, domain.UpsertContactOperationCreate, response.Operations[0].Action)
	})

	t.Run("insufficient permissions for lists", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: "test1@example.com"},
		}
		listIDs := []string{"list1"}

		userWorkspaceNoListPerms := &domain.UserWorkspace{
			UserID:      "user123",
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceContacts: {Read: true, Write: true},
				domain.PermissionResourceLists:    {Read: true, Write: false},
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspaceNoListPerms, nil)

		response := service.BatchImportContacts(ctx, workspaceID, contacts, listIDs)

		assert.NotNil(t, response)
		assert.NotEmpty(t, response.Error)
		assert.Contains(t, response.Error, "write access to lists required")
	})
}

func TestContactService_BatchImportContacts_DuplicateEmails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service, mockRepo, _, mockAuthService, _, _, _, _, _ := createContactServiceWithMocks(ctrl)

	ctx := context.Background()
	workspaceID := "workspace123"

	userWorkspace := &domain.UserWorkspace{
		UserID:      "user123",
		WorkspaceID: workspaceID,
		Role:        "admin",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true, Write: true},
		},
	}

	t.Run("duplicate emails are deduplicated - last occurrence wins", func(t *testing.T) {
		// Input has the same email appearing 3 times with different first names
		contacts := []*domain.Contact{
			{Email: "duplicate@example.com", FirstName: &domain.NullableString{String: "First", IsNull: false}},
			{Email: "unique@example.com", FirstName: &domain.NullableString{String: "Unique", IsNull: false}},
			{Email: "duplicate@example.com", FirstName: &domain.NullableString{String: "Second", IsNull: false}},
			{Email: "duplicate@example.com", FirstName: &domain.NullableString{String: "Third", IsNull: false}}, // This should be kept
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)

		// After deduplication, only 2 contacts should be passed to BulkUpsertContacts
		// The last occurrence of duplicate@example.com (with FirstName="Third") should be kept
		mockRepo.EXPECT().BulkUpsertContacts(ctx, workspaceID, gomock.Any()).DoAndReturn(
			func(ctx context.Context, wsID string, deduped []*domain.Contact) ([]domain.BulkUpsertResult, error) {
				// Verify deduplication happened correctly
				assert.Len(t, deduped, 2)
				// Find the duplicate email contact and verify it has the last occurrence's data
				for _, c := range deduped {
					if c.Email == "duplicate@example.com" {
						assert.Equal(t, "Third", c.FirstName.String, "Should keep last occurrence")
					}
				}
				return []domain.BulkUpsertResult{
					{Email: "unique@example.com", IsNew: true},
					{Email: "duplicate@example.com", IsNew: true},
				}, nil
			})

		response := service.BatchImportContacts(ctx, workspaceID, contacts, nil)

		assert.NotNil(t, response)
		assert.Empty(t, response.Error)
		// Response should have 2 operations (deduplicated count)
		assert.Len(t, response.Operations, 2)
	})

	t.Run("single contact with duplicate email in batch", func(t *testing.T) {
		// Even with just 2 contacts where both have the same email
		contacts := []*domain.Contact{
			{Email: "same@example.com", FirstName: &domain.NullableString{String: "First", IsNull: false}},
			{Email: "same@example.com", FirstName: &domain.NullableString{String: "Last", IsNull: false}},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)

		mockRepo.EXPECT().BulkUpsertContacts(ctx, workspaceID, gomock.Any()).DoAndReturn(
			func(ctx context.Context, wsID string, deduped []*domain.Contact) ([]domain.BulkUpsertResult, error) {
				assert.Len(t, deduped, 1)
				assert.Equal(t, "Last", deduped[0].FirstName.String)
				return []domain.BulkUpsertResult{
					{Email: "same@example.com", IsNew: true},
				}, nil
			})

		response := service.BatchImportContacts(ctx, workspaceID, contacts, nil)

		assert.NotNil(t, response)
		assert.Empty(t, response.Error)
		assert.Len(t, response.Operations, 1)
	})

	t.Run("no duplicates passes through unchanged", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: "a@example.com"},
			{Email: "b@example.com"},
			{Email: "c@example.com"},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)

		mockRepo.EXPECT().BulkUpsertContacts(ctx, workspaceID, contacts).Return([]domain.BulkUpsertResult{
			{Email: "a@example.com", IsNew: true},
			{Email: "b@example.com", IsNew: true},
			{Email: "c@example.com", IsNew: true},
		}, nil)

		response := service.BatchImportContacts(ctx, workspaceID, contacts, nil)

		assert.NotNil(t, response)
		assert.Empty(t, response.Error)
		assert.Len(t, response.Operations, 3)
	})
}

func TestContactService_BatchImportContacts_Chunking(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service, mockRepo, _, mockAuthService, _, _, _, _, mockLogger := createContactServiceWithMocks(ctrl)

	ctx := context.Background()
	workspaceID := "workspace123"

	userWorkspace := &domain.UserWorkspace{
		UserID:      "user123",
		WorkspaceID: workspaceID,
		Role:        "admin",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true, Write: true},
		},
	}

	t.Run("large batch is chunked into multiple BulkUpsertContacts calls", func(t *testing.T) {
		// Create 501 contacts to force 2 chunks (500 + 1)
		contacts := make([]*domain.Contact, 501)
		for i := range contacts {
			contacts[i] = &domain.Contact{Email: fmt.Sprintf("test%d@example.com", i)}
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)

		// Expect 2 BulkUpsertContacts calls: first chunk of 500, then chunk of 1
		firstResults := make([]domain.BulkUpsertResult, 500)
		for i := 0; i < 500; i++ {
			firstResults[i] = domain.BulkUpsertResult{
				Email: fmt.Sprintf("test%d@example.com", i), IsNew: true,
			}
		}

		firstCall := mockRepo.EXPECT().BulkUpsertContacts(ctx, workspaceID, gomock.Any()).DoAndReturn(
			func(ctx context.Context, wsID string, chunk []*domain.Contact) ([]domain.BulkUpsertResult, error) {
				assert.Len(t, chunk, 500, "first chunk should have 500 contacts")
				return firstResults, nil
			})

		secondCall := mockRepo.EXPECT().BulkUpsertContacts(ctx, workspaceID, gomock.Any()).DoAndReturn(
			func(ctx context.Context, wsID string, chunk []*domain.Contact) ([]domain.BulkUpsertResult, error) {
				assert.Len(t, chunk, 1, "second chunk should have 1 contact")
				return []domain.BulkUpsertResult{
					{Email: "test500@example.com", IsNew: true},
				}, nil
			})

		gomock.InOrder(firstCall, secondCall)

		response := service.BatchImportContacts(ctx, workspaceID, contacts, nil)

		assert.NotNil(t, response)
		assert.Empty(t, response.Error)
		assert.Len(t, response.Operations, 501)

		// All operations should be successful creates
		for _, op := range response.Operations {
			assert.Equal(t, domain.UpsertContactOperationCreate, op.Action)
		}
	})

	t.Run("chunk failure continues with remaining chunks", func(t *testing.T) {
		// Create 501 contacts to force 2 chunks
		contacts := make([]*domain.Contact, 501)
		for i := range contacts {
			contacts[i] = &domain.Contact{Email: fmt.Sprintf("test%d@example.com", i)}
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)

		// First chunk of 500 fails
		firstCall := mockRepo.EXPECT().BulkUpsertContacts(ctx, workspaceID, gomock.Any()).DoAndReturn(
			func(ctx context.Context, wsID string, chunk []*domain.Contact) ([]domain.BulkUpsertResult, error) {
				assert.Len(t, chunk, 500)
				return nil, errors.New("database error on chunk 1")
			})
		mockLogger.EXPECT().Error(gomock.Any())

		// Second chunk of 1 succeeds
		secondCall := mockRepo.EXPECT().BulkUpsertContacts(ctx, workspaceID, gomock.Any()).DoAndReturn(
			func(ctx context.Context, wsID string, chunk []*domain.Contact) ([]domain.BulkUpsertResult, error) {
				assert.Len(t, chunk, 1)
				return []domain.BulkUpsertResult{
					{Email: "test500@example.com", IsNew: true},
				}, nil
			})

		gomock.InOrder(firstCall, secondCall)

		response := service.BatchImportContacts(ctx, workspaceID, contacts, nil)

		assert.NotNil(t, response)
		assert.Empty(t, response.Error)
		assert.Len(t, response.Operations, 501) // 500 errors + 1 success

		errorCount := 0
		successCount := 0
		for _, op := range response.Operations {
			if op.Action == domain.UpsertContactOperationError {
				errorCount++
			} else {
				successCount++
			}
		}
		assert.Equal(t, 500, errorCount, "failed chunk should produce 500 error operations")
		assert.Equal(t, 1, successCount, "successful chunk should produce 1 success operation")
	})
}

func TestContactService_CountContacts(t *testing.T) {
	// Test ContactService.CountContacts - this was at 0% coverage
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service, mockRepo, _, mockAuthService, _, _, _, _, mockLogger := createContactServiceWithMocks(ctrl)

	ctx := context.Background()
	workspaceID := "workspace123"

	userWorkspace := &domain.UserWorkspace{
		UserID:      "user123",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: {Read: true, Write: true},
		},
	}

	t.Run("Success - Returns count", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().Count(ctx, workspaceID).Return(42, nil)

		count, err := service.CountContacts(ctx, workspaceID)
		assert.NoError(t, err)
		assert.Equal(t, 42, count)
	})

	t.Run("Error - Authentication fails", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, errors.New("auth error"))

		count, err := service.CountContacts(ctx, workspaceID)
		assert.Error(t, err)
		assert.Equal(t, 0, count)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("Error - Insufficient permissions", func(t *testing.T) {
		userWorkspaceNoPerms := &domain.UserWorkspace{
			UserID:      "user123",
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceContacts: {Read: false, Write: false},
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspaceNoPerms, nil)

		count, err := service.CountContacts(ctx, workspaceID)
		assert.Error(t, err)
		assert.Equal(t, 0, count)
		var permErr *domain.PermissionError
		assert.True(t, errors.As(err, &permErr))
	})

	t.Run("Error - Repository error", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().Count(ctx, workspaceID).Return(0, errors.New("repository error"))
		mockLogger.EXPECT().Error(gomock.Any())

		count, err := service.CountContacts(ctx, workspaceID)
		assert.Error(t, err)
		assert.Equal(t, 0, count)
		assert.Contains(t, err.Error(), "failed to count contacts")
	})
}
