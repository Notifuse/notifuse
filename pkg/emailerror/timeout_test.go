package emailerror

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A send cut by its deadline is a retryable provider error for every provider, whatever its text
// says: a network timeout's text carries addresses, which can read as a recipient's status code.
func TestClassify_ASendCutByItsDeadlineIsARetryableProviderTimeout(t *testing.T) {
	readTimeout := &net.OpError{
		Op:   "read",
		Net:  "tcp",
		Addr: &net.TCPAddr{IP: net.ParseIP("10.5.1.1"), Port: 25},
		Err:  os.ErrDeadlineExceeded,
	}
	require.Contains(t, readTimeout.Error(), "5.1.1", "the address reads as the 5.1.1 'bad mailbox' code")

	errs := map[string]error{
		"a context deadline":                fmt.Errorf("failed to send email: %w", context.DeadlineExceeded),
		"a network timeout with an address": fmt.Errorf("failed to read RCPT response: %w", readTimeout),
	}
	kinds := []domain.EmailProviderKind{
		domain.EmailProviderKindSMTP, domain.EmailProviderKindSES, domain.EmailProviderKindSparkPost,
		domain.EmailProviderKindPostmark, domain.EmailProviderKindMailgun, domain.EmailProviderKindMailjet,
		domain.EmailProviderKindSendGrid, domain.EmailProviderKind("other"),
	}
	classifier := NewClassifier()
	for name, err := range errs {
		for _, kind := range kinds {
			t.Run(name+" from "+string(kind), func(t *testing.T) {
				got := classifier.Classify(err, kind)
				require.NotNil(t, got)
				assert.Equal(t, ErrorTypeProvider, got.Type)
				assert.True(t, got.Retryable)
				assert.False(t, got.RateLimited)
				assert.True(t, errors.Is(got, err) || got.Original == err)
			})
		}
	}
}
