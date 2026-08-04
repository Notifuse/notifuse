package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	credentialsv2 "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturingHTTPClient stands in for the network: it records the request the AWS SDK actually
// produced and returns a canned SES response.
type capturingHTTPClient struct {
	req  *http.Request
	body []byte
}

func (c *capturingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.req = req
	if req.Body != nil {
		c.body, _ = io.ReadAll(req.Body)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"MessageId":"0100018f-wire-test"}`))),
		Request:    req,
	}, nil
}

// newWireCapturingService builds the service with a REAL sesv2.Client whose transport is
// intercepted.
//
// This is the closest thing to a live send that can run without an AWS account: unlike a gomock
// client, it exercises the SDK's own input validation, JSON serialisation, endpoint resolution
// and SigV4 signing. A mock will happily accept a request that SES would reject as malformed;
// this will not.
//
// What it still cannot prove is SES's *semantic* acceptance — whether the account may send from
// that address, and whether SES parses a friendly-from the way the v1 API did. Those need a real
// account (see the manual verification steps in the plan).
func newWireCapturingService(t *testing.T) (*SESService, *capturingHTTPClient) {
	t.Helper()

	ctrl := gomock.NewController(t)
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()
	logger.EXPECT().Warn(gomock.Any()).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Debug(gomock.Any()).AnyTimes()

	capture := &capturingHTTPClient{}
	service := NewSESService(nil, logger)
	service.sesV2ClientFactory = func(config domain.AmazonSESSettings) domain.SESv2Client {
		return sesv2.NewFromConfig(awsv2.Config{
			Region: config.Region,
			Credentials: credentialsv2.NewStaticCredentialsProvider(
				config.AccessKey, config.SecretKey, ""),
			HTTPClient:       capture,
			RetryMaxAttempts: 1,
		})
	}
	return service, capture
}

func wireSettings() *domain.AmazonSESSettings {
	return &domain.AmazonSESSettings{
		AccessKey:               "AKIAIOSFODNN7EXAMPLE",
		SecretKey:               "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Region:                  "eu-west-3",
		ManagedConfigurationSet: "notifuse-int-1",
		ManagedTenantName:       "notifuse-int-1",
		TenantIsolationEnabled:  true,
	}
}

func TestSESWire_SimpleSendSerialisesCorrectly(t *testing.T) {
	service, capture := newWireCapturingService(t)

	request := tenantTestRequest(&domain.EmailProvider{SES: wireSettings()})
	request.FromName = "Acme Mail"
	request.EmailOptions = domain.EmailOptions{
		CC:      []string{"cc@example.com"},
		BCC:     []string{"bcc@example.com"},
		ReplyTo: "reply@example.com",
	}

	require.NoError(t, service.SendEmail(context.Background(), request))
	require.NotNil(t, capture.req, "the SDK must have produced a request")

	// Endpoint resolution and signing, which a mock never exercises.
	assert.Equal(t, "email.eu-west-3.amazonaws.com", capture.req.URL.Host)
	assert.Equal(t, "/v2/email/outbound-emails", capture.req.URL.Path)
	assert.Equal(t, http.MethodPost, capture.req.Method)
	assert.Contains(t, capture.req.Header.Get("Authorization"), "AWS4-HMAC-SHA256")
	assert.Contains(t, capture.req.Header.Get("Authorization"), "/eu-west-3/ses/aws4_request")

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(capture.body, &body))

	assert.Equal(t, "Acme Mail <from@example.com>", body["FromEmailAddress"])
	assert.Equal(t, "notifuse-int-1", body["ConfigurationSetName"])
	assert.Equal(t, "notifuse-int-1", body["TenantName"])
	assert.Equal(t, []interface{}{"reply@example.com"}, body["ReplyToAddresses"])

	destination := body["Destination"].(map[string]interface{})
	assert.Equal(t, []interface{}{"to@example.com"}, destination["ToAddresses"])
	assert.Equal(t, []interface{}{"cc@example.com"}, destination["CcAddresses"])
	assert.Equal(t, []interface{}{"bcc@example.com"}, destination["BccAddresses"])

	simple := body["Content"].(map[string]interface{})["Simple"].(map[string]interface{})
	assert.Equal(t, "Subject", simple["Subject"].(map[string]interface{})["Data"])
	assert.Equal(t, "<p>hi</p>", simple["Body"].(map[string]interface{})["Html"].(map[string]interface{})["Data"])

	tags := body["EmailTags"].([]interface{})
	require.Len(t, tags, 1)
	assert.Equal(t, "notifuse_message_id", tags[0].(map[string]interface{})["Name"])
}

func TestSESWire_RawSendSerialisesCorrectly(t *testing.T) {
	service, capture := newWireCapturingService(t)

	request := tenantTestRequest(&domain.EmailProvider{SES: wireSettings()})
	request.EmailOptions = domain.EmailOptions{
		CC:  []string{"cc@example.com"},
		BCC: []string{"bcc@example.com"},
		Attachments: []domain.Attachment{{
			Filename:    "note.txt",
			Content:     "aGVsbG8=",
			ContentType: "text/plain",
		}},
	}

	require.NoError(t, service.SendEmail(context.Background(), request))
	require.NotNil(t, capture.req)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(capture.body, &body))

	content := body["Content"].(map[string]interface{})
	require.Contains(t, content, "Raw", "attachments must go through the raw path")
	assert.NotContains(t, content, "Simple")

	// The raw path deliberately omits FromEmailAddress so the envelope sender comes from the
	// MIME From header, exactly as the v1 SendRawEmail path behaved.
	assert.NotContains(t, body, "FromEmailAddress")

	// Every recipient class must be in the envelope; CC used to be missing here.
	destination := body["Destination"].(map[string]interface{})
	assert.Equal(t, []interface{}{"to@example.com"}, destination["ToAddresses"])
	assert.Equal(t, []interface{}{"cc@example.com"}, destination["CcAddresses"])
	assert.Equal(t, []interface{}{"bcc@example.com"}, destination["BccAddresses"])

	assert.Equal(t, "notifuse-int-1", body["TenantName"])
	assert.Equal(t, "notifuse-int-1", body["ConfigurationSetName"])
}

// TestSESWire_EncodedFromNameSurvivesSerialisation pins the friendly-from format on the field
// that carries every outbound message. v1 put an RFC 2047 encoded display name into Source; the
// v2 equivalent must accept the same string shape.
func TestSESWire_EncodedFromNameSurvivesSerialisation(t *testing.T) {
	service, capture := newWireCapturingService(t)

	request := tenantTestRequest(&domain.EmailProvider{SES: wireSettings()})
	request.FromName = "Ünïcode Sender"

	require.NoError(t, service.SendEmail(context.Background(), request))

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(capture.body, &body))

	from := body["FromEmailAddress"].(string)
	assert.Contains(t, from, "=?UTF-8?b?", "non-ASCII display names must be RFC 2047 encoded")
	assert.Contains(t, from, "<from@example.com>")
}

// TestSESWire_RegionsResolve covers every region the console offers, including GovCloud, whose
// endpoints follow a different pattern.
func TestSESWire_RegionsResolve(t *testing.T) {
	for _, tc := range []struct{ region, host string }{
		{"us-east-1", "email.us-east-1.amazonaws.com"},
		{"eu-west-3", "email.eu-west-3.amazonaws.com"},
		{"ap-southeast-1", "email.ap-southeast-1.amazonaws.com"},
		{"us-gov-west-1", "email.us-gov-west-1.amazonaws.com"},
	} {
		t.Run(tc.region, func(t *testing.T) {
			service, capture := newWireCapturingService(t)

			settings := wireSettings()
			settings.Region = tc.region

			require.NoError(t, service.SendEmail(context.Background(),
				tenantTestRequest(&domain.EmailProvider{SES: settings})))

			require.NotNil(t, capture.req)
			assert.Equal(t, tc.host, capture.req.URL.Host)
		})
	}
}

// TestSESWire_TenantOmittedWhenUnset proves the field is absent from the wire payload rather
// than present-and-empty, which SES rejects.
func TestSESWire_TenantOmittedWhenUnset(t *testing.T) {
	service, capture := newWireCapturingService(t)

	settings := wireSettings()
	settings.ManagedTenantName = ""

	require.NoError(t, service.SendEmail(context.Background(),
		tenantTestRequest(&domain.EmailProvider{SES: settings})))

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(capture.body, &body))
	assert.NotContains(t, body, "TenantName")
}
