# SMTP Relay Authentication

## Authentication Flow

The SMTP relay uses a modified authentication scheme where:

### Credentials

- **Username**: API key email (e.g., `api@yourdomain.com`)
- **Password**: API key JWT token

### How It Works

1. **Client sends AUTH PLAIN** with email and JWT token
2. **Server validates JWT token**:
   - Extracts `user_id`, `email`, and `type` from claims
   - Verifies token signature and expiration
   - Validates `type` is `api_key`
   - **Verifies email in token matches the username**
3. **Server returns `user_id`** for the session
4. **Email body must include `workspace_id`**:
   ```json
   {
     "workspace_id": "workspace_abc123",
     "notification": {
       "id": "password_reset",
       "contact": {
         "email": "user@example.com"
       }
     }
   }
   ```
5. **Server verifies workspace access**:
   - Checks if `user_id` has access to the specified `workspace_id`
   - Validates permissions
6. **Notification is triggered** for the workspace

## Why This Design?

### Email as Username

- **More intuitive**: SMTP traditionally uses email addresses
- **Human-readable**: Easy to identify which API key is being used
- **Secure**: Email is embedded in the JWT and verified

### workspace_id in JSON Payload

- **Flexibility**: One API key could potentially access multiple workspaces (future feature)
- **Explicit**: Clear which workspace the notification should be sent from
- **Validated**: Server verifies access before processing

### JWT Includes Email

The API key JWT token now includes the email claim:

```go
claims := UserClaims{
    UserID: user.ID,
    Email:  user.Email, // ← Added for SMTP auth
    Type:   string(domain.UserTypeAPIKey),
    RegisteredClaims: jwt.RegisteredClaims{
        ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 24 * 365 * 10)),
        IssuedAt:  jwt.NewNumericDate(time.Now()),
        NotBefore: jwt.NewNumericDate(time.Now()),
    },
}
```

## Example Usage

### Creating an API Key

```bash
# Via API (returns email and token)
curl -X POST https://api.yourdomain.com/api/workspaces.createAPIKey \
  -H "Authorization: Bearer YOUR_AUTH_TOKEN" \
  -d '{
    "workspace_id": "workspace_abc123",
    "email_prefix": "smtp"
  }'

# Response:
{
  "email": "smtp@yourdomain.com",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

### Sending via SMTP

```bash
swaks --to test@example.com \
  --from sender@example.com \
  --server mail.yourdomain.com:587 \
  --tls \
  --auth-user "smtp@yourdomain.com" \
  --auth-password "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
  --body '{
    "workspace_id": "workspace_abc123",
    "notification": {
      "id": "password_reset",
      "contact": {
        "email": "user@example.com"
      },
      "data": {
        "reset_token": "abc123"
      }
    }
  }'
```

## JSON Payload Structure

### Required Fields

```json
{
  "workspace_id": "workspace_abc123", // ← REQUIRED
  "notification": {
    "id": "notification_template_id", // ← REQUIRED
    "contact": {
      "email": "user@example.com" // ← REQUIRED
    }
  }
}
```

### Full Example

```json
{
  "workspace_id": "workspace_abc123",
  "notification": {
    "id": "order_confirmation",
    "contact": {
      "email": "customer@example.com",
      "first_name": "John",
      "last_name": "Doe"
    },
    "data": {
      "order_id": "12345",
      "total": "99.99",
      "items": [{ "name": "Product A", "quantity": 2 }]
    },
    "email_options": {
      "cc": ["manager@example.com"],
      "bcc": ["archive@example.com"],
      "reply_to": "support@example.com"
    },
    "metadata": {
      "source": "smtp_relay",
      "campaign": "order_notifications"
    }
  }
}
```

## Security Features

1. **JWT Validation**: Full signature and expiration checks
2. **Email Verification**: Username must match email in JWT
3. **Type Checking**: Token must be of type `api_key`
4. **Workspace Access**: User must have access to specified workspace
5. **TLS Required**: PLAIN auth only allowed after STARTTLS

## Error Messages

### Authentication Errors

| Error                        | Cause                    | Solution                         |
| ---------------------------- | ------------------------ | -------------------------------- |
| `invalid API key`            | JWT parsing failed       | Check token format and secret    |
| `invalid API key token`      | Token validation failed  | Check expiration and signature   |
| `token must be an API key`   | Wrong user type          | Use an API key, not a user token |
| `email does not match token` | Username != email in JWT | Use the correct email address    |

### Message Processing Errors

| Error                                      | Cause                    | Solution                   |
| ------------------------------------------ | ------------------------ | -------------------------- |
| `workspace_id is required in JSON payload` | Missing workspace_id     | Add workspace_id to JSON   |
| `user does not have access to workspace`   | Invalid workspace access | Check workspace membership |
| `email body is not valid JSON`             | Malformed JSON           | Validate JSON syntax       |
| `notification.id is required`              | Missing notification ID  | Add notification.id        |
| `notification.contact.email is required`   | Missing contact email    | Add contact.email          |

## Testing

### Unit Tests

Tests are in `internal/service/smtp_relay_handler_test.go`

### E2E Tests

Tests are in `tests/integration/smtp_relay_e2e_test.go`

Run tests:

```bash
# Unit tests
go test ./internal/service -run "TestSMTPRelay" -v

# E2E tests
go test ./tests/integration -run "TestSMTPRelayE2E" -v
```

### Manual Testing

Use the test scripts:

```bash
# Simple test
./scripts/test-smtp-relay.sh "api@yourdomain.com" "YOUR_JWT_TOKEN"

# Advanced test suite
./scripts/test-smtp-relay-advanced.sh "api@yourdomain.com" "YOUR_JWT_TOKEN"
```

## Migration Notes

### Breaking Changes

- **Username changed**: From `workspace_id` to `api_email`
- **JSON payload**: Now requires `workspace_id` field
- **JWT claims**: Now includes `email` field

### Updating Existing Integrations

1. Get the API key email from your workspace settings
2. Update SMTP username from workspace ID to email
3. Add `workspace_id` to your JSON payload:
   ```json
   {
     "workspace_id": "your_workspace_id",  // ← ADD THIS
     "notification": { ... }
   }
   ```

### Backwards Compatibility

**None**. This is a breaking change. All existing SMTP relay integrations must be updated.

## See Also

- [SMTP Relay Implementation](SMTP_RELAY_IMPLEMENTATION.md)
- [Setup Local SMTP](SETUP_LOCAL_SMTP.md)
- [Test Scripts](scripts/README.md)

---

**Last Updated**: November 5, 2025  
**Breaking Change**: Yes (from workspace_id auth to email auth)
