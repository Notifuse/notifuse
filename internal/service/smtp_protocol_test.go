package service

import (
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lockedBuffer captures peer-side reads without racing the test goroutine
// (CI runs -race).
type lockedBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// smtpPipe returns a client connection whose peer plays back the scripted
// server replies; everything the client writes is captured for assertion.
func smtpPipe(t *testing.T, replies string) (*smtpConnection, *lockedBuffer) {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	written := &lockedBuffer{}
	// net.Pipe is unbuffered: the reply writer and the command reader must run
	// in separate goroutines or both sides block forever.
	go func() { _, _ = server.Write([]byte(replies)) }()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := server.Read(buf)
			if n > 0 {
				written.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	conn := newSMTPConnection(client)
	require.NoError(t, client.SetDeadline(time.Now().Add(5*time.Second)))
	return conn, written
}

func TestSMTPReadResponse_ProtocolGuards(t *testing.T) {
	t.Run("code and text are split at the separator", func(t *testing.T) {
		c, _ := smtpPipe(t, "250 Queued as 12345\r\n")
		code, text, err := c.readResponse()
		require.NoError(t, err)
		assert.Equal(t, 250, code)
		assert.Equal(t, "Queued as 12345", text)
	})

	t.Run("a line shorter than a code is a protocol error, not a panic", func(t *testing.T) {
		c, _ := smtpPipe(t, "2\n")
		_, _, err := c.readResponse()
		require.EqualError(t, err, "short response: 2\n")
	})

	t.Run("a non-numeric code is a protocol error naming the line", func(t *testing.T) {
		c, _ := smtpPipe(t, "XXX hello\r\n")
		_, _, err := c.readResponse()
		require.EqualError(t, err, "invalid response code: XXX hello\r\n")
	})

	t.Run("EOF surfaces as an error with no code", func(t *testing.T) {
		client, server := net.Pipe()
		_ = server.Close()
		c := newSMTPConnection(client)
		code, _, err := c.readResponse()
		assert.Equal(t, 0, code)
		assert.ErrorIs(t, err, io.EOF)
		_ = client.Close()
	})
}

// Multiline replies end only on "code SPACE"; "code-" lines are continuations.
// Getting this wrong hangs or truncates capability parsing (AUTH mechanisms).
func TestSMTPReadMultilineResponse(t *testing.T) {
	t.Run("dash lines continue, space line terminates", func(t *testing.T) {
		c, _ := smtpPipe(t, "250-smtp.example.com\r\n250-AUTH PLAIN LOGIN\r\n250 SIZE 35882577\r\n")
		code, lines, err := c.readMultilineResponseWithLines()
		require.NoError(t, err)
		assert.Equal(t, 250, code)
		assert.Equal(t, []string{"smtp.example.com", "AUTH PLAIN LOGIN", "SIZE 35882577"}, lines)
	})

	t.Run("single line multiline returns immediately", func(t *testing.T) {
		c, _ := smtpPipe(t, "220 ready\r\n")
		code, err := c.readMultilineResponse()
		require.NoError(t, err)
		assert.Equal(t, 220, code)
	})

	t.Run("a short continuation line is an error", func(t *testing.T) {
		c, _ := smtpPipe(t, "250-ok\r\n2\r\n")
		_, err := c.readMultilineResponse()
		require.EqualError(t, err, "short response: 2\r\n")
	})

	t.Run("an invalid code inside a multiline reply is an error", func(t *testing.T) {
		c, _ := smtpPipe(t, "250-ok\r\nZZZ bad\r\n")
		_, _, err := c.readMultilineResponseWithLines()
		require.EqualError(t, err, "invalid response code: ZZZ bad\r\n")
	})

	t.Run("EOF mid-multiline is an error, not an infinite loop", func(t *testing.T) {
		client, server := net.Pipe()
		go func() { _, _ = server.Write([]byte("250-never ends\r\n")); _ = server.Close() }()
		c := newSMTPConnection(client)
		_, err := c.readMultilineResponse()
		assert.ErrorIs(t, err, io.EOF)
		_ = client.Close()
	})
}

func TestSMTPSendCommand_WireFormat(t *testing.T) {
	t.Run("command is CRLF-terminated and the reply is parsed", func(t *testing.T) {
		c, written := smtpPipe(t, "250 2.0.0 OK\r\n")
		code, text, err := c.sendCommand("NOOP")
		require.NoError(t, err)
		assert.Equal(t, 250, code)
		assert.Equal(t, "2.0.0 OK", text)
		assert.Eventually(t, func() bool { return written.String() == "NOOP\r\n" },
			2*time.Second, 5*time.Millisecond, "client must send exactly the command plus CRLF")
	})

	t.Run("multiline command collects capability lines", func(t *testing.T) {
		c, written := smtpPipe(t, "250-host\r\n250 AUTH LOGIN\r\n")
		code, lines, err := c.sendCommandMultilineWithLines("EHLO notifuse")
		require.NoError(t, err)
		assert.Equal(t, 250, code)
		assert.Equal(t, []string{"host", "AUTH LOGIN"}, lines)
		assert.Eventually(t, func() bool { return written.String() == "EHLO notifuse\r\n" },
			2*time.Second, 5*time.Millisecond)
	})

	t.Run("writing to a closed connection errors instead of hanging", func(t *testing.T) {
		client, server := net.Pipe()
		_ = server.Close()
		_ = client.Close()
		c := newSMTPConnection(client)
		_, _, err := c.sendCommand("NOOP")
		assert.Error(t, err)
	})
}
