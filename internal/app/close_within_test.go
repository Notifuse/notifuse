package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCloseWithin(t *testing.T) {
	t.Run("returns the close's own error", func(t *testing.T) {
		closeErr := errors.New("close failed")
		err := closeWithin(context.Background(), func() error { return closeErr })
		assert.ErrorIs(t, err, closeErr)
	})

	t.Run("returns nil when the close ends without error", func(t *testing.T) {
		assert.NoError(t, closeWithin(context.Background(), func() error { return nil }))
	})

	t.Run("returns when the context ends before a stuck close", func(t *testing.T) {
		release := make(chan struct{})
		defer close(release)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		start := time.Now()
		err := closeWithin(ctx, func() error { <-release; return nil })

		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Less(t, time.Since(start), 2*time.Second)
	})

	t.Run("with the deadline already passed, a quick close still ends", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		assert.NoError(t, closeWithin(ctx, func() error { time.Sleep(10 * time.Millisecond); return nil }))
	})

	t.Run("with the deadline already passed, a stuck close is left after the grace", func(t *testing.T) {
		original := closeGrace
		closeGrace = 50 * time.Millisecond
		defer func() { closeGrace = original }()
		release := make(chan struct{})
		defer close(release)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		start := time.Now()
		err := closeWithin(ctx, func() error { <-release; return nil })

		assert.ErrorIs(t, err, context.Canceled)
		assert.Less(t, time.Since(start), 2*time.Second)
	})
}
