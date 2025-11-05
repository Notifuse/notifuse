# Scripts Directory

Utility scripts for development and testing.

## Available Scripts

### 📜 `generate-dev-certs.sh`

Generates self-signed TLS certificates for local development.

**Usage:**
```bash
./scripts/generate-dev-certs.sh [domain]
```

**Example:**
```bash
./scripts/generate-dev-certs.sh localapi.notifuse.com
```

**Output:**
- `dev-certs/[domain].cert.pem` - TLS certificate
- `dev-certs/[domain].key.pem` - Private key
- `dev-certs/.env.smtp-relay` - Environment variables (base64 encoded)

---

### 📧 `test-smtp-relay.sh`

Sends a test email to the local SMTP relay server.

**Usage:**
```bash
./scripts/test-smtp-relay.sh <workspace_id> <api_key>
```

**Example:**
```bash
./scripts/test-smtp-relay.sh workspace_abc123 "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

**Features:**
- ✅ Checks server connectivity
- ✅ Uses TLS with proper certificate verification
- ✅ Sends JSON payload with test data
- ✅ Colored output with clear success/failure messages
- ✅ Helpful error messages and debugging tips

**Environment Variables:**
```bash
SMTP_SERVER=localapi.notifuse.com    # SMTP server address
SMTP_PORT=587                         # SMTP server port
FROM_EMAIL=test@example.com           # Sender email
TO_EMAIL=recipient@example.com        # Recipient email
NOTIFICATION_ID=password_reset        # Notification template ID
```

**Example with custom settings:**
```bash
NOTIFICATION_ID=welcome_email \
FROM_EMAIL=sender@myapp.com \
TO_EMAIL=user@example.com \
./scripts/test-smtp-relay.sh workspace_123 "api_key_jwt"
```

---

### 🧪 `test-smtp-relay-advanced.sh`

Runs multiple SMTP relay test scenarios to verify different features.

**Usage:**
```bash
./scripts/test-smtp-relay-advanced.sh <workspace_id> <api_key>
```

**Test Scenarios:**
1. ✅ Simple notification
2. ✅ Full contact details
3. ✅ Email headers (CC, BCC, Reply-To)
4. ✅ JSON email options
5. ✅ Complex data objects
6. ✅ Metadata and tags

**Example:**
```bash
./scripts/test-smtp-relay-advanced.sh workspace_abc123 "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

**Output:**
```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Test 1: Simple Notification
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

▶ Testing: Simple notification
✓ Simple notification: PASSED

...

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Test Summary
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Passed: 6/6

✓ All tests passed!
```

---

### 🧪 `run-integration-tests.sh`

Runs integration tests with proper setup and teardown.

**Usage:**
```bash
./scripts/run-integration-tests.sh [test_pattern]
```

**Example:**
```bash
# Run all tests
./scripts/run-integration-tests.sh

# Run specific test
./scripts/run-integration-tests.sh TestBroadcast
```

---

## Prerequisites

### Install swaks (for SMTP testing)

```bash
# macOS
brew install swaks

# Ubuntu/Debian
sudo apt-get install swaks

# Fedora/RHEL
dnf install swaks
```

### Install netcat (usually pre-installed)

Used for connectivity checks.

```bash
# macOS (via Homebrew)
brew install netcat

# Ubuntu/Debian
sudo apt-get install netcat
```

## Quick Start

### 1. Generate Certificates

```bash
./scripts/generate-dev-certs.sh localapi.notifuse.com
```

### 2. Add Domain to Hosts

```bash
echo "127.0.0.1 localapi.notifuse.com" | sudo tee -a /etc/hosts
```

### 3. Configure Environment

```bash
cat dev-certs/.env.smtp-relay >> .env
```

### 4. Start Server

```bash
make dev
```

### 5. Test SMTP Relay

First, get your workspace ID and API key from the application, then:

```bash
./scripts/test-smtp-relay.sh your_workspace_id "your_api_key_jwt"
```

## Troubleshooting

### Script Permission Denied

```bash
chmod +x scripts/*.sh
```

### swaks Not Found

Install swaks (see Prerequisites above).

### Connection Refused

Ensure:
1. SMTP relay is enabled in `.env`
2. Server is running (`make dev`)
3. Domain is in `/etc/hosts`
4. Port 587 is not blocked

### Authentication Failed

Verify:
1. Workspace ID is correct
2. API key is a valid JWT token
3. API key hasn't expired
4. JWT secret matches

### TLS Verification Failed

Use the certificate path:
```bash
--tls-ca-path ./dev-certs/localapi.notifuse.com.cert.pem
```

Or disable verification for testing:
```bash
--tls-verify=no
```

## See Also

- [Setup Local SMTP](../SETUP_LOCAL_SMTP.md)
- [SMTP Relay Implementation](../SMTP_RELAY_IMPLEMENTATION.md)
- [Environment Variables](../env.example)

