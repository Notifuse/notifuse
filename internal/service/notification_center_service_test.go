package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/pkg/logger"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// TestNotificationCenterService extends the original service to allow easier testing
type TestNotificationCenterService struct {
	*NotificationCenterService
	mockHMACVerifier func(email string, providedHMAC string, secretKey string) bool
}

// NewTestNotificationCenterService creates a testable notification center service
func NewTestNotificationCenterService(
	contactRepo domain.ContactRepository,
	workspaceRepo domain.WorkspaceRepository,
	listRepo domain.ListRepository,
	logger logger.Logger,
	mockHMACVerifier func(email string, providedHMAC string, secretKey string) bool,
) *TestNotificationCenterService {
	return &TestNotificationCenterService{
		NotificationCenterService: NewNotificationCenterService(contactRepo, workspaceRepo, listRepo, logger),
		mockHMACVerifier:          mockHMACVerifier,
	}
}

// UpdateContactPreferences overrides the original method to use our mock HMAC verifier
func (s *TestNotificationCenterService) UpdateContactPreferences(ctx context.Context, req *domain.UpdateContactPreferencesRequest) error {
	workspace, err := s.workspaceRepo.GetByID(ctx, req.WorkspaceID)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to get workspace: %v", err))
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	if !s.mockHMACVerifier(req.Email, req.EmailHMAC, workspace.Settings.SecretKey) {
		return fmt.Errorf("invalid email verification")
	}

	contact := &domain.Contact{Email: req.Email}
	if req.Language != "" {
		contact.Language = &domain.NullableString{String: req.Language, IsNull: false}
	}
	if req.Timezone != "" {
		contact.Timezone = &domain.NullableString{String: req.Timezone, IsNull: false}
	}

	_, err = s.contactRepo.UpsertContact(ctx, req.WorkspaceID, contact)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to upsert contact preferences: %v", err))
		return fmt.Errorf("failed to update contact preferences: %w", err)
	}

	return nil
}

// GetContactPreferences overrides the original method to use our mock HMAC verifier
func (s *TestNotificationCenterService) GetContactPreferences(ctx context.Context, workspaceID string, email string, emailHMAC string) (*domain.ContactPreferencesResponse, error) {
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to get workspace: %v", err))
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	// Use our mock verifier instead of the domain function
	if !s.mockHMACVerifier(email, emailHMAC, workspace.Settings.SecretKey) {
		return nil, fmt.Errorf("invalid email verification")
	}

	// Get the contact
	contact, err := s.contactRepo.GetContactByEmail(ctx, workspaceID, email)
	if err != nil {
		if strings.Contains(err.Error(), "contact not found") {
			return nil, err
		}
		s.logger.WithField("email", email).Error(fmt.Sprintf("Failed to get contact: %v", err))
		return nil, fmt.Errorf("failed to get contact: %w", err)
	}

	// Get public lists for this workspace
	publicLists := make([]*domain.List, 0)

	// Get lists using the list service
	lists, err := s.listRepo.GetLists(ctx, workspaceID)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to get lists: %v", err))
	} else {
		// Filter to only include public lists
		for _, list := range lists {
			if list.IsPublic {
				publicLists = append(publicLists, list)
			}
		}
	}

	return &domain.ContactPreferencesResponse{
		Contact:      contact,
		PublicLists:  publicLists,
		ContactLists: contact.ContactLists,
		LogoURL:      workspace.Settings.LogoURL,
		WebsiteURL:   workspace.Settings.WebsiteURL,
	}, nil
}

func TestNotificationCenterService_GetContactPreferences(t *testing.T) {
	// Set up a fixed secret key for all tests
	secretKey := "test-secret-key"

	// Pre-compute the valid HMAC for test@example.com using our secret key
	validEmail := "user@example.com"
	validHMAC := crypto.ComputeHMAC256([]byte(validEmail), secretKey)

	// Set up a mock contact for use in the expected responses
	mockContactLists := []*domain.ContactList{
		{
			ListID: "list-1",
			Status: domain.ContactListStatusActive,
		},
	}

	// Define test cases
	testCases := []struct {
		name          string
		workspaceID   string
		email         string
		emailHMAC     string
		setupMocks    func(ctrl *gomock.Controller, mockContactRepo *mocks.MockContactRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockListRepo *mocks.MockListRepository, mockLogger *pkgmocks.MockLogger)
		expectedResp  *domain.ContactPreferencesResponse
		expectedError string
	}{
		{
			name:        "Success with all data",
			workspaceID: "workspace-123",
			email:       validEmail,
			emailHMAC:   validHMAC, // Use the pre-computed valid HMAC
			setupMocks: func(ctrl *gomock.Controller, mockContactRepo *mocks.MockContactRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockListRepo *mocks.MockListRepository, mockLogger *pkgmocks.MockLogger) {
				// Setup workspace
				workspace := &domain.Workspace{
					ID:   "workspace-123",
					Name: "Test Workspace",
					Settings: domain.WorkspaceSettings{
						LogoURL:    "https://example.com/logo.png",
						WebsiteURL: "https://example.com",
						SecretKey:  secretKey, // Use our test secret key
					},
				}
				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(workspace, nil)

				// Setup contact
				contact := &domain.Contact{
					Email: validEmail,
					ContactLists: []*domain.ContactList{
						{
							ListID: "list-1",
							Status: domain.ContactListStatusActive,
						},
					},
				}
				mockContactRepo.EXPECT().
					GetContactByEmail(gomock.Any(), "workspace-123", validEmail).
					Return(contact, nil)

				// Setup lists
				lists := []*domain.List{
					{
						ID:       "list-1",
						Name:     "Public List 1",
						IsPublic: true,
					},
					{
						ID:       "list-2",
						Name:     "Public List 2",
						IsPublic: true,
					},
					{
						ID:       "list-3",
						Name:     "Private List",
						IsPublic: false, // This one should be filtered out
					},
				}
				mockListRepo.EXPECT().
					GetLists(gomock.Any(), "workspace-123").
					Return(lists, nil)
			},
			expectedResp: &domain.ContactPreferencesResponse{
				Contact: &domain.Contact{
					Email: validEmail,
					ContactLists: []*domain.ContactList{
						{
							ListID: "list-1",
							Status: domain.ContactListStatusActive,
						},
					},
				},
				PublicLists: []*domain.List{
					{
						ID:       "list-1",
						Name:     "Public List 1",
						IsPublic: true,
					},
					{
						ID:       "list-2",
						Name:     "Public List 2",
						IsPublic: true,
					},
				},
				ContactLists: mockContactLists,
				LogoURL:      "https://example.com/logo.png",
				WebsiteURL:   "https://example.com",
			},
			expectedError: "",
		},
		{
			name:        "Invalid email HMAC",
			workspaceID: "workspace-123",
			email:       validEmail,
			emailHMAC:   "invalid-hmac", // This will be rejected
			setupMocks: func(ctrl *gomock.Controller, mockContactRepo *mocks.MockContactRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockListRepo *mocks.MockListRepository, mockLogger *pkgmocks.MockLogger) {
				// Setup workspace
				workspace := &domain.Workspace{
					ID:   "workspace-123",
					Name: "Test Workspace",
					Settings: domain.WorkspaceSettings{
						SecretKey: secretKey, // Use our test secret key
					},
				}
				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(workspace, nil)

				// No contact or list repo calls expected since we'll fail at HMAC check
			},
			expectedResp:  nil,
			expectedError: "invalid email verification",
		},
		{
			name:        "Workspace not found",
			workspaceID: "nonexistent-workspace",
			email:       validEmail,
			emailHMAC:   validHMAC,
			setupMocks: func(ctrl *gomock.Controller, mockContactRepo *mocks.MockContactRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockListRepo *mocks.MockListRepository, mockLogger *pkgmocks.MockLogger) {
				// Setup workspace repo to return error
				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "nonexistent-workspace").
					Return(nil, errors.New("workspace not found"))

				// Expect error to be logged
				mockLogger.EXPECT().
					Error(gomock.Any())
			},
			expectedResp:  nil,
			expectedError: "failed to get workspace: workspace not found",
		},
		{
			name:        "Contact not found",
			workspaceID: "workspace-123",
			email:       "nonexistent@example.com",
			emailHMAC:   crypto.ComputeHMAC256([]byte("nonexistent@example.com"), secretKey), // Pre-compute valid HMAC for this email
			setupMocks: func(ctrl *gomock.Controller, mockContactRepo *mocks.MockContactRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockListRepo *mocks.MockListRepository, mockLogger *pkgmocks.MockLogger) {
				// Setup workspace
				workspace := &domain.Workspace{
					ID:   "workspace-123",
					Name: "Test Workspace",
					Settings: domain.WorkspaceSettings{
						SecretKey: secretKey, // Use our test secret key
					},
				}
				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(workspace, nil)

				// Setup contact repo to return error
				mockContactRepo.EXPECT().
					GetContactByEmail(gomock.Any(), "workspace-123", "nonexistent@example.com").
					Return(nil, errors.New("contact not found"))

				// No logging expected since this is an expected error
			},
			expectedResp:  nil,
			expectedError: "contact not found",
		},
		{
			name:        "Contact fetch database error",
			workspaceID: "workspace-123",
			email:       validEmail,
			emailHMAC:   validHMAC,
			setupMocks: func(ctrl *gomock.Controller, mockContactRepo *mocks.MockContactRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockListRepo *mocks.MockListRepository, mockLogger *pkgmocks.MockLogger) {
				// Setup workspace
				workspace := &domain.Workspace{
					ID:   "workspace-123",
					Name: "Test Workspace",
					Settings: domain.WorkspaceSettings{
						SecretKey: secretKey, // Use our test secret key
					},
				}
				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(workspace, nil)

				// Setup contact repo to return generic error
				mockContactRepo.EXPECT().
					GetContactByEmail(gomock.Any(), "workspace-123", validEmail).
					Return(nil, errors.New("database connection error"))

				// Expect error to be logged with email field
				mockLogger.EXPECT().
					WithField("email", validEmail).
					Return(mockLogger)
				mockLogger.EXPECT().
					Error(gomock.Any())
			},
			expectedResp:  nil,
			expectedError: "failed to get contact: database connection error",
		},
		{
			name:        "Lists fetch error",
			workspaceID: "workspace-123",
			email:       validEmail,
			emailHMAC:   validHMAC,
			setupMocks: func(ctrl *gomock.Controller, mockContactRepo *mocks.MockContactRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockListRepo *mocks.MockListRepository, mockLogger *pkgmocks.MockLogger) {
				// Setup workspace
				workspace := &domain.Workspace{
					ID:   "workspace-123",
					Name: "Test Workspace",
					Settings: domain.WorkspaceSettings{
						LogoURL:    "https://example.com/logo.png",
						WebsiteURL: "https://example.com",
						SecretKey:  secretKey, // Use our test secret key
					},
				}
				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(workspace, nil)

				// Setup contact
				contact := &domain.Contact{
					Email: validEmail,
					ContactLists: []*domain.ContactList{
						{
							ListID: "list-1",
							Status: domain.ContactListStatusActive,
						},
					},
				}
				mockContactRepo.EXPECT().
					GetContactByEmail(gomock.Any(), "workspace-123", validEmail).
					Return(contact, nil)

				// Setup list repo to return error
				mockListRepo.EXPECT().
					GetLists(gomock.Any(), "workspace-123").
					Return(nil, errors.New("list fetch error"))

				// Expect error to be logged
				mockLogger.EXPECT().
					Error(gomock.Any())
			},
			expectedResp: &domain.ContactPreferencesResponse{
				Contact: &domain.Contact{
					Email: validEmail,
					ContactLists: []*domain.ContactList{
						{
							ListID: "list-1",
							Status: domain.ContactListStatusActive,
						},
					},
				},
				PublicLists:  nil, // Using nil instead of empty slice to match implementation
				ContactLists: mockContactLists,
				LogoURL:      "https://example.com/logo.png",
				WebsiteURL:   "https://example.com",
			},
			expectedError: "",
		},
		{
			name:        "Empty public lists",
			workspaceID: "workspace-123",
			email:       validEmail,
			emailHMAC:   validHMAC,
			setupMocks: func(ctrl *gomock.Controller, mockContactRepo *mocks.MockContactRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockListRepo *mocks.MockListRepository, mockLogger *pkgmocks.MockLogger) {
				// Setup workspace
				workspace := &domain.Workspace{
					ID:   "workspace-123",
					Name: "Test Workspace",
					Settings: domain.WorkspaceSettings{
						LogoURL:    "https://example.com/logo.png",
						WebsiteURL: "https://example.com",
						SecretKey:  secretKey, // Use our test secret key
					},
				}
				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(workspace, nil)

				// Setup contact
				contact := &domain.Contact{
					Email: validEmail,
					ContactLists: []*domain.ContactList{
						{
							ListID: "list-1",
							Status: domain.ContactListStatusActive,
						},
					},
				}
				mockContactRepo.EXPECT().
					GetContactByEmail(gomock.Any(), "workspace-123", validEmail).
					Return(contact, nil)

				// Return only private lists
				lists := []*domain.List{
					{
						ID:       "list-1",
						Name:     "Private List 1",
						IsPublic: false,
					},
					{
						ID:       "list-2",
						Name:     "Private List 2",
						IsPublic: false,
					},
				}
				mockListRepo.EXPECT().
					GetLists(gomock.Any(), "workspace-123").
					Return(lists, nil)
			},
			expectedResp: &domain.ContactPreferencesResponse{
				Contact: &domain.Contact{
					Email: validEmail,
					ContactLists: []*domain.ContactList{
						{
							ListID: "list-1",
							Status: domain.ContactListStatusActive,
						},
					},
				},
				PublicLists:  nil, // Using nil instead of empty slice to match implementation
				ContactLists: mockContactLists,
				LogoURL:      "https://example.com/logo.png",
				WebsiteURL:   "https://example.com",
			},
			expectedError: "",
		},
	}

	// Execute test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockContactRepo := mocks.NewMockContactRepository(ctrl)
			mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			mockListRepo := mocks.NewMockListRepository(ctrl)
			mockLogger := pkgmocks.NewMockLogger(ctrl)

			tc.setupMocks(ctrl, mockContactRepo, mockWorkspaceRepo, mockListRepo, mockLogger)

			// Create service with mocks
			service := NewNotificationCenterService(
				mockContactRepo,
				mockWorkspaceRepo,
				mockListRepo,
				mockLogger,
			)

			// Call the method under test
			result, err := service.GetContactPreferences(context.Background(), tc.workspaceID, tc.email, tc.emailHMAC)

			// Verify expectations
			if tc.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			} else {
				assert.NoError(t, err)

				// For testing empty slices vs. nil slices, we'll compare specific fields
				if result != nil && tc.expectedResp != nil {
					// Compare Contact field
					assert.Equal(t, tc.expectedResp.Contact, result.Contact)

					// Compare ContactLists field
					assert.Equal(t, tc.expectedResp.ContactLists, result.ContactLists)

					// Compare LogoURL field
					assert.Equal(t, tc.expectedResp.LogoURL, result.LogoURL)

					// Compare WebsiteURL field
					assert.Equal(t, tc.expectedResp.WebsiteURL, result.WebsiteURL)

					// Special handling for PublicLists which might be nil or empty
					if len(tc.expectedResp.PublicLists) == 0 && result.PublicLists == nil {
						// Both are effectively empty, so consider them equal
					} else {
						assert.Equal(t, tc.expectedResp.PublicLists, result.PublicLists)
					}
				} else {
					assert.Equal(t, tc.expectedResp, result)
				}
			}
		})
	}
}

func TestNotificationCenterService_UpdateContactPreferences(t *testing.T) {
	secretKey := "test-secret-key"
	validEmail := "user@example.com"
	validHMAC := crypto.ComputeHMAC256([]byte(validEmail), secretKey)

	testCases := []struct {
		name          string
		req           *domain.UpdateContactPreferencesRequest
		setupMocks    func(ctrl *gomock.Controller, mockContactRepo *mocks.MockContactRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockLogger *pkgmocks.MockLogger)
		expectedError string
	}{
		{
			name: "Success with both language and timezone",
			req: &domain.UpdateContactPreferencesRequest{
				WorkspaceID: "workspace-123",
				Email:       validEmail,
				EmailHMAC:   validHMAC,
				Language:    "fr",
				Timezone:    "Europe/Paris",
			},
			setupMocks: func(ctrl *gomock.Controller, mockContactRepo *mocks.MockContactRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockLogger *pkgmocks.MockLogger) {
				workspace := &domain.Workspace{
					ID: "workspace-123",
					Settings: domain.WorkspaceSettings{
						SecretKey: secretKey,
					},
				}
				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(workspace, nil)
				mockContactRepo.EXPECT().
					UpsertContact(gomock.Any(), "workspace-123", gomock.Any()).
					DoAndReturn(func(ctx context.Context, wsID string, contact *domain.Contact) (bool, error) {
						assert.Equal(t, validEmail, contact.Email)
						assert.NotNil(t, contact.Language)
						assert.Equal(t, "fr", contact.Language.String)
						assert.NotNil(t, contact.Timezone)
						assert.Equal(t, "Europe/Paris", contact.Timezone.String)
						return false, nil
					})
			},
		},
		{
			name: "Success with language only",
			req: &domain.UpdateContactPreferencesRequest{
				WorkspaceID: "workspace-123",
				Email:       validEmail,
				EmailHMAC:   validHMAC,
				Language:    "en",
			},
			setupMocks: func(ctrl *gomock.Controller, mockContactRepo *mocks.MockContactRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockLogger *pkgmocks.MockLogger) {
				workspace := &domain.Workspace{
					ID: "workspace-123",
					Settings: domain.WorkspaceSettings{
						SecretKey: secretKey,
					},
				}
				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(workspace, nil)
				mockContactRepo.EXPECT().
					UpsertContact(gomock.Any(), "workspace-123", gomock.Any()).
					DoAndReturn(func(ctx context.Context, wsID string, contact *domain.Contact) (bool, error) {
						assert.Equal(t, validEmail, contact.Email)
						assert.NotNil(t, contact.Language)
						assert.Equal(t, "en", contact.Language.String)
						assert.Nil(t, contact.Timezone)
						return false, nil
					})
			},
		},
		{
			name: "Success with timezone only",
			req: &domain.UpdateContactPreferencesRequest{
				WorkspaceID: "workspace-123",
				Email:       validEmail,
				EmailHMAC:   validHMAC,
				Timezone:    "America/New_York",
			},
			setupMocks: func(ctrl *gomock.Controller, mockContactRepo *mocks.MockContactRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockLogger *pkgmocks.MockLogger) {
				workspace := &domain.Workspace{
					ID: "workspace-123",
					Settings: domain.WorkspaceSettings{
						SecretKey: secretKey,
					},
				}
				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(workspace, nil)
				mockContactRepo.EXPECT().
					UpsertContact(gomock.Any(), "workspace-123", gomock.Any()).
					DoAndReturn(func(ctx context.Context, wsID string, contact *domain.Contact) (bool, error) {
						assert.Equal(t, validEmail, contact.Email)
						assert.Nil(t, contact.Language)
						assert.NotNil(t, contact.Timezone)
						assert.Equal(t, "America/New_York", contact.Timezone.String)
						return false, nil
					})
			},
		},
		{
			name: "Invalid HMAC",
			req: &domain.UpdateContactPreferencesRequest{
				WorkspaceID: "workspace-123",
				Email:       validEmail,
				EmailHMAC:   "invalid-hmac",
				Language:    "fr",
			},
			setupMocks: func(ctrl *gomock.Controller, mockContactRepo *mocks.MockContactRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockLogger *pkgmocks.MockLogger) {
				workspace := &domain.Workspace{
					ID: "workspace-123",
					Settings: domain.WorkspaceSettings{
						SecretKey: secretKey,
					},
				}
				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(workspace, nil)
			},
			expectedError: "invalid email verification",
		},
		{
			name: "Workspace not found",
			req: &domain.UpdateContactPreferencesRequest{
				WorkspaceID: "nonexistent",
				Email:       validEmail,
				EmailHMAC:   validHMAC,
				Language:    "fr",
			},
			setupMocks: func(ctrl *gomock.Controller, mockContactRepo *mocks.MockContactRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockLogger *pkgmocks.MockLogger) {
				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "nonexistent").
					Return(nil, errors.New("workspace not found"))
				mockLogger.EXPECT().Error(gomock.Any())
			},
			expectedError: "failed to get workspace",
		},
		{
			name: "Upsert failure",
			req: &domain.UpdateContactPreferencesRequest{
				WorkspaceID: "workspace-123",
				Email:       validEmail,
				EmailHMAC:   validHMAC,
				Language:    "fr",
			},
			setupMocks: func(ctrl *gomock.Controller, mockContactRepo *mocks.MockContactRepository, mockWorkspaceRepo *mocks.MockWorkspaceRepository, mockLogger *pkgmocks.MockLogger) {
				workspace := &domain.Workspace{
					ID: "workspace-123",
					Settings: domain.WorkspaceSettings{
						SecretKey: secretKey,
					},
				}
				mockWorkspaceRepo.EXPECT().
					GetByID(gomock.Any(), "workspace-123").
					Return(workspace, nil)
				mockContactRepo.EXPECT().
					UpsertContact(gomock.Any(), "workspace-123", gomock.Any()).
					Return(false, errors.New("database error"))
				mockLogger.EXPECT().Error(gomock.Any())
			},
			expectedError: "failed to update contact preferences",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockContactRepo := mocks.NewMockContactRepository(ctrl)
			mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			mockListRepo := mocks.NewMockListRepository(ctrl)
			mockLogger := pkgmocks.NewMockLogger(ctrl)

			tc.setupMocks(ctrl, mockContactRepo, mockWorkspaceRepo, mockLogger)

			service := NewNotificationCenterService(
				mockContactRepo,
				mockWorkspaceRepo,
				mockListRepo,
				mockLogger,
			)

			err := service.UpdateContactPreferences(context.Background(), tc.req)

			if tc.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
