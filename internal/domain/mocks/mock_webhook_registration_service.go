// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/Notifuse/notifuse/internal/domain (interfaces: WebhookRegistrationService)

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	domain "github.com/Notifuse/notifuse/internal/domain"
	gomock "github.com/golang/mock/gomock"
)

// MockWebhookRegistrationService is a mock of WebhookRegistrationService interface.
type MockWebhookRegistrationService struct {
	ctrl     *gomock.Controller
	recorder *MockWebhookRegistrationServiceMockRecorder
}

// MockWebhookRegistrationServiceMockRecorder is the mock recorder for MockWebhookRegistrationService.
type MockWebhookRegistrationServiceMockRecorder struct {
	mock *MockWebhookRegistrationService
}

// NewMockWebhookRegistrationService creates a new mock instance.
func NewMockWebhookRegistrationService(ctrl *gomock.Controller) *MockWebhookRegistrationService {
	mock := &MockWebhookRegistrationService{ctrl: ctrl}
	mock.recorder = &MockWebhookRegistrationServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockWebhookRegistrationService) EXPECT() *MockWebhookRegistrationServiceMockRecorder {
	return m.recorder
}

// GetWebhookStatus mocks base method.
func (m *MockWebhookRegistrationService) GetWebhookStatus(arg0 context.Context, arg1, arg2 string) (*domain.WebhookRegistrationStatus, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetWebhookStatus", arg0, arg1, arg2)
	ret0, _ := ret[0].(*domain.WebhookRegistrationStatus)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetWebhookStatus indicates an expected call of GetWebhookStatus.
func (mr *MockWebhookRegistrationServiceMockRecorder) GetWebhookStatus(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetWebhookStatus", reflect.TypeOf((*MockWebhookRegistrationService)(nil).GetWebhookStatus), arg0, arg1, arg2)
}

// RegisterWebhooks mocks base method.
func (m *MockWebhookRegistrationService) RegisterWebhooks(arg0 context.Context, arg1 string, arg2 *domain.WebhookRegistrationConfig) (*domain.WebhookRegistrationStatus, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RegisterWebhooks", arg0, arg1, arg2)
	ret0, _ := ret[0].(*domain.WebhookRegistrationStatus)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// RegisterWebhooks indicates an expected call of RegisterWebhooks.
func (mr *MockWebhookRegistrationServiceMockRecorder) RegisterWebhooks(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RegisterWebhooks", reflect.TypeOf((*MockWebhookRegistrationService)(nil).RegisterWebhooks), arg0, arg1, arg2)
}

// UnregisterWebhooks mocks base method.
func (m *MockWebhookRegistrationService) UnregisterWebhooks(arg0 context.Context, arg1, arg2 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UnregisterWebhooks", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// UnregisterWebhooks indicates an expected call of UnregisterWebhooks.
func (mr *MockWebhookRegistrationServiceMockRecorder) UnregisterWebhooks(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UnregisterWebhooks", reflect.TypeOf((*MockWebhookRegistrationService)(nil).UnregisterWebhooks), arg0, arg1, arg2)
}
