package domain

import "testing"

func TestClassify(t *testing.T) {
	testCases := []struct {
		name  string
		reply *InboundReply
		want  ReplyClass
	}{
		{
			name:  "nil is droppable",
			reply: nil,
			want:  ReplyBounce,
		},
		{
			name:  "plain human reply is genuine",
			reply: &InboundReply{FromEmail: "jane@example.com", Subject: "Re: Welcome"},
			want:  ReplyGenuine,
		},
		{
			name:  "bounce via multipart/report content-type",
			reply: &InboundReply{FromEmail: "jane@example.com", ContentType: "multipart/report; report-type=delivery-status; boundary=x"},
			want:  ReplyBounce,
		},
		{
			name:  "bounce via delivery-status content-type",
			reply: &InboundReply{FromEmail: "x@y.com", ContentType: "message/delivery-status"},
			want:  ReplyBounce,
		},
		{
			name:  "bounce via mailer-daemon sender",
			reply: &InboundReply{FromEmail: "MAILER-DAEMON@mail.example.com", Subject: "Undelivered Mail"},
			want:  ReplyBounce,
		},
		{
			name:  "bounce via postmaster sender",
			reply: &InboundReply{FromEmail: "postmaster@example.com"},
			want:  ReplyBounce,
		},
		{
			name:  "auto-responder via Auto-Submitted auto-replied",
			reply: &InboundReply{FromEmail: "jane@example.com", Subject: "Re: Welcome", AutoSubmitted: "auto-replied"},
			want:  ReplyAutoResponder,
		},
		{
			name:  "auto-responder via Auto-Submitted auto-generated",
			reply: &InboundReply{FromEmail: "jane@example.com", AutoSubmitted: "auto-generated"},
			want:  ReplyAutoResponder,
		},
		{
			name:  "Auto-Submitted: no is NOT an auto-responder",
			reply: &InboundReply{FromEmail: "jane@example.com", Subject: "Re: Welcome", AutoSubmitted: "no"},
			want:  ReplyGenuine,
		},
		{
			name:  "auto-responder via Precedence bulk",
			reply: &InboundReply{FromEmail: "jane@example.com", Precedence: "bulk"},
			want:  ReplyAutoResponder,
		},
		{
			name:  "auto-responder via X-Autoreply header",
			reply: &InboundReply{FromEmail: "jane@example.com", RawHeaders: map[string]string{"X-Autoreply": "yes"}},
			want:  ReplyAutoResponder,
		},
		{
			name:  "auto-responder header match is case-insensitive",
			reply: &InboundReply{FromEmail: "jane@example.com", RawHeaders: map[string]string{"x-autorespond": "1"}},
			want:  ReplyAutoResponder,
		},
		{
			name:  "a subject asking to unsubscribe is a genuine reply",
			reply: &InboundReply{FromEmail: "jane@example.com", Subject: "Unsubscribe"},
			want:  ReplyGenuine,
		},
		{
			name:  "a STOP reply is a genuine reply",
			reply: &InboundReply{FromEmail: "jane@example.com", Subject: "Re: STOP"},
			want:  ReplyGenuine,
		},
		{
			name:  "an auto-responder titled STOP stays an auto-responder",
			reply: &InboundReply{FromEmail: "jane@example.com", Subject: "Re: STOP", AutoSubmitted: "auto-replied"},
			want:  ReplyAutoResponder,
		},
		{
			name:  "a bounce titled STOP stays a bounce",
			reply: &InboundReply{FromEmail: "postmaster@example.com", Subject: "STOP"},
			want:  ReplyBounce,
		},
		{
			name:  "subject merely containing unsubscribe is still a genuine reply",
			reply: &InboundReply{FromEmail: "jane@example.com", Subject: "Re: how do I unsubscribe later?"},
			want:  ReplyGenuine,
		},
		{
			name:  "bounce takes precedence over auto-responder headers",
			reply: &InboundReply{FromEmail: "mailer-daemon@x.com", AutoSubmitted: "auto-replied"},
			want:  ReplyBounce,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.reply); got != tc.want {
				t.Fatalf("Classify() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestClassify_StopWordsAreGenuineReplies(t *testing.T) {
	subjects := []string{
		"unsubscribe", "stop", "remove", "remove me", "opt out", "opt-out",
		"Re: Re: stop", "  RE:  Opt-Out  ",
	}
	for _, subject := range subjects {
		t.Run(subject, func(t *testing.T) {
			reply := &InboundReply{FromEmail: "jane@example.com", Subject: subject}
			if got := Classify(reply); got != ReplyGenuine {
				t.Fatalf("Classify(%q) = %s, want %s", subject, got, ReplyGenuine)
			}
		})
	}
}

func TestReplyClassString(t *testing.T) {
	cases := map[ReplyClass]string{
		ReplyGenuine:       "genuine",
		ReplyAutoResponder: "auto_responder",
		ReplyBounce:        "bounce",
		ReplyClass(99):     "unknown",
	}
	for c, want := range cases {
		if got := c.String(); got != want {
			t.Errorf("ReplyClass(%d).String() = %q, want %q", c, got, want)
		}
	}
}
