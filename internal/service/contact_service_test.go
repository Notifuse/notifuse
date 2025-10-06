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
func createContactServiceWithMocks(ctrl *gomock.Controller) (*ContactService, *mocks.MockContactRepository, *mocks.MockWorkspaceRepository, *mocks.MockAuthService, *mocks.MockMessageHistoryRepository, *mocks.MockWebhookEventRepository, *mocks.MockContactListRepository, *mocks.MockContactTimelineRepository, *pkgmocks.MockLogger) {
	mockRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockWebhookEventRepo := mocks.NewMockWebhookEventRepository(ctrl)
	mockContactListRepo := mocks.NewMockContactListRepository(ctrl)
	mockContactTimelineRepo := mocks.NewMockContactTimelineRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewContactService(
		mockRepo,
		mockWorkspaceRepo,
		mockAuthService,
		mockMessageHistoryRepo,
		mockWebhookEventRepo,
		mockContactListRepo,
		mockContactTimelineRepo,
		mockLogger,
	)

	return service, mockRepo, mockWorkspaceRepo, mockAuthService, mockMessageHistoryRepo, mockWebhookEventRepo, mockContactListRepo, mockContactTimelineRepo, mockLogger
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

	service, mockContactRepo, _, mockAuthService, mockMessageHistoryRepo, mockWebhookEventRepo, mockContactListRepo, mockContactTimelineRepo, mockLogger := createContactServiceWithMocks(ctrl)

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
		mockWebhookEventRepo.EXPECT().DeleteForEmail(ctx, workspaceID, email).Return(nil)
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
		mockWebhookEventRepo.EXPECT().DeleteForEmail(ctx, workspaceID, email).Return(nil)
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

	t.Run("authentication error", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: "contact1@example.com"},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, errors.New("auth error"))

		response := service.BatchImportContacts(ctx, workspaceID, contacts)
		assert.NotNil(t, response)
		assert.Contains(t, response.Error, "failed to authenticate user")
	})

	t.Run("validation error", func(t *testing.T) {
		contacts := []*domain.Contact{
			{Email: ""}, // Invalid email
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)

		response := service.BatchImportContacts(ctx, workspaceID, contacts)
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
		mockRepo.EXPECT().UpsertContact(ctx, workspaceID, gomock.Any()).Return(false, errors.New("repo error"))

		response := service.BatchImportContacts(ctx, workspaceID, contacts)
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

		// First contact is new
		mockRepo.EXPECT().UpsertContact(ctx, workspaceID, gomock.Any()).DoAndReturn(
			func(ctx context.Context, workspaceID string, contact *domain.Contact) (bool, error) {
				assert.Equal(t, "new@example.com", contact.Email)
				// CreatedAt and UpdatedAt are optional - if not provided (zero value), DB will set them
				return true, nil // true means it's a new contact
			})

		// Second contact is an update
		mockRepo.EXPECT().UpsertContact(ctx, workspaceID, gomock.Any()).DoAndReturn(
			func(ctx context.Context, workspaceID string, contact *domain.Contact) (bool, error) {
				assert.Equal(t, "existing@example.com", contact.Email)
				// CreatedAt and UpdatedAt are optional - if not provided (zero value), DB will set them
				return false, nil // false means it's an existing contact
			})

		response := service.BatchImportContacts(ctx, workspaceID, contacts)
		assert.NotNil(t, response)
		assert.Empty(t, response.Error)

		// Due to how the method works, we need to filter to verify the operations
		createOperations := []*domain.UpsertContactOperation{}
		updateOperations := []*domain.UpsertContactOperation{}

		for _, op := range response.Operations {
			if op != nil {
				if op.Action == domain.UpsertContactOperationCreate && op.Email == "new@example.com" {
					createOperations = append(createOperations, op)
				} else if op.Action == domain.UpsertContactOperationUpdate && op.Email == "existing@example.com" {
					updateOperations = append(updateOperations, op)
				}
			}
		}

		assert.GreaterOrEqual(t, len(createOperations), 1)
		assert.GreaterOrEqual(t, len(updateOperations), 1)
	})
}
