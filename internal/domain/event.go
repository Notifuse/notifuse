package domain

import (
	"context"
	"fmt"
	"sync"
	"time"
)

//go:generate mockgen -destination mocks/mock_event_bus.go -package mocks github.com/Notifuse/notifuse/internal/domain EventBus

// EventType defines the type of an event
type EventType string

// Define event types
const (
	EventBroadcastScheduled      EventType = "broadcast.scheduled"
	EventBroadcastPaused         EventType = "broadcast.paused"
	EventBroadcastResumed        EventType = "broadcast.resumed"
	EventBroadcastSent           EventType = "broadcast.sent"
	EventBroadcastFailed         EventType = "broadcast.failed"
	EventBroadcastCancelled      EventType = "broadcast.cancelled"
	EventBroadcastCircuitBreaker EventType = "broadcast.circuit_breaker"
)

// EventPayload represents the data associated with an event
type EventPayload struct {
	Type        EventType              `json:"type"`
	WorkspaceID string                 `json:"workspace_id"`
	EntityID    string                 `json:"entity_id"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

// EventHandler is a function that handles events
type EventHandler func(ctx context.Context, payload EventPayload)

// EventAckCallback is a function that's called after an event is processed
// to acknowledge success or failure
type EventAckCallback func(err error)

// EventBus provides a way for services to publish and subscribe to events
type EventBus interface {
	// Publish sends an event to all subscribers
	Publish(ctx context.Context, event EventPayload)

	// PublishWithAck sends an event to all subscribers and calls the acknowledgment callback
	// when all subscribers have processed the event or if an error occurs
	PublishWithAck(ctx context.Context, event EventPayload, callback EventAckCallback)

	// Subscribe registers a handler for a specific event type
	Subscribe(eventType EventType, handler EventHandler)

	// Unsubscribe removes a handler for an event type
	Unsubscribe(eventType EventType, handler EventHandler)
}

// InMemoryEventBus is a simple in-memory implementation of the EventBus
type InMemoryEventBus struct {
	subscribers map[EventType][]EventHandler
	mu          sync.RWMutex
}

// NewInMemoryEventBus creates a new in-memory event bus
func NewInMemoryEventBus() *InMemoryEventBus {
	return &InMemoryEventBus{
		subscribers: make(map[EventType][]EventHandler),
	}
}

// Publish sends an event to all subscribers
func (b *InMemoryEventBus) Publish(ctx context.Context, event EventPayload) {
	b.PublishWithAck(ctx, event, nil)
}

// PublishWithAck sends an event to all subscribers and calls the acknowledgment callback
func (b *InMemoryEventBus) PublishWithAck(ctx context.Context, event EventPayload, callback EventAckCallback) {
	b.mu.RLock()
	handlers, exists := b.subscribers[event.Type]
	b.mu.RUnlock()

	if !exists || len(handlers) == 0 {
		// No handlers, consider it a success
		if callback != nil {
			callback(nil)
		}
		return
	}

	// If we have a callback, we need to track when all handlers are done
	if callback != nil {
		var wg sync.WaitGroup
		wg.Add(len(handlers))

		// Create a channel to collect errors
		errCh := make(chan error, len(handlers))

		// Process each handler concurrently
		for _, handler := range handlers {
			go func(h EventHandler) {
				defer wg.Done()

				// Create a timeout context for the handler
				handlerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()

				// Create a channel to detect handler completion
				done := make(chan struct{})

				// Process the event in a goroutine
				go func() {
					defer close(done)

					// Catch and handle panics in event handlers
					defer func() {
						if r := recover(); r != nil {
							errMsg := fmt.Sprintf("panic in event handler: %v", r)
							errCh <- fmt.Errorf("%s", errMsg)
						}
					}()

					// Call the actual handler
					h(handlerCtx, event)
				}()

				// Wait for handler completion or timeout
				select {
				case <-done:
					// Handler completed normally
				case <-handlerCtx.Done():
					// Handler timed out
					errCh <- fmt.Errorf("event handler timed out: %v", handlerCtx.Err())
				}
			}(handler)
		}

		// Wait for all handlers to complete in a separate goroutine
		go func() {
			wg.Wait()
			close(errCh)

			// Collect errors
			var allErrors []error
			for err := range errCh {
				allErrors = append(allErrors, err)
			}

			// Call the callback with aggregated errors
			if len(allErrors) > 0 {
				// Create a combined error
				errMsg := fmt.Sprintf("%d errors occurred processing event", len(allErrors))
				for i, err := range allErrors {
					errMsg += fmt.Sprintf("\n  %d: %v", i+1, err)
				}
				callback(fmt.Errorf("%s", errMsg))
			} else {
				callback(nil)
			}
		}()
	} else {
		// No callback, just process handlers without waiting
		for _, handler := range handlers {
			go func(h EventHandler) {
				defer func() {
					if r := recover(); r != nil {
						// Just log the panic, no callback to report to
						fmt.Printf("ERROR: Panic in event handler: %v\n", r)
					}
				}()

				h(ctx, event)
			}(handler)
		}
	}
}

// Subscribe registers a handler for a specific event type
func (b *InMemoryEventBus) Subscribe(eventType EventType, handler EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Initialize slice if needed
	if _, exists := b.subscribers[eventType]; !exists {
		b.subscribers[eventType] = make([]EventHandler, 0)
	}

	// Add the handler
	b.subscribers[eventType] = append(b.subscribers[eventType], handler)
}

// Unsubscribe removes a handler for an event type
func (b *InMemoryEventBus) Unsubscribe(eventType EventType, handler EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	handlers, exists := b.subscribers[eventType]
	if !exists {
		return
	}

	// Find and remove the handler based on pointer equality
	for i, h := range handlers {
		// Go doesn't support function comparison, so we use the pointer value
		// This is a simplification and may not work for all use cases
		if &h == &handler {
			// Remove by swapping with the last element and truncating
			handlers[i] = handlers[len(handlers)-1]
			b.subscribers[eventType] = handlers[:len(handlers)-1]
			break
		}
	}
}
