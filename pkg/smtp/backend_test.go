package smtp

import (
	"errors"
	"strings"
	"testing"

	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/emersion/go-smtp"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBackend(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	authenticator := func(username, password string) (string, error) {
		return "workspace123", nil
	}
	handler := func(workspaceID string, from string, to []string, data []byte) error {
		return nil
	}

	backend := NewBackend(authenticator, handler, mockLogger)

	require.NotNil(t, backend)
	assert.NotNil(t, backend.authenticator)
	assert.NotNil(t, backend.handler)
	assert.Equal(t, mockLogger, backend.logger)
}

func TestBackend_NewSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	backend := NewBackend(nil, nil, mockLogger)

	conn := &smtp.Conn{}
	session, err := backend.NewSession(conn)

	require.NoError(t, err)
	require.NotNil(t, session)
	assert.IsType(t, &Session{}, session)
}

func TestSession_AuthPlain(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	t.Run("successful authentication", func(t *testing.T) {
		authenticator := func(username, password string) (string, error) {
			return "workspace123", nil
		}
		backend := NewBackend(authenticator, nil, mockLogger)
		session := &Session{backend: backend, logger: mockLogger}

		err := session.AuthPlain("user", "pass")
		assert.NoError(t, err)
		assert.Equal(t, "workspace123", session.workspaceID)
	})

	t.Run("failed authentication", func(t *testing.T) {
		authenticator := func(username, password string) (string, error) {
			return "", errors.New("invalid credentials")
		}
		backend := NewBackend(authenticator, nil, mockLogger)
		session := &Session{backend: backend, logger: mockLogger}

		err := session.AuthPlain("user", "pass")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid credentials")
		assert.Empty(t, session.workspaceID)
	})
}

func TestSession_Mail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	t.Run("authenticated session", func(t *testing.T) {
		backend := NewBackend(nil, nil, mockLogger)
		session := &Session{
			backend:     backend,
			logger:      mockLogger,
			workspaceID: "workspace123",
		}

		err := session.Mail("sender@example.com", nil)
		assert.NoError(t, err)
		assert.Equal(t, "sender@example.com", session.from)
	})

	t.Run("unauthenticated session", func(t *testing.T) {
		backend := NewBackend(nil, nil, mockLogger)
		session := &Session{
			backend: backend,
			logger:  mockLogger,
		}

		err := session.Mail("sender@example.com", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not authenticated")
	})
}

func TestSession_Rcpt(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	t.Run("authenticated session", func(t *testing.T) {
		backend := NewBackend(nil, nil, mockLogger)
		session := &Session{
			backend:     backend,
			logger:      mockLogger,
			workspaceID: "workspace123",
			to:          []string{},
		}

		err := session.Rcpt("recipient@example.com", nil)
		assert.NoError(t, err)
		assert.Len(t, session.to, 1)
		assert.Equal(t, "recipient@example.com", session.to[0])
	})

	t.Run("multiple recipients", func(t *testing.T) {
		backend := NewBackend(nil, nil, mockLogger)
		session := &Session{
			backend:     backend,
			logger:      mockLogger,
			workspaceID: "workspace123",
			to:          []string{},
		}

		err1 := session.Rcpt("recipient1@example.com", nil)
		err2 := session.Rcpt("recipient2@example.com", nil)

		assert.NoError(t, err1)
		assert.NoError(t, err2)
		assert.Len(t, session.to, 2)
		assert.Equal(t, "recipient1@example.com", session.to[0])
		assert.Equal(t, "recipient2@example.com", session.to[1])
	})

	t.Run("unauthenticated session", func(t *testing.T) {
		backend := NewBackend(nil, nil, mockLogger)
		session := &Session{
			backend: backend,
			logger:  mockLogger,
		}

		err := session.Rcpt("recipient@example.com", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not authenticated")
	})
}

func TestSession_Data(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	t.Run("authenticated session", func(t *testing.T) {
		messageData := []byte("Subject: Test\n\nBody")
		handlerCalled := false
		handler := func(workspaceID string, from string, to []string, data []byte) error {
			handlerCalled = true
			assert.Equal(t, "workspace123", workspaceID)
			assert.Equal(t, "sender@example.com", from)
			assert.Len(t, to, 1)
			assert.Equal(t, messageData, data)
			return nil
		}

		backend := NewBackend(nil, handler, mockLogger)
		session := &Session{
			backend:     backend,
			logger:      mockLogger,
			workspaceID: "workspace123",
			from:        "sender@example.com",
			to:          []string{"recipient@example.com"},
		}

		reader := strings.NewReader(string(messageData))
		err := session.Data(reader)
		assert.NoError(t, err)
		assert.True(t, handlerCalled)
	})

	t.Run("unauthenticated session", func(t *testing.T) {
		backend := NewBackend(nil, nil, mockLogger)
		session := &Session{
			backend: backend,
			logger:  mockLogger,
		}

		reader := strings.NewReader("test")
		err := session.Data(reader)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not authenticated")
	})

	t.Run("handler returns error", func(t *testing.T) {
		handler := func(workspaceID string, from string, to []string, data []byte) error {
			return errors.New("handler error")
		}

		backend := NewBackend(nil, handler, mockLogger)
		session := &Session{
			backend:     backend,
			logger:      mockLogger,
			workspaceID: "workspace123",
			from:        "sender@example.com",
			to:          []string{"recipient@example.com"},
		}

		reader := strings.NewReader("test")
		err := session.Data(reader)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "handler error")
	})

	t.Run("reader error", func(t *testing.T) {
		backend := NewBackend(nil, nil, mockLogger)
		session := &Session{
			backend:     backend,
			logger:      mockLogger,
			workspaceID: "workspace123",
		}

		// Create a reader that will fail
		reader := &errorReader{err: errors.New("read error")}
		err := session.Data(reader)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read message")
	})
}

type errorReader struct {
	err error
}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, e.err
}

func TestSession_Reset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	backend := NewBackend(nil, nil, mockLogger)
	session := &Session{
		backend: backend,
		logger:  mockLogger,
		from:    "sender@example.com",
		to:      []string{"recipient@example.com"},
	}

	session.Reset()

	assert.Empty(t, session.from)
	assert.Nil(t, session.to)
}

func TestSession_Reset_MultipleTimes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	backend := NewBackend(nil, nil, mockLogger)
	session := &Session{
		backend: backend,
		logger:  mockLogger,
		from:    "sender@example.com",
		to:      []string{"recipient@example.com"},
	}

	session.Reset()
	session.Reset() // Should be safe to call multiple times

	assert.Empty(t, session.from)
	assert.Nil(t, session.to)
}

func TestSession_Logout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	backend := NewBackend(nil, nil, mockLogger)
	session := &Session{
		backend: backend,
		logger:  mockLogger,
	}

	err := session.Logout()
	assert.NoError(t, err)
}

func TestSession_Logout_MultipleTimes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	backend := NewBackend(nil, nil, mockLogger)
	session := &Session{
		backend: backend,
		logger:  mockLogger,
	}

	err1 := session.Logout()
	err2 := session.Logout()

	assert.NoError(t, err1)
	assert.NoError(t, err2)
}
