#!/bin/bash

# Script to test the SMTP relay server locally
# Usage: ./scripts/test-smtp-relay.sh [workspace_id] [api_key]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
SMTP_SERVER="${SMTP_SERVER:-localapi.notifuse.com}"
SMTP_PORT="${SMTP_PORT:-587}"
FROM_EMAIL="${FROM_EMAIL:-test@example.com}"
TO_EMAIL="${TO_EMAIL:-recipient@example.com}"
API_EMAIL="${1}"
API_KEY="${2}"
WORKSPACE_ID="${3}"
NOTIFICATION_ID="${NOTIFICATION_ID:-password_reset}"

# Function to print colored output
print_info() {
    echo -e "${BLUE}ℹ ${NC}$1"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

# Check if swaks is installed
if ! command -v swaks &> /dev/null; then
    print_error "swaks is not installed"
    echo ""
    echo "Install swaks:"
    echo "  macOS:        brew install swaks"
    echo "  Ubuntu/Debian: sudo apt-get install swaks"
    echo ""
    exit 1
fi

# Check if required parameters are provided
if [ -z "$API_EMAIL" ] || [ -z "$API_KEY" ] || [ -z "$WORKSPACE_ID" ]; then
    print_error "Missing required parameters"
    echo ""
    echo "Usage: $0 <api_email> <api_key> <workspace_id>"
    echo ""
    echo "Example:"
    echo "  $0 api@example.com \"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...\" workspace_abc123"
    echo ""
    echo "Environment variables (optional):"
    echo "  SMTP_SERVER=localapi.notifuse.com    # SMTP server address"
    echo "  SMTP_PORT=587                         # SMTP server port"
    echo "  FROM_EMAIL=test@example.com           # Sender email"
    echo "  TO_EMAIL=recipient@example.com        # Recipient email"
    echo "  NOTIFICATION_ID=password_reset        # Notification template ID"
    echo ""
    exit 1
fi

# Create JSON payload matching OpenAPI spec
read -r -d '' JSON_PAYLOAD <<'EOF' || true
{
  "workspace_id": "WORKSPACE_ID_PLACEHOLDER",
  "notification": {
    "id": "NOTIFICATION_ID_PLACEHOLDER",
    "contact": {
      "email": "user@example.com",
      "first_name": "John",
      "last_name": "Doe"
    },
    "data": {
      "reset_token": "abc123xyz",
      "expires_in": "1 hour",
      "test_timestamp": "TIMESTAMP_PLACEHOLDER",
      "custom_var": "CUSTOM_VAR_PLACEHOLDER"
    },
    "metadata": {
      "source": "smtp_relay_test_script",
      "test_id": "TEST_ID_PLACEHOLDER"
    }
  }
}
EOF

# Replace placeholders
JSON_PAYLOAD="${JSON_PAYLOAD//WORKSPACE_ID_PLACEHOLDER/$WORKSPACE_ID}"
JSON_PAYLOAD="${JSON_PAYLOAD//NOTIFICATION_ID_PLACEHOLDER/$NOTIFICATION_ID}"
JSON_PAYLOAD="${JSON_PAYLOAD//TIMESTAMP_PLACEHOLDER/$(date +%Y-%m-%dT%H:%M:%S%z)}"
JSON_PAYLOAD="${JSON_PAYLOAD//TEST_ID_PLACEHOLDER/$(date +%s)}"
JSON_PAYLOAD="${JSON_PAYLOAD//CUSTOM_VAR_PLACEHOLDER/test-$(openssl rand -hex 8)}"

# Check if server is reachable
print_info "Checking SMTP server connectivity..."
if ! nc -z -w 5 "$(echo $SMTP_SERVER | sed 's/:.*$//')" "$SMTP_PORT" 2>/dev/null; then
    print_error "Cannot connect to $SMTP_SERVER:$SMTP_PORT"
    print_warning "Make sure the SMTP relay server is running (make dev)"
    print_warning "And that '$SMTP_SERVER' is in your /etc/hosts file"
    exit 1
fi
print_success "Server is reachable"

# Display test configuration
echo ""
print_info "Test Configuration:"
echo "  Server:         $SMTP_SERVER:$SMTP_PORT"
echo "  API Email:      $API_EMAIL"
echo "  Workspace ID:   $WORKSPACE_ID"
echo "  From:           $FROM_EMAIL"
echo "  To:             $TO_EMAIL"
echo "  Notification:   $NOTIFICATION_ID"
echo ""

# Send test email
print_info "Sending test email..."
echo ""

# Check if TLS certificate exists
TLS_CA_PATH=""
if [ -f "./dev-certs/$SMTP_SERVER.cert.pem" ]; then
    TLS_CA_PATH="--tls-ca-path ./dev-certs/$SMTP_SERVER.cert.pem"
    print_info "Using TLS certificate: ./dev-certs/$SMTP_SERVER.cert.pem"
elif [ -f "./dev-certs/localapi.notifuse.com.cert.pem" ]; then
    TLS_CA_PATH="--tls-ca-path ./dev-certs/localapi.notifuse.com.cert.pem"
    print_info "Using TLS certificate: ./dev-certs/localapi.notifuse.com.cert.pem"
else
    print_warning "TLS certificate not found, using --tls-verify=no (INSECURE)"
    TLS_CA_PATH="--tls-verify=no"
fi

# Run swaks
if swaks \
    --to "$TO_EMAIL" \
    --from "$FROM_EMAIL" \
    --server "$SMTP_SERVER:$SMTP_PORT" \
    --tls \
    $TLS_CA_PATH \
    --auth-user "$API_EMAIL" \
    --auth-password "$API_KEY" \
    --header "Subject: SMTP Relay Test - $NOTIFICATION_ID" \
    --header "Content-Type: application/json" \
    --body "$JSON_PAYLOAD" \
    --hide-all; then
    
    echo ""
    print_success "Email sent successfully!"
    echo ""
    print_info "Check server logs for processing details:"
    echo "  Look for: \"SMTP relay: Notification sent successfully\""
    echo ""
else
    echo ""
    print_error "Failed to send email"
    echo ""
    print_info "Common issues:"
    echo "  1. Server not running:    Run 'make dev' first"
    echo "  2. Invalid credentials:   Check api_email, api_key, and workspace_id"
    echo "  3. TLS verification:      Ensure domain is in /etc/hosts"
    echo "  4. Authentication error:  Verify API key is valid and matches api_email"
    echo ""
    exit 1
fi

