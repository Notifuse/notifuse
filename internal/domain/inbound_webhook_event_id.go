package domain

import (
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// providerEventNamespace is the UUID namespace of the IDs derived from a provider's own
// identifier of an event. It must never change: the IDs of stored events depend on it.
var providerEventNamespace = uuid.MustParse("6f0e4b8a-3c1d-5e2f-9a7b-1d4c8e6f2a90")

// ProviderEventID gives an event of an email provider the same ID each time the provider
// delivers it, so that a redelivery conflicts with the row the first delivery stored. The
// key is what identifies the event for its provider: its event ID, or the fields that make
// up one when the provider has none. The ID is a version 5 UUID of the integration, the
// source, the event type and the key, each part prefixed by its length so that no two
// different lists of parts give the same name.
//
// When the key is missing or any of its parts is empty, the event cannot be told apart from
// another one of the same email, and it gets a new random ID: it is stored again on a
// redelivery rather than merged with a different event.
func ProviderEventID(integrationID string, source WebhookSource, eventType EmailEventType, key ...string) string {
	if len(key) == 0 {
		return uuid.New().String()
	}
	for _, part := range key {
		if part == "" {
			return uuid.New().String()
		}
	}

	var name strings.Builder
	for _, part := range append([]string{integrationID, string(source), string(eventType)}, key...) {
		name.WriteString(strconv.Itoa(len(part)))
		name.WriteByte(':')
		name.WriteString(part)
	}
	return uuid.NewSHA1(providerEventNamespace, []byte(name.String())).String()
}
