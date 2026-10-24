package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// accountIDFromARN gates the SES publish policy's SourceAccount condition:
// a malformed ARN must yield "" so EnsureInboundRoute fails closed instead
// of granting an unconditioned publish.
func TestAccountIDFromARN(t *testing.T) {
	assert.Equal(t, "123456789012",
		accountIDFromARN("arn:aws:sns:us-east-1:123456789012:notifuse-ses-inbound-x"))
	assert.Equal(t, "", accountIDFromARN(""))
	assert.Equal(t, "", accountIDFromARN("not-an-arn"))
	assert.Equal(t, "", accountIDFromARN("arn:aws:sns:us-east-1:"))
	assert.Equal(t, "", accountIDFromARN("arn:aws:sns:us-east-1"))
}
