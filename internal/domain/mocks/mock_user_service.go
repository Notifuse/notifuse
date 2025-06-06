// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/Notifuse/notifuse/internal/domain (interfaces: UserServiceInterface)

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	domain "github.com/Notifuse/notifuse/internal/domain"
	gomock "github.com/golang/mock/gomock"
)

// MockUserServiceInterface is a mock of UserServiceInterface interface.
type MockUserServiceInterface struct {
	ctrl     *gomock.Controller
	recorder *MockUserServiceInterfaceMockRecorder
}

// MockUserServiceInterfaceMockRecorder is the mock recorder for MockUserServiceInterface.
type MockUserServiceInterfaceMockRecorder struct {
	mock *MockUserServiceInterface
}

// NewMockUserServiceInterface creates a new mock instance.
func NewMockUserServiceInterface(ctrl *gomock.Controller) *MockUserServiceInterface {
	mock := &MockUserServiceInterface{ctrl: ctrl}
	mock.recorder = &MockUserServiceInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockUserServiceInterface) EXPECT() *MockUserServiceInterfaceMockRecorder {
	return m.recorder
}

// GetUserByEmail mocks base method.
func (m *MockUserServiceInterface) GetUserByEmail(arg0 context.Context, arg1 string) (*domain.User, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetUserByEmail", arg0, arg1)
	ret0, _ := ret[0].(*domain.User)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetUserByEmail indicates an expected call of GetUserByEmail.
func (mr *MockUserServiceInterfaceMockRecorder) GetUserByEmail(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetUserByEmail", reflect.TypeOf((*MockUserServiceInterface)(nil).GetUserByEmail), arg0, arg1)
}

// GetUserByID mocks base method.
func (m *MockUserServiceInterface) GetUserByID(arg0 context.Context, arg1 string) (*domain.User, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetUserByID", arg0, arg1)
	ret0, _ := ret[0].(*domain.User)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetUserByID indicates an expected call of GetUserByID.
func (mr *MockUserServiceInterfaceMockRecorder) GetUserByID(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetUserByID", reflect.TypeOf((*MockUserServiceInterface)(nil).GetUserByID), arg0, arg1)
}

// SignIn mocks base method.
func (m *MockUserServiceInterface) SignIn(arg0 context.Context, arg1 domain.SignInInput) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SignIn", arg0, arg1)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SignIn indicates an expected call of SignIn.
func (mr *MockUserServiceInterfaceMockRecorder) SignIn(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SignIn", reflect.TypeOf((*MockUserServiceInterface)(nil).SignIn), arg0, arg1)
}

// VerifyCode mocks base method.
func (m *MockUserServiceInterface) VerifyCode(arg0 context.Context, arg1 domain.VerifyCodeInput) (*domain.AuthResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "VerifyCode", arg0, arg1)
	ret0, _ := ret[0].(*domain.AuthResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// VerifyCode indicates an expected call of VerifyCode.
func (mr *MockUserServiceInterfaceMockRecorder) VerifyCode(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "VerifyCode", reflect.TypeOf((*MockUserServiceInterface)(nil).VerifyCode), arg0, arg1)
}

// VerifyUserSession mocks base method.
func (m *MockUserServiceInterface) VerifyUserSession(arg0 context.Context, arg1, arg2 string) (*domain.User, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "VerifyUserSession", arg0, arg1, arg2)
	ret0, _ := ret[0].(*domain.User)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// VerifyUserSession indicates an expected call of VerifyUserSession.
func (mr *MockUserServiceInterfaceMockRecorder) VerifyUserSession(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "VerifyUserSession", reflect.TypeOf((*MockUserServiceInterface)(nil).VerifyUserSession), arg0, arg1, arg2)
}
