package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The seed's clock is the one it is given, not the moment the event is drawn.
func TestDemoEventAfter_StaysInThePast(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 1000; i++ {
		after := now.Add(-time.Hour)
		at := demoEventAfter(now, after, 48*time.Hour)
		assert.False(t, at.Before(after), "before the moment it follows")
		assert.False(t, at.After(now), "after the seed's now")
	}
	// Room enough: the whole window is used.
	old := now.Add(-30 * 24 * time.Hour)
	at := demoEventAfter(now, old, time.Hour)
	assert.False(t, at.Before(old))
	assert.True(t, at.Before(old.Add(time.Hour)))
	// After now already: now.
	assert.Equal(t, now, demoEventAfter(now, now.Add(time.Hour), time.Hour))
}

// The newest newsletter is a day old, while its opens fall within a week of delivery and its
// complaints within two days: none of them may be dated after the seed.
func TestDemoMessageHistory_NoEventInTheFuture(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}
	contact := &domain.Contact{Email: "ana@example.com"}
	broadcast := &domain.Broadcast{ID: "bc-1"}
	now := time.Now()
	complaints := 0
	check := func(label string, message *domain.MessageHistory, engagement messageEngagement) {
		for name, at := range map[string]*time.Time{
			"sent": &message.SentAt, "delivered": &engagement.deliveredTime, "bounced": &engagement.bouncedTime,
			"opened": &engagement.openedTime, "clicked": &engagement.clickedTime, "complained": &engagement.complainedTime,
			"unsubscribed": message.UnsubscribedAt,
		} {
			if at != nil {
				require.False(t, at.After(now), "%s: %s at %s, after %s", label, name, at, now)
			}
		}
	}
	for i := 0; i < 20000; i++ {
		message, engagement := svc.generateMessageHistoryForContact(contact, "newsletter-weekly", 1, broadcast, now.AddDate(0, 0, -1), now)
		check("newsletter", message, engagement)
		if engagement.shouldComplain {
			complaints++
		}
		message, engagement = svc.generateTransactionalMessageHistoryForContact(contact, "welcome-email", 1, "welcome", now.Add(-2*time.Hour), now)
		check("transactional", message, engagement)
	}
	require.Positive(t, complaints, "no complaint drawn: the check above saw none")
}

// A broadcast's message stores the list it went to, as a real send does: its complaint or hard
// bounce then sets the contact's status on that list (apply_message_status_to_contact_lists).
func TestDemoBroadcastMessages_CarryTheirList(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}
	now := time.Now()
	message, _ := svc.generateMessageHistoryForContact(&domain.Contact{Email: "ana@example.com"}, "newsletter-weekly", 1, &domain.Broadcast{ID: "bc-1"}, now.AddDate(0, 0, -3), now)
	require.NotNil(t, message.ListID)
	assert.Equal(t, demoListNewsletter, *message.ListID)
}

// A broadcast's message carries the UTM parameters its links are tagged with: the broadcast's
// campaign name, which the seeded web sessions share, not its ID.
func TestDemoBroadcastMessages_CarryTheirCampaign(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}
	now := time.Now()
	broadcast := &domain.Broadcast{ID: "6f1c2a3b-uuid", UTMParameters: &domain.UTMParameters{Campaign: "weekly_newsletter_2"}}
	message, _ := svc.generateMessageHistoryForContact(&domain.Contact{Email: "ana@example.com"}, "newsletter-weekly-v2", 1, broadcast, now.AddDate(0, 0, -3), now)
	assert.Equal(t, "weekly_newsletter_2", message.MessageData.Data["utm_campaign"])
	assert.Equal(t, demoBroadcastUTMSource, message.MessageData.Data["utm_source"])
	assert.Equal(t, "email", message.MessageData.Data["utm_medium"])
	require.NotNil(t, message.BroadcastID)
	assert.Equal(t, "6f1c2a3b-uuid", *message.BroadcastID)
}

// After a hard bounce nothing more reaches the address, and after a spam complaint the later
// newsletters skip it: a contact who bounced on an old newsletter is not shown opening a newer one.
func TestDemoContactMessages_StopAfterABounceOrAComplaint(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}
	now := time.Now()
	campaigns := demoTestCampaigns(t, demoTestBroadcasts(now))
	ok := func(*domain.MessageHistory) error { return nil }

	var cutByBounce, cutByComplaint int
	for i := 0; i < 20000; i++ {
		contact := &domain.Contact{Email: fmt.Sprintf("c%d@example.com", i)}
		sent := svc.demoContactMessages(contact, campaigns, now, ok)
		require.NotEmpty(t, sent)
		for j := 1; j < len(sent); j++ {
			assert.False(t, sent[j].message.SentAt.Before(sent[j-1].message.SentAt), "sent oldest first")
		}
		for j, data := range sent {
			later := sent[j+1:]
			if data.engagement.shouldBounce {
				require.Empty(t, later, "%s received an email after a hard bounce", contact.Email)
			}
			if data.engagement.shouldComplain {
				for _, next := range later {
					require.Nil(t, next.message.BroadcastID, "%s received a newsletter after a complaint", contact.Email)
				}
			}
		}
		// Count the contacts whose emails were really cut short, so the test proves it saw some.
		if n := len(sent); n > 0 && n < 2 && sent[n-1].engagement.shouldBounce {
			cutByBounce++
		}
		for _, data := range sent {
			if data.engagement.shouldComplain && data.message.SentAt.Before(time.Now().AddDate(0, 0, -2)) {
				cutByComplaint++
			}
		}
	}
	assert.Positive(t, cutByBounce, "no contact bounced before its second email")
	assert.Positive(t, cutByComplaint, "no complaint on an older newsletter")
}

func TestDemoContactMessages_LeaveOutWhatCreateRefuses(t *testing.T) {
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}
	refuse := func(*domain.MessageHistory) error { return errors.New("insert failed") }
	assert.Empty(t, svc.demoContactMessages(&domain.Contact{Email: "ana@example.com"}, demoTransactionalCampaigns, time.Now(), refuse))
}

func TestDemoStillReachable(t *testing.T) {
	newsletter := demoCampaignTemplate{broadcast: &domain.Broadcast{ID: "bc-1"}}
	transactional := demoCampaignTemplate{}
	assert.True(t, demoStillReachable(newsletter, false, false))
	assert.False(t, demoStillReachable(newsletter, true, false))
	assert.False(t, demoStillReachable(transactional, true, false))
	assert.False(t, demoStillReachable(newsletter, false, true))
	assert.True(t, demoStillReachable(transactional, false, true))
}
