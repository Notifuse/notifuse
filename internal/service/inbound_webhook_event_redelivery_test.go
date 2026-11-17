package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A provider that gets no answer, or a server error, posts the same events again. Each event
// must get the ID it got the first time, so the redelivery stores nothing more, and two
// different events must never share one, which would lose one of them.
func TestProviderEventIDsAcrossRedeliveries(t *testing.T) {
	service := &InboundWebhookEventService{}
	const integrationID = "integration1"

	sesNotification := func(snsMessageID, message string) []byte {
		body, err := json.Marshal(domain.SESWebhookPayload{Type: "Notification", MessageID: snsMessageID, Message: message})
		require.NoError(t, err)
		return body
	}
	const sesSoftBounce = `{"eventType":"Bounce","bounce":{"bounceType":"Transient","bounceSubType":"MailboxFull","bouncedRecipients":[{"emailAddress":"test@example.com"}],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"ses-1","tags":{"notifuse_message_id":["msg-1"]}}}`
	const sesTwoRecipients = `{"eventType":"Bounce","bounce":{"bounceType":"Transient","bounceSubType":"MailboxFull","bouncedRecipients":[{"emailAddress":"copy@example.com"},{"emailAddress":"test@example.com"}],"timestamp":"2023-01-01T12:00:00Z"},"mail":{"messageId":"ses-1","tags":{"notifuse_message_id":["msg-1"]}}}`

	const mailgunTemporary = `{"event-data":{"event":"failed","id":"%s","severity":"temporary","timestamp":1672574400,"recipient":"test@example.com","reason":"greylisted","message":{"headers":{"message-id":"mg-1"}},"user-variables":{"notifuse_message_id":"msg-1"}}}`
	const postmarkBounce = `{"RecordType":"Bounce","ID":%s,"Type":"SoftBounce","TypeCode":4096,"MessageID":"pm-1","Email":"test@example.com","Details":"mailbox full","BouncedAt":"2023-01-01T12:00:00Z","Metadata":{"notifuse_message_id":"msg-1"}}`
	const postmarkComplaint = `{"RecordType":"SpamComplaint","ID":692560174,"Type":"SpamComplaint","MessageID":"pm-1","Email":"test@example.com","ComplainedAt":"2023-01-01T12:00:00Z","Metadata":{"notifuse_message_id":"msg-1"}}`
	const postmarkDelivery = `{"RecordType":"Delivery","MessageID":"pm-1","Recipient":"%s","DeliveredAt":"%s","Details":"250 OK","Metadata":{"notifuse_message_id":"msg-1"}}`
	const sparkPostBounce = `{"msys":{"message_event":{"type":"bounce","event_id":"%s","rcpt_to":"test@example.com","message_id":"sp-1","timestamp":"2023-01-01T12:00:00Z","bounce_class":"21","rcpt_meta":{"notifuse_message_id":"msg-1"}}}}`
	const mailjetBounce = `{"event":"bounce","time":%d,"MessageID":%d,"email":"test@example.com","hard_bounce":false,"CustomID":"msg-1"}`
	const smtpBounce = `{"event":"bounce","timestamp":"%s","message_id":"msg-1","recipient":"test@example.com","bounce_category":"soft"}`
	const sendGridDropped = `{"email":"test@example.com","timestamp":1672574400,"event":"dropped","reason":"Bounced Address","sg_event_id":"%s","sg_message_id":"sg-1","notifuse_message_id":"msg-1"}`

	sprintf := func(format string, args ...interface{}) []byte { return []byte(sprintfString(format, args...)) }

	cases := []struct {
		name      string
		process   func(string, []byte) ([]*domain.InboundWebhookEvent, error)
		body      []byte
		different []byte // the same email and recipient, another event of the provider
	}{
		{"SES", service.processSESWebhook, sesNotification("sns-1", sesSoftBounce), sesNotification("sns-2", sesSoftBounce)},
		{"Postmark bounce", service.processPostmarkWebhook, sprintf(postmarkBounce, "692560173"), sprintf(postmarkBounce, "692560175")},
		{"Postmark delivery", service.processPostmarkWebhook, sprintf(postmarkDelivery, "test@example.com", "2023-01-01T12:00:00Z"), sprintf(postmarkDelivery, "test@example.com", "2023-01-02T12:00:00Z")},
		{"Mailgun", service.processMailgunWebhook, sprintf(mailgunTemporary, "mg-event-1"), sprintf(mailgunTemporary, "mg-event-2")},
		{"SparkPost", service.processSparkPostWebhook, sprintf("["+sparkPostBounce+"]", "sp-event-1"), sprintf("["+sparkPostBounce+"]", "sp-event-2")},
		{"Mailjet", service.processMailjetWebhook, sprintf(mailjetBounce, 1672574400, 7), sprintf(mailjetBounce, 1672574460, 7)},
		{"SMTP", service.processSMTPWebhook, sprintf(smtpBounce, "2023-01-01T12:00:00Z"), sprintf(smtpBounce, "2023-01-01T12:05:00Z")},
		{"SendGrid", service.processSendGridWebhook, sprintf("["+sendGridDropped+"]", "sg-event-1"), sprintf("["+sendGridDropped+"]", "sg-event-2")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first, err := tc.process(integrationID, tc.body)
			require.NoError(t, err)
			require.Len(t, first, 1)
			again, err := tc.process(integrationID, tc.body)
			require.NoError(t, err)
			require.Len(t, again, 1)
			assert.Equal(t, first[0].ID, again[0].ID, "a redelivery keeps the ID")

			other, err := tc.process(integrationID, tc.different)
			require.NoError(t, err)
			require.Len(t, other, 1)
			assert.NotEqual(t, first[0].ID, other[0].ID, "another event of the same email keeps its own ID")

			fromAnother, err := tc.process("integration2", tc.body)
			require.NoError(t, err)
			assert.NotEqual(t, first[0].ID, fromAnother[0].ID, "another integration stores its own row")
		})
	}

	t.Run("SES gives each recipient of a notification its own stable ID", func(t *testing.T) {
		first, err := service.processSESWebhook(integrationID, sesNotification("sns-1", sesTwoRecipients))
		require.NoError(t, err)
		again, err := service.processSESWebhook(integrationID, sesNotification("sns-1", sesTwoRecipients))
		require.NoError(t, err)
		require.Len(t, first, 2)
		require.Len(t, again, 2)
		assert.NotEqual(t, first[0].ID, first[1].ID)
		assert.Equal(t, first[0].ID, again[0].ID)
		assert.Equal(t, first[1].ID, again[1].ID)
	})

	t.Run("a Mailjet event of another recipient gets its own ID", func(t *testing.T) {
		first, err := service.processMailjetWebhook(integrationID, sprintf(mailjetBounce, 1672574400, 7))
		require.NoError(t, err)
		other, err := service.processMailjetWebhook(integrationID, []byte(strings.Replace(sprintfString(mailjetBounce, 1672574400, 7), "test@example.com", "copy@example.com", 1)))
		require.NoError(t, err)
		assert.NotEqual(t, first[0].ID, other[0].ID)
	})

	t.Run("an address is the same whatever its case", func(t *testing.T) {
		lower, err := service.processSESWebhook(integrationID, sesNotification("sns-1", sesSoftBounce))
		require.NoError(t, err)
		upper, err := service.processSESWebhook(integrationID, sesNotification("sns-1", strings.Replace(sesSoftBounce, "test@example.com", "Test@Example.com", 1)))
		require.NoError(t, err)
		assert.Equal(t, lower[0].ID, upper[0].ID)
	})

	t.Run("a Postmark delivery without its date or its recipient gets a new ID each time", func(t *testing.T) {
		for _, body := range [][]byte{sprintf(postmarkDelivery, "test@example.com", ""), sprintf(postmarkDelivery, "", "2023-01-01T12:00:00Z")} {
			first, err := service.processPostmarkWebhook(integrationID, body)
			require.NoError(t, err)
			again, err := service.processPostmarkWebhook(integrationID, body)
			require.NoError(t, err)
			assert.NotEqual(t, first[0].ID, again[0].ID)
		}
	})

	t.Run("a Postmark delivery to another recipient of the email gets its own ID", func(t *testing.T) {
		first, err := service.processPostmarkWebhook(integrationID, sprintf(postmarkDelivery, "test@example.com", "2023-01-01T12:00:00Z"))
		require.NoError(t, err)
		other, err := service.processPostmarkWebhook(integrationID, sprintf(postmarkDelivery, "copy@example.com", "2023-01-01T12:00:00Z"))
		require.NoError(t, err)
		assert.NotEqual(t, first[0].ID, other[0].ID)
	})

	t.Run("a Postmark bounce and spam complaint with a numeric ID, as Postmark sends them, are read", func(t *testing.T) {
		big, err := service.processPostmarkWebhook(integrationID, sprintf(postmarkBounce, "4323372036854775807"))
		require.NoError(t, err)
		require.Len(t, big, 1)
		// A bounce and a complaint with the same ID stay apart
		bounces, err := service.processPostmarkWebhook(integrationID, sprintf(postmarkBounce, "692560174"))
		require.NoError(t, err)
		require.Len(t, bounces, 1)
		assert.Equal(t, domain.EmailEventBounce, bounces[0].Type)
		complaints, err := service.processPostmarkWebhook(integrationID, []byte(postmarkComplaint))
		require.NoError(t, err)
		require.Len(t, complaints, 1)
		assert.Equal(t, domain.EmailEventComplaint, complaints[0].Type)
		assert.NotEqual(t, bounces[0].ID, complaints[0].ID)
	})

	t.Run("a batch posted again with one more event keeps the IDs of the first ones", func(t *testing.T) {
		for name, c := range map[string]struct {
			process func(string, []byte) ([]*domain.InboundWebhookEvent, error)
			one     string
			two     string
		}{
			"SendGrid":  {service.processSendGridWebhook, sprintfString(sendGridDropped, "sg-event-1"), sprintfString(sendGridDropped, "sg-event-2")},
			"SparkPost": {service.processSparkPostWebhook, sprintfString(sparkPostBounce, "sp-event-1"), sprintfString(sparkPostBounce, "sp-event-2")},
			"Mailjet":   {service.processMailjetWebhook, sprintfString(mailjetBounce, 1672574400, 7), sprintfString(mailjetBounce, 1672574400, 8)},
		} {
			t.Run(name, func(t *testing.T) {
				first, err := c.process(integrationID, []byte("["+c.one+"]"))
				require.NoError(t, err)
				both, err := c.process(integrationID, []byte("["+c.two+","+c.one+"]"))
				require.NoError(t, err)
				require.Len(t, both, 2)
				assert.Equal(t, first[0].ID, both[1].ID)
				assert.NotEqual(t, both[0].ID, both[1].ID)
			})
		}
	})

	// Without its identifier an event cannot be told from another one, so it is never
	// merged with one: each parse stores it, as before
	t.Run("an event without the provider's identifier gets a new ID each time", func(t *testing.T) {
		for name, c := range map[string]struct {
			process func(string, []byte) ([]*domain.InboundWebhookEvent, error)
			body    []byte
		}{
			"SES without the SNS MessageId": {service.processSESWebhook, sesNotification("", sesSoftBounce)},
			"Postmark bounce without ID":    {service.processPostmarkWebhook, sprintf(postmarkBounce, `""`)},
			"Mailgun without id":            {service.processMailgunWebhook, sprintf(mailgunTemporary, "")},
			"SparkPost without event_id":    {service.processSparkPostWebhook, sprintf("["+sparkPostBounce+"]", "")},
			"Mailjet without time":          {service.processMailjetWebhook, sprintf(mailjetBounce, 0, 7)},
			"Mailjet without MessageID":     {service.processMailjetWebhook, sprintf(mailjetBounce, 1672574400, 0)},
			"SMTP without timestamp":        {service.processSMTPWebhook, sprintf(smtpBounce, "")},
			"SendGrid without sg_event_id":  {service.processSendGridWebhook, sprintf("["+sendGridDropped+"]", "")},
		} {
			t.Run(name, func(t *testing.T) {
				first, err := c.process(integrationID, c.body)
				require.NoError(t, err)
				again, err := c.process(integrationID, c.body)
				require.NoError(t, err)
				require.Len(t, first, 1)
				require.Len(t, again, 1)
				assert.NotEqual(t, first[0].ID, again[0].ID)
			})
		}
	})
}

func sprintfString(format string, args ...interface{}) string { return fmt.Sprintf(format, args...) }
