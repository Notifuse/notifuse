#!/bin/bash

# Advanced SMTP relay test script with multiple test scenarios
# Usage: ./scripts/test-smtp-relay-advanced.sh [api_email] [api_key] [workspace_id]

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SMTP_SERVER="${SMTP_SERVER:-localapi.notifuse.com}"
SMTP_PORT="${SMTP_PORT:-587}"
API_EMAIL="${1}"
API_KEY="${2}"
WORKSPACE_ID="${3}"

print_header() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

send_test() {
    local test_name="$1"
    local from="$2"
    local to="$3"
    local payload="$4"
    local headers="$5"
    
    echo -e "${YELLOW}▶${NC} Testing: $test_name"
    
    if swaks \
        --to "$to" \
        --from "$from" \
        --server "$SMTP_SERVER:$SMTP_PORT" \
        --tls \
        --tls-verify=no \
        --auth-user "$API_EMAIL" \
        --auth-password "$API_KEY" \
        --header "Subject: Test - $test_name" \
        $headers \
        --body "$payload" \
        --hide-all 2>&1 | grep -q "250 Ok"; then
        echo -e "${GREEN}✓${NC} $test_name: PASSED"
        return 0
    else
        echo -e "  $test_name: FAILED"
        return 1
    fi
}

if [ -z "$API_EMAIL" ] || [ -z "$API_KEY" ] || [ -z "$WORKSPACE_ID" ]; then
    echo "Usage: $0 <api_email> <api_key> <workspace_id>"
    echo ""
    echo "Example:"
    echo "  $0 api@example.com \"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...\" workspace_abc123"
    exit 1
fi

print_header "SMTP Relay Advanced Tests"

passed=0
failed=0

# Test 1: Simple notification
print_header "Test 1: Simple Notification"
if send_test \
    "Simple notification" \
    "test@example.com" \
    "user@example.com" \
    "{\"workspace_id\": \"$WORKSPACE_ID\", \"notification\": {\"id\": \"welcome_email\", \"contact\": {\"email\": \"user@example.com\"}}}" \
    ""; then
    ((passed++))
else
    ((failed++))
fi

# Test 2: Notification with contact details
print_header "Test 2: Full Contact Details"
if send_test \
    "Full contact details" \
    "sender@example.com" \
    "recipient@example.com" \
    "{\"workspace_id\": \"$WORKSPACE_ID\", \"notification\": {\"id\": \"password_reset\", \"contact\": {\"email\": \"user@example.com\", \"first_name\": \"John\", \"last_name\": \"Doe\"}, \"data\": {\"reset_token\": \"abc123\"}}}" \
    ""; then
    ((passed++))
else
    ((failed++))
fi

# Test 3: With email headers (CC, BCC, Reply-To)
print_header "Test 3: Email Headers (CC, BCC, Reply-To)"
if send_test \
    "Email headers" \
    "sender@example.com" \
    "recipient@example.com" \
    "{\"workspace_id\": \"$WORKSPACE_ID\", \"notification\": {\"id\": \"order_confirmation\", \"contact\": {\"email\": \"customer@example.com\"}, \"data\": {\"order_id\": \"12345\"}}}" \
    '--header "Cc: manager@example.com" --header "Bcc: archive@example.com" --header "Reply-To: support@example.com"'; then
    ((passed++))
else
    ((failed++))
fi

# Test 4: With JSON email_options
print_header "Test 4: JSON Email Options"
if send_test \
    "JSON email options" \
    "sender@example.com" \
    "recipient@example.com" \
    "{\"workspace_id\": \"$WORKSPACE_ID\", \"notification\": {\"id\": \"newsletter\", \"contact\": {\"email\": \"subscriber@example.com\"}, \"email_options\": {\"cc\": [\"team@example.com\"], \"bcc\": [\"analytics@example.com\"], \"reply_to\": \"noreply@example.com\"}}}" \
    ""; then
    ((passed++))
else
    ((failed++))
fi

# Test 5: Complex data object
print_header "Test 5: Complex Data Object"
if send_test \
    "Complex data" \
    "app@example.com" \
    "user@example.com" \
    "{\"workspace_id\": \"$WORKSPACE_ID\", \"notification\": {\"id\": \"invoice\", \"contact\": {\"email\": \"customer@example.com\", \"first_name\": \"Jane\"}, \"data\": {\"invoice_id\": \"INV-2024-001\", \"amount\": \"99.99\", \"currency\": \"USD\", \"items\": [{\"name\": \"Product A\", \"quantity\": 2}]}, \"metadata\": {\"source\": \"billing_system\", \"priority\": \"high\"}}}" \
    ""; then
    ((passed++))
else
    ((failed++))
fi

# Test 6: Metadata and tags
print_header "Test 6: Metadata and Tags"
if send_test \
    "Metadata" \
    "system@example.com" \
    "admin@example.com" \
    "{\"workspace_id\": \"$WORKSPACE_ID\", \"notification\": {\"id\": \"system_alert\", \"contact\": {\"email\": \"admin@example.com\"}, \"metadata\": {\"environment\": \"production\", \"severity\": \"warning\", \"triggered_by\": \"monitoring\"}}}" \
    ""; then
    ((passed++))
else
    ((failed++))
fi

# Summary
print_header "Test Summary"
total=$((passed + failed))
echo -e "${GREEN}Passed:${NC} $passed/$total"
if [ $failed -gt 0 ]; then
    echo -e "Failed: $failed/$total"
fi
echo ""

if [ $failed -eq 0 ]; then
    echo -e "${GREEN}✓ All tests passed!${NC}"
    exit 0
else
    echo "✗ Some tests failed"
    exit 1
fi

