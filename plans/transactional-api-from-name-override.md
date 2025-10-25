# Transactional API: From Name Override Feature

## Overview

Add support for an optional `from_name` parameter in the `EmailOptions` of the transactional API, allowing users to override the default sender from name configured in the email provider settings on a per-message basis.

## Current Behavior

Currently, the transactional API determines the sender's name (`from_name`) from:
1. The template's configured sender ID (`template.Email.SenderID`)
2. Looking up the sender in the email provider's list of senders
3. Using `emailSender.Name` as the from name

This is hardcoded and cannot be overridden at send time.

## Desired Behavior

Allow users to optionally override the sender's display name when sending a transactional notification by providing a `from_name` field in the `EmailOptions` parameter of the API request.

### API Request Example

```json
{
  "workspace_id": "workspace123",
  "notification": {
    "id": "welcome-email",
    "contact": {
      "email": "user@example.com",
      "first_name": "John"
    },
    "channels": ["email"],
    "email_options": {
      "from_name": "John's Personal Assistant",
      "reply_to": "support@example.com",
      "cc": ["manager@example.com"],
      "bcc": []
    },
    "data": {
      "product_name": "Premium Plan"
    }
  }
}
```

## Implementation Plan

### 1. Domain Layer Changes

**File**: `internal/domain/email_provider.go`

**Changes**:
- Add `FromName` field to `EmailOptions` struct
- Make the field optional with `omitempty` JSON tag
- Field should be of type `*string` to distinguish between "not provided" and "empty string"

**Updated Structure**:
```go
type EmailOptions struct {
    FromName    *string      `json:"from_name,omitempty"`    // NEW: Override default sender from name
    CC          []string     `json:"cc,omitempty"`
    BCC         []string     `json:"bcc,omitempty"`
    ReplyTo     string       `json:"reply_to,omitempty"`
    Attachments []Attachment `json:"attachments,omitempty"`
}
```

**Rationale**:
- Using `*string` allows us to distinguish between:
  - `nil` (not provided, use default)
  - `""` (empty string, explicitly set to empty)
  - `"Custom Name"` (override with this value)

**Test File**: `internal/domain/email_provider_test.go`
- Test EmailOptions JSON marshaling/unmarshaling with from_name
- Test nil vs empty string vs provided value handling

---

### 2. Service Layer Changes

**File**: `internal/service/email_service.go`

**Method**: `SendEmailForTemplate` (starting at line 225)

**Changes**:
- After determining the default `fromName` from the email sender (line 338)
- Check if `request.EmailOptions.FromName` is provided
- If provided, use the override instead of the default

**Updated Logic** (around line 336-339):
```go
// Get necessary email information from the template
fromEmail := emailSender.Email
fromName := emailSender.Name

// NEW: Allow override of from name via email options
if request.EmailOptions.FromName != nil && *request.EmailOptions.FromName != "" {
    fromName = *request.EmailOptions.FromName
}
```

**Logging**:
- Add debug log when from_name is overridden
- Include both default and override values for traceability

**Example Log**:
```go
if request.EmailOptions.FromName != nil && *request.EmailOptions.FromName != "" {
    s.logger.WithFields(map[string]interface{}{
        "message_id":           request.MessageID,
        "default_from_name":    emailSender.Name,
        "override_from_name":   *request.EmailOptions.FromName,
    }).Debug("Using from_name override")
    fromName = *request.EmailOptions.FromName
}
```

**Test File**: `internal/service/email_service_test.go`
- Test SendEmailForTemplate with no from_name override (existing behavior)
- Test SendEmailForTemplate with from_name override
- Test SendEmailForTemplate with empty string from_name override
- Test SendEmailForTemplate with nil from_name (should use default)

**Method**: `TestTemplate` (starting at line 650)

**Changes**:
- Similar logic to handle from_name override in test template functionality
- After getting the email sender (line 689-693)
- Check if `emailOptions.FromName` is provided and override if present

**Updated Logic** (around line 767-786):
```go
// Create SendEmailProviderRequest
emailRequest := domain.SendEmailProviderRequest{
    WorkspaceID:   workspaceID,
    IntegrationID: integrationID,
    MessageID:     messageID,
    FromAddress:   emailSender.Email,
    FromName:      emailSender.Name,
    To:            recipientEmail,
    Subject:       processedSubject,
    Content:       *compiledResult.HTML,
    Provider:      emailProvider,
    EmailOptions:  emailOptions,
}

// NEW: Allow override of from name via email options
if emailOptions.FromName != nil && *emailOptions.FromName != "" {
    emailRequest.FromName = *emailOptions.FromName
}
```

**Test File**: `internal/service/transactional_service_test.go`
- Test TestTemplate with no from_name override
- Test TestTemplate with from_name override

---

### 3. HTTP Layer Changes

**File**: `internal/http/transactional_handler.go`

**Method**: `handleSend` (starting at line 222)

**Changes**:
- No changes required in this file
- The handler already properly decodes `EmailOptions` from the request
- The validation already checks CC, BCC, and ReplyTo
- Optional: Add validation for from_name if needed (e.g., length limits)

**Optional Enhancement**:
Add validation in `SendTransactionalRequest.Validate()` in `internal/domain/transactional.go` (line 349):
```go
// Validate from_name if provided (optional - can enforce length limits)
if req.Notification.EmailOptions.FromName != nil && 
   len(*req.Notification.EmailOptions.FromName) > 255 {
    return NewValidationError("from_name exceeds maximum length of 255 characters")
}
```

**Test File**: `internal/http/transactional_handler_test.go`
- Test handleSend with from_name in email_options
- Test handleSend without from_name (existing behavior)
- Test handleSend with empty string from_name
- Test validation of from_name if length validation is added

**Method**: `handleTestTemplate` (starting at line 262)

**Changes**:
- No changes required
- Already properly decodes EmailOptions from TestTemplateRequest
- The service layer changes will handle the override

---

### 4. Documentation Changes

**File**: `openapi.json`

**Changes**:
- Update the `EmailOptions` schema to include the new `from_name` field
- Document it as optional with description

**Schema Update**:
```json
"EmailOptions": {
  "type": "object",
  "properties": {
    "from_name": {
      "type": "string",
      "description": "Override the default sender from name. If not provided, uses the sender's configured name.",
      "example": "John's Support Team"
    },
    "cc": {
      "type": "array",
      "items": {
        "type": "string"
      },
      "description": "Carbon copy recipients"
    },
    "bcc": {
      "type": "array",
      "items": {
        "type": "string"
      },
      "description": "Blind carbon copy recipients"
    },
    "reply_to": {
      "type": "string",
      "description": "Reply-to email address"
    }
  }
}
```

**File**: `README.md` or API Documentation

**Changes**:
- Add example showing the from_name override in the transactional API section
- Document the precedence: override > template sender > default

---

### 5. Testing Strategy

#### Unit Tests

**Domain Layer** (`internal/domain/email_provider_test.go`):
- Test `EmailOptions` struct with from_name field
- Test JSON marshaling/unmarshaling
- Test nil, empty, and provided values

**Service Layer** (`internal/service/email_service_test.go`):
- Test `SendEmailForTemplate` with from_name override
- Test `SendEmailForTemplate` without override (default behavior)
- Test `SendEmailForTemplate` with empty string override
- Mock template service and workspace repository
- Verify the correct from_name is passed to SendEmail

**Service Layer** (`internal/service/transactional_service_test.go`):
- Test `SendNotification` with from_name in EmailOptions
- Test `TestTemplate` with from_name override
- Verify EmailOptions are properly propagated through the service layers

**HTTP Layer** (`internal/http/transactional_handler_test.go`):
- Test `handleSend` endpoint with from_name in request body
- Test `handleTestTemplate` endpoint with from_name
- Test request validation with various from_name values

#### Integration Tests

**File**: `tests/integration/transactional_test.go`

**Test Scenarios**:
1. **Send transactional email with from_name override**
   - Create a transactional notification
   - Send it with custom from_name in email_options
   - Verify email was sent with the overridden from_name
   - Check message history contains correct metadata

2. **Send transactional email without from_name override**
   - Create a transactional notification
   - Send it without from_name in email_options
   - Verify email was sent with default sender name
   - Ensure backward compatibility

3. **Test template with from_name override**
   - Test a template with custom from_name
   - Verify test email uses the override

4. **Multiple sends with different from_name values**
   - Send same notification multiple times
   - Each time with different from_name
   - Verify each message uses the correct override

#### Test Execution Commands

After implementation, run the following test commands:

```bash
# Run domain layer tests
make test-domain

# Run service layer tests  
make test-service

# Run HTTP handler tests
make test-http

# Run all unit tests
make test-unit

# Run integration tests
make test-integration

# Generate coverage report
make coverage
```

---

### 6. Implementation Steps

1. **Step 1: Update Domain Layer**
   - Add `FromName *string` field to `EmailOptions` struct
   - Write unit tests for the updated struct
   - Run domain tests: `make test-domain`

2. **Step 2: Update Service Layer - Email Service**
   - Modify `SendEmailForTemplate` method to check for from_name override
   - Modify `TestTemplate` method similarly
   - Add logging for override traceability
   - Write unit tests for email service
   - Run service tests: `make test-service`

3. **Step 3: Update Service Layer - Transactional Service**
   - Verify EmailOptions are properly passed through
   - Write unit tests for transactional service
   - Run service tests: `make test-service`

4. **Step 4: Update HTTP Layer (Optional Validation)**
   - Add validation for from_name if desired
   - Write handler tests
   - Run HTTP tests: `make test-http`

5. **Step 5: Integration Testing**
   - Write integration tests covering all scenarios
   - Run integration tests: `make test-integration`
   - Verify backward compatibility

6. **Step 6: Documentation**
   - Update OpenAPI spec
   - Update README or API documentation
   - Add code examples

7. **Step 7: Manual Testing**
   - Test with real email provider (SES, SMTP, etc.)
   - Verify email headers show correct from_name
   - Test with various email clients (Gmail, Outlook, etc.)

8. **Step 8: Run Full Test Suite**
   - Execute `make test-unit` to run all unit tests
   - Execute `make test-integration` for integration tests
   - Execute `make coverage` to ensure adequate coverage
   - Verify minimum 75% test coverage is maintained

---

## Edge Cases to Consider

1. **Empty String Override**: 
   - If `from_name: ""` is provided, should we use empty string or default?
   - Decision: Use empty string if explicitly provided (user intent)

2. **Very Long From Names**:
   - Email standards recommend max 78 characters per line
   - Consider adding validation for max length (e.g., 255 characters)

3. **Special Characters**:
   - From names can contain Unicode characters
   - Email providers should handle encoding
   - Test with non-ASCII characters (émojis, accents, etc.)

4. **Null vs Undefined**:
   - JSON: `{"from_name": null}` vs not including the field
   - Both should use the default sender name

5. **Provider-Specific Behavior**:
   - Some providers (SES, SMTP, Mailgun, etc.) may format from_name differently
   - Test with multiple providers to ensure consistency

---

## Backward Compatibility

This feature is fully backward compatible:

- Existing API calls without `from_name` continue to work exactly as before
- The field is optional (`omitempty`)
- Default behavior (using sender's configured name) is preserved
- No database schema changes required
- No migration needed

---

## Security Considerations

1. **Email Spoofing Prevention**:
   - The from_name is only the display name, not the email address
   - The actual from_address (email) remains unchanged and verified
   - Email providers' SPF/DKIM/DMARC validations still apply

2. **Input Validation**:
   - Consider sanitizing from_name to prevent injection attacks
   - Limit length to prevent abuse
   - Reject potentially malicious characters if needed

3. **Audit Trail**:
   - Log when from_name is overridden for audit purposes
   - Include both default and override values in logs
   - Message history should reflect actual sent email parameters

---

## Performance Impact

- **Minimal**: Only adds a simple conditional check
- **No database impact**: No additional queries required
- **No latency**: Negligible performance overhead

---

## Rollout Plan

1. **Development**: Implement changes following the steps above
2. **Testing**: Comprehensive unit and integration tests
3. **Staging**: Deploy to staging environment for manual testing
4. **Production**: Deploy with feature flag (optional)
5. **Monitoring**: Watch for errors or unexpected behavior
6. **Documentation**: Update public API documentation

---

## Success Metrics

- All unit tests pass (75%+ coverage maintained)
- All integration tests pass
- Backward compatibility verified (existing tests still pass)
- Manual testing confirms correct from_name in sent emails
- No performance degradation observed

---

## Future Enhancements

1. **From Address Override**: 
   - Similarly allow overriding the from email address
   - Requires additional validation and provider configuration

2. **Template-Level Default Override**:
   - Allow templates to specify a default from_name override
   - API-level override would take precedence

3. **Workspace-Level Settings**:
   - Allow workspace settings to enforce or restrict from_name overrides
   - Could prevent spoofing in multi-tenant scenarios

---

## Files Modified Summary

### Core Changes (Required)
- `internal/domain/email_provider.go` - Add FromName to EmailOptions
- `internal/service/email_service.go` - Implement override logic in SendEmailForTemplate and TestTemplate

### Test Files (Required)
- `internal/domain/email_provider_test.go` - Domain tests
- `internal/service/email_service_test.go` - Service tests
- `internal/service/transactional_service_test.go` - Service tests
- `internal/http/transactional_handler_test.go` - Handler tests
- `tests/integration/transactional_test.go` - Integration tests

### Optional Changes
- `internal/domain/transactional.go` - Add validation for from_name length
- `openapi.json` - Update API documentation
- `README.md` - Add usage examples

---

## Estimated Effort

- **Implementation**: 2-3 hours
- **Testing**: 2-3 hours
- **Documentation**: 1 hour
- **Total**: 5-7 hours

---

## Implementation Checklist

- [ ] Update `EmailOptions` struct in `internal/domain/email_provider.go`
- [ ] Write domain layer tests for EmailOptions
- [ ] Update `SendEmailForTemplate` in `internal/service/email_service.go`
- [ ] Update `TestTemplate` in `internal/service/email_service.go`
- [ ] Add logging for from_name override
- [ ] Write email service unit tests
- [ ] Write transactional service unit tests
- [ ] (Optional) Add validation for from_name in request validation
- [ ] Write HTTP handler tests
- [ ] Write integration tests for all scenarios
- [ ] Run `make test-unit` and verify all tests pass
- [ ] Run `make test-integration` and verify all tests pass
- [ ] Run `make coverage` and verify coverage is adequate
- [ ] Test manually with real email provider
- [ ] Verify email headers in sent emails
- [ ] Update OpenAPI specification
- [ ] Update README or API documentation
- [ ] Add code examples
- [ ] Create pull request with comprehensive description
