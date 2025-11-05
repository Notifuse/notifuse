# Local SMTP Relay Development Setup

Quick guide to set up the SMTP relay server for local development.

## 🚀 Quick Start (3 Steps)

### 1. Generate Certificates

Already done! ✅

```bash
./scripts/generate-dev-certs.sh localapi.notifuse.com
```

Certificates are in `dev-certs/`:
- `localapi.notifuse.com.cert.pem` - TLS certificate
- `localapi.notifuse.com.key.pem` - Private key
- `.env.smtp-relay` - Environment variables (base64 encoded)

### 2. Add to Hosts File

```bash
sudo nano /etc/hosts
```

Add this line:
```
127.0.0.1 localapi.notifuse.com
```

Save and exit (Ctrl+O, Ctrl+X).

### 3. Configure Environment

Copy the SMTP relay configuration to your `.env` file:

```bash
cat dev-certs/.env.smtp-relay >> .env
```

Or manually add to `.env`:

```bash
SMTP_RELAY_ENABLED=true
SMTP_RELAY_PORT=587
SMTP_RELAY_HOST=0.0.0.0
SMTP_RELAY_DOMAIN=localapi.notifuse.com

# Copy the base64 values from dev-certs/.env.smtp-relay
SMTP_RELAY_TLS_CERT_BASE64="..."
SMTP_RELAY_TLS_KEY_BASE64="..."
```

## 🏃 Start Development Server

```bash
make dev
```

You should see:
```
{"level":"info","message":"SMTP relay: TLS enabled"}
{"level":"info","addr":"0.0.0.0:587","domain":"localapi.notifuse.com","tls":true,"message":"SMTP relay server initialized"}
{"level":"info","addr":"0.0.0.0:587","message":"Starting SMTP relay server"}
{"level":"info","addr":"0.0.0.0:587","message":"SMTP relay server listening"}
```

The server is now running on:
- **HTTP API**: http://localhost:8080
- **SMTP Relay**: smtp://localapi.notifuse.com:587

## 🧪 Testing

### Prerequisites

Install swaks (SMTP testing tool):
```bash
# macOS
brew install swaks

# Ubuntu/Debian
sudo apt-get install swaks
```

### Get an API Key

1. Create a workspace via the API or UI
2. Generate an API key for that workspace
3. Note the workspace ID and API key JWT token

### Send a Test Email

```bash
swaks --to test@example.com \
  --from sender@example.com \
  --server localapi.notifuse.com:587 \
  --tls \
  --tls-ca-path ./dev-certs/localapi.notifuse.com.cert.pem \
  --auth-user your_workspace_id \
  --auth-password "your-api-key-jwt-token" \
  --header "Subject: Test Notification" \
  --body '{"notification": {"id": "password_reset", "contact": {"email": "user@example.com", "first_name": "John"}, "data": {"reset_token": "abc123"}}}'
```

### Expected Response

Success:
```
<~  250 Ok: queued
 ~> QUIT
<~  221 Bye
=== Connection closed with remote host.
```

In server logs:
```json
{"level":"debug","username":"your_workspace_id","message":"SMTP relay: AUTH PLAIN attempt"}
{"level":"info","workspace_id":"your_workspace_id","message":"SMTP relay: Authentication successful"}
{"level":"info","workspace_id":"your_workspace_id","notification_id":"password_reset","message":"SMTP relay: Notification sent successfully"}
```

## 📝 Example Notification Payloads

### Simple Notification

```json
{
  "notification": {
    "id": "welcome_email",
    "contact": {
      "email": "user@example.com"
    }
  }
}
```

### With Contact Details

```json
{
  "notification": {
    "id": "password_reset",
    "contact": {
      "email": "user@example.com",
      "first_name": "John",
      "last_name": "Doe"
    },
    "data": {
      "reset_token": "abc123",
      "expires_in": "1 hour"
    }
  }
}
```

### With CC, BCC, Reply-To

```json
{
  "notification": {
    "id": "order_confirmation",
    "contact": {
      "email": "customer@example.com"
    },
    "data": {
      "order_id": "12345"
    },
    "email_options": {
      "cc": ["manager@example.com"],
      "bcc": ["archive@example.com"],
      "reply_to": "support@example.com"
    }
  }
}
```

### Email with Headers

You can also use email headers (CC, BCC, Reply-To) instead of JSON:

```bash
swaks --to customer@example.com \
  --from noreply@example.com \
  --server localapi.notifuse.com:587 \
  --tls \
  --tls-ca-path ./dev-certs/localapi.notifuse.com.cert.pem \
  --auth-user workspace_id \
  --auth-password "api-key" \
  --header "Cc: manager@example.com" \
  --header "Bcc: archive@example.com" \
  --header "Reply-To: support@example.com" \
  --body '{"notification": {"id": "order_confirmation", "contact": {"email": "customer@example.com"}}}'
```

**Note**: JSON `email_options` take precedence over email headers.

## 🔧 Troubleshooting

### Connection Refused

**Problem**: Can't connect to port 587

**Solutions**:
1. Check SMTP relay is enabled: `grep SMTP_RELAY_ENABLED .env`
2. Verify server is running: `lsof -i :587`
3. Check firewall: `sudo pfctl -sr | grep 587` (macOS)

### Certificate Verification Failed

**Problem**: TLS handshake error

**Solutions**:
1. Use `--tls-ca-path` pointing to the cert file
2. Or disable verification (testing only): `--tls-verify=no`
3. Check domain matches: must use `localapi.notifuse.com`, not `localhost`

### Authentication Failed

**Problem**: 535 authentication failed

**Solutions**:
1. Verify workspace ID is correct
2. Check API key is valid JWT token (not expired)
3. Ensure JWT secret in `.env` matches the one used to generate API key
4. Check workspace exists and user has access

### Name Resolution Failed

**Problem**: Can't resolve `localapi.notifuse.com`

**Solutions**:
1. Add to `/etc/hosts`: `127.0.0.1 localapi.notifuse.com`
2. Verify with: `ping localapi.notifuse.com`
3. Flush DNS cache: `sudo dscacheutil -flushcache` (macOS)

### Invalid JSON

**Problem**: Failed to extract JSON payload

**Solutions**:
1. Ensure entire email body is valid JSON
2. Use `Content-Type: application/json` header
3. Test JSON validity: `echo '{"notification": {...}}' | jq`
4. Check for hidden characters or encoding issues

## 📚 Additional Resources

- [SMTP Relay Implementation](SMTP_RELAY_IMPLEMENTATION.md)
- [Full Test Suite](tests/integration/smtp_relay_e2e_test.go)
- [Environment Variables](env.example)
- [Certificate Details](dev-certs/README.md)

## 🔒 Security Notes

⚠️ **Development Only**
- Self-signed certificates are NOT trusted by default
- Never use these certificates in production
- The `dev-certs/` directory is in `.gitignore`

For production:
- Use Let's Encrypt or other trusted CA
- See [env.example](env.example) for production TLS configuration
- Use proper DNS and MX records
- Configure SPF, DKIM, and DMARC

## 🎯 What's Next?

1. ✅ Certificates generated
2. ✅ Domain added to hosts file
3. ✅ Environment configured
4. 🚀 Run `make dev`
5. 🧪 Test with swaks
6. 🎉 Start building!

---

**Generated**: $(date +%Y-%m-%d)
**Domain**: localapi.notifuse.com
**Validity**: 365 days

