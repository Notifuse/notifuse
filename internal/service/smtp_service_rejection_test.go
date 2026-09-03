package service

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rejectingSMTPServer is a minimal SMTP server that answers one chosen step with a
// scripted rejection. The full mock in smtp_service_test.go models authentication and
// STARTTLS; none of that is needed to prove that a server's own explanation survives
// into the error, and scripting a single reply keeps each case readable.
type rejectingSMTPServer struct {
	listener net.Listener
	wg       sync.WaitGroup

	// Each field, when set, replaces the success reply for that step.
	rcptReply    string // answer to RCPT TO instead of "250 OK"
	dataEndReply string // answer after the message body instead of "250 OK"
	mailFromEply string // answer to MAIL FROM instead of "250 OK"
}

func newRejectingSMTPServer(t *testing.T, s *rejectingSMTPServer) *rejectingSMTPServer {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	s.listener = listener
	s.wg.Add(1)
	go s.serve()
	return s
}

func (s *rejectingSMTPServer) serve() {
	defer s.wg.Done()
	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	conn.Write([]byte("220 localhost SMTP Rejecting Mock\r\n"))

	inData := false
	for {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)

		if inData {
			if line == "." {
				inData = false
				conn.Write([]byte(reply(s.dataEndReply, "250 OK message queued") + "\r\n"))
			}
			continue
		}

		switch {
		case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
			conn.Write([]byte("250-localhost\r\n250 OK\r\n"))
		case strings.HasPrefix(line, "MAIL FROM"):
			conn.Write([]byte(reply(s.mailFromEply, "250 OK") + "\r\n"))
		case strings.HasPrefix(line, "RCPT TO"):
			conn.Write([]byte(reply(s.rcptReply, "250 OK") + "\r\n"))
		case line == "DATA":
			conn.Write([]byte("354 Start mail input\r\n"))
			inData = true
		case line == "QUIT":
			conn.Write([]byte("221 Bye\r\n"))
			return
		default:
			conn.Write([]byte("250 OK\r\n"))
		}
	}
}

func reply(scripted, success string) string {
	if scripted != "" {
		return scripted
	}
	return success
}

func (s *rejectingSMTPServer) Port() int { return s.listener.Addr().(*net.TCPAddr).Port }

func (s *rejectingSMTPServer) Close() {
	s.listener.Close()
	s.wg.Wait()
}

// TestSendRawEmail_KeepsTheServersOwnExplanation covers the loss that made a rejection
// unreadable: the reply text was discarded and only the numeric code survived, so the
// operator saw "message rejected with code: 452" and had no way to learn it meant a
// daily quota. The code has to stay first in the message, because status_info is
// stored truncated at 255 characters.
func TestSendRawEmail_KeepsTheServersOwnExplanation(t *testing.T) {
	msg := []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nTest body")

	t.Run("quota rejection after the message body", func(t *testing.T) {
		server := newRejectingSMTPServer(t, &rejectingSMTPServer{
			dataEndReply: "452 4.3.1 Daily sending quota exceeded",
		})
		defer server.Close()

		err := sendRawEmail("127.0.0.1", server.Port(), "", "", false,
			"sender@example.com", []string{"recipient@example.com"}, msg)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "452")
		assert.Contains(t, err.Error(), "Daily sending quota exceeded")
	})

	t.Run("unknown mailbox on RCPT TO", func(t *testing.T) {
		server := newRejectingSMTPServer(t, &rejectingSMTPServer{
			rcptReply: "550 5.1.1 User unknown; rejecting",
		})
		defer server.Close()

		err := sendRawEmail("127.0.0.1", server.Port(), "", "", false,
			"sender@example.com", []string{"nobody@example.com"}, msg)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "550")
		assert.Contains(t, err.Error(), "User unknown")
		assert.Contains(t, err.Error(), "nobody@example.com", "the rejected address stays in the message")
	})

	t.Run("rejection on MAIL FROM", func(t *testing.T) {
		server := newRejectingSMTPServer(t, &rejectingSMTPServer{
			mailFromEply: "553 5.7.1 Sender address rejected",
		})
		defer server.Close()

		err := sendRawEmail("127.0.0.1", server.Port(), "", "", false,
			"sender@example.com", []string{"recipient@example.com"}, msg)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "553")
		assert.Contains(t, err.Error(), "Sender address rejected")
	})

	t.Run("a bare code with no text still reads", func(t *testing.T) {
		server := newRejectingSMTPServer(t, &rejectingSMTPServer{
			dataEndReply: "452",
		})
		defer server.Close()

		err := sendRawEmail("127.0.0.1", server.Port(), "", "", false,
			"sender@example.com", []string{"recipient@example.com"}, msg)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "452")
		assert.NotContains(t, err.Error(), "code :", "no dangling separator when the server said nothing")
	})
}

// TestSmtpReplyDetail_BoundsTheServerText keeps one verbose server from crowding the
// code out of a status_info field that is stored truncated.
func TestSmtpReplyDetail_BoundsTheServerText(t *testing.T) {
	long := strings.Repeat("x", 500)
	got := smtpReplyDetail(452, long)

	assert.True(t, strings.HasPrefix(got, "452: "), "the code leads, so truncation cannot lose it")
	assert.Less(t, len(got), 200)
	assert.Equal(t, "550", smtpReplyDetail(550, "   "), "a blank reply yields the bare code")
}

// TestSmtpReplyDetail_SurvivesANonASCIIReply is the case an ASCII fixture cannot reach.
// A server answering in its own language is cut mid-character by a byte slice, and the
// invalid UTF-8 that produces is refused by Postgres as a bind parameter — so the
// message-history upsert fails, and because that upsert only logs its errors the failed
// recipient vanishes from the Failed tile and from the logs entirely. The truncation
// has to happen on runes.
func TestSmtpReplyDetail_SurvivesANonASCIIReply(t *testing.T) {
	// Every character here is multi-byte, so any byte-wise cut lands inside one.
	reply := strings.Repeat("é", 300)

	got := smtpReplyDetail(550, reply)

	assert.True(t, utf8.ValidString(got), "a reply cut mid-character would be rejected by the database")
	assert.True(t, strings.HasPrefix(got, "550: "))
	assert.Equal(t, 185, utf8.RuneCountInString(got), "bounded on runes, not bytes")

	// A reply mixing scripts must survive too — the cut point is what matters.
	mixed := "5.1.1 Destinatário desconhecido — " + strings.Repeat("ü", 300)
	assert.True(t, utf8.ValidString(smtpReplyDetail(550, mixed)))
}
