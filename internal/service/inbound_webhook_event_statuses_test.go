package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ProcessWebhook tells each kind of failure apart, so the handler answers 404, 400 or 500.
func TestProcessWebhook_FailureKinds(t *testing.T) {
	const workspaceID, integrationID = "ws1", "int1"
	newService := func(t *testing.T, kind domain.EmailProviderKind, transport http.RoundTripper) *InboundWebhookEventService {
		ctrl := gomock.NewController(t)
		log := pkgmocks.NewMockLogger(ctrl)
		log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().Info(gomock.Any()).AnyTimes()
		log.EXPECT().Error(gomock.Any()).AnyTimes()
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{ID: workspaceID,
			Integrations: []domain.Integration{{ID: integrationID, Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{Kind: kind}}}}, nil)
		svc := &InboundWebhookEventService{logger: log, workspaceRepo: workspaceRepo}
		if transport != nil {
			svc.httpClient = &http.Client{Transport: transport}
		}
		return svc
	}
	confirmation := func(t *testing.T, subscribeURL string) []byte {
		body, err := json.Marshal(domain.SESWebhookPayload{Type: "SubscriptionConfirmation",
			TopicARN: "arn:aws:sns:us-east-1:123456789:topic", SubscribeURL: subscribeURL})
		require.NoError(t, err)
		return body
	}
	const awsURL = "https://sns.us-east-1.amazonaws.com/?Action=ConfirmSubscription&Token=t"
	answer := func(status int, err error) http.RoundTripper {
		return roundTripFunc(func(*http.Request) (*http.Response, error) {
			if err != nil {
				return nil, err
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
		})
	}

	t.Run("an unknown workspace", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(nil, &domain.ErrWorkspaceNotFound{WorkspaceID: workspaceID})
		svc := &InboundWebhookEventService{workspaceRepo: workspaceRepo}

		err := svc.ProcessWebhook(context.Background(), workspaceID, integrationID, []byte(`{}`))
		var notFound *domain.ErrWorkspaceNotFound
		assert.True(t, errors.As(err, &notFound), "got %v", err)
	})

	t.Run("an unknown integration", func(t *testing.T) {
		svc := newService(t, domain.EmailProviderKindPostmark, nil)
		err := svc.ProcessWebhook(context.Background(), workspaceID, "another", []byte(`{}`))
		assert.ErrorIs(t, err, domain.ErrInboundIntegrationNotFound)
	})

	t.Run("an integration of a kind that sends no events", func(t *testing.T) {
		svc := newService(t, domain.EmailProviderKind("unknown"), nil)
		err := svc.ProcessWebhook(context.Background(), workspaceID, integrationID, []byte(`{}`))
		assert.ErrorIs(t, err, domain.ErrWebhookProviderUnsupported)
	})

	t.Run("a payload the parser refuses", func(t *testing.T) {
		svc := newService(t, domain.EmailProviderKindPostmark, nil)
		err := svc.ProcessWebhook(context.Background(), workspaceID, integrationID, []byte(`{not json`))
		assert.ErrorIs(t, err, domain.ErrWebhookPayloadInvalid)
	})

	t.Run("an SNS confirmation whose URL is not AWS's", func(t *testing.T) {
		svc := newService(t, domain.EmailProviderKindSES, answer(http.StatusOK, nil))
		// The sender's mistake: logged at Warn, never at Error (the strict mock refuses it).
		log := pkgmocks.NewMockLogger(gomock.NewController(t))
		log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().Info(gomock.Any()).AnyTimes()
		log.EXPECT().Warn("Refused the subscription confirmation URL").Times(1)
		svc.logger = log
		err := svc.ProcessWebhook(context.Background(), workspaceID, integrationID, confirmation(t, "https://attacker.example/confirm"))
		assert.ErrorIs(t, err, domain.ErrWebhookPayloadInvalid)
	})

	t.Run("an SNS confirmation AWS cannot be reached for", func(t *testing.T) {
		svc := newService(t, domain.EmailProviderKindSES, answer(0, errors.New("dial tcp: i/o timeout")))
		log := pkgmocks.NewMockLogger(gomock.NewController(t))
		log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().Info(gomock.Any()).AnyTimes()
		log.EXPECT().Error("Failed to confirm subscription").Times(1)
		svc.logger = log
		err := svc.ProcessWebhook(context.Background(), workspaceID, integrationID, confirmation(t, awsURL))
		require.Error(t, err)
		assert.NotErrorIs(t, err, domain.ErrWebhookPayloadInvalid, "ours: SNS sends the confirmation again")
	})

	t.Run("an SNS confirmation AWS refuses", func(t *testing.T) {
		svc := newService(t, domain.EmailProviderKindSES, answer(http.StatusServiceUnavailable, nil))
		err := svc.ProcessWebhook(context.Background(), workspaceID, integrationID, confirmation(t, awsURL))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "503")
		assert.NotErrorIs(t, err, domain.ErrWebhookPayloadInvalid, "ours: SNS sends the confirmation again")
	})

	t.Run("an SNS confirmation AWS accepts", func(t *testing.T) {
		svc := newService(t, domain.EmailProviderKindSES, answer(http.StatusOK, nil))
		require.NoError(t, svc.ProcessWebhook(context.Background(), workspaceID, integrationID, confirmation(t, awsURL)))
	})
}
