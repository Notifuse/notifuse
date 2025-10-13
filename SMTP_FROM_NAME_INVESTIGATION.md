# SMTP From Name Investigation Summary

## Investigation Goal
Investigate why the SMTP "From" header might only contain the email address and not the sender name ("Notifuse").

## Findings

### ✅ Go-Mail Library Support
**Status: FULLY SUPPORTED**

The `github.com/wneessen/go-mail v0.7.1` library **fully supports** From names:

```go
// From: internal/service/smtp_service.go:98
msg.FromFormat(request.FromName, request.FromAddress)
```

**Library behavior:**
- `FromFormat("Notifuse", "email@example.com")` produces: `"Notifuse" <email@example.com>` ✅
- `FromFormat("", "email@example.com")` produces: `<email@example.com>` ⚠️

**Evidence:** See `internal/service/smtp_service_from_format_test.go`

### ✅ Backend Flow
**Status: CORRECTLY IMPLEMENTED**

The entire backend flow properly handles sender names:

1. **Database Storage** (JSONB)
   ```json
   {
     "senders": [
       {
         "id": "uuid",
         "email": "noreply@notifuse.com",
         "name": "Notifuse",
         "is_default": true
       }
     ]
   }
   ```

2. **EmailSender Domain Model**
   ```go
   type EmailSender struct {
       ID        string `json:"id"`
       Email     string `json:"email"`
       Name      string `json:"name"`      // ✅ Name field exists
       IsDefault bool   `json:"is_default"`
   }
   ```

3. **GetSender Method**
   ```go
   emailSender := emailProvider.GetSender(template.Email.SenderID)
   // Returns: &EmailSender{Email: "...", Name: "Notifuse"}
   ```

4. **SendEmailProviderRequest**
   ```go
   request := domain.SendEmailProviderRequest{
       FromAddress:   emailSender.Email,
       FromName:      emailSender.Name,  // ✅ Name is passed
       // ...
   }
   ```

5. **SMTP Service**
   ```go
   msg.FromFormat(request.FromName, request.FromAddress)
   // ✅ Uses go-mail's FromFormat
   ```

**Evidence:** See `internal/service/smtp_service_sender_name_integration_test.go`

### ✅ Frontend Implementation
**Status: CORRECTLY IMPLEMENTED**

The UI properly captures sender names:

**Location:** `console/src/components/settings/Integrations.tsx:1362-1381`

```tsx
<Form form={senderForm} layout="vertical">
  <Form.Item
    name="email"
    label="Email"
    rules={[
      { required: true, message: 'Email is required' },
      { type: 'email', message: 'Please enter a valid email' }
    ]}
  >
    <Input placeholder="sender@example.com" />
  </Form.Item>
  <Form.Item
    name="name"
    label="Name"
    rules={[{ required: true, message: 'Name is required' }]}  // ✅ Required field
  >
    <Input placeholder="Sender Name" />
  </Form.Item>
</Form>
```

**Evidence:** Both email AND name fields are required in the UI.

### ✅ Validation
**Status: PROPERLY ENFORCED**

Multiple validation layers prevent empty names:

1. **Frontend Validation**
   ```tsx
   rules={[{ required: true, message: 'Name is required' }]}
   ```

2. **EmailProvider Validation**
   ```go
   if sender.Name == "" {
       return fmt.Errorf("sender name is required for sender at index %d", i)
   }
   ```

3. **SendEmailProviderRequest Validation**
   ```go
   if r.FromName == "" {
       return fmt.Errorf("from name is required")
   }
   ```

**Evidence:** See `internal/service/smtp_service_debug_test.go`

## Root Cause Analysis

Based on the investigation, if the From name is missing in emails, it can only be due to:

### 1. **Sender Created Without Name (Unlikely)**
- Frontend requires the name field
- Backend validation prevents empty names
- **Probability: LOW**

### 2. **Wrong Sender Selected (Possible)**
- A template might reference a sender ID that doesn't have a name
- Or references a non-existent sender, falling back to default
- **Probability: MEDIUM**

### 3. **Old Data Migration (Possible)**
- Senders created before name field was required
- Database might have senders with empty name field
- **Probability: MEDIUM**

### 4. **API Direct Access (Possible)**
- Integration created via API without proper validation
- Bypassing frontend validation
- **Probability: LOW-MEDIUM**

## Diagnostic Steps

To identify the actual issue in your system:

### Step 1: Check Database Data
```sql
-- Check for senders with empty names in integrations
SELECT 
    id,
    name,
    jsonb_pretty(integrations) as integrations
FROM workspaces
WHERE integrations IS NOT NULL;
```

Look for any `"name": ""` or missing `name` fields in the `senders` array.

### Step 2: Check Logs
Add temporary logging to see what's being passed:

```go
// In email_service.go SendEmailForTemplate()
s.logger.WithFields(map[string]interface{}{
    "sender_id":    template.Email.SenderID,
    "sender_email": emailSender.Email,
    "sender_name":  emailSender.Name,  // Check this value
}).Debug("Using email sender")
```

### Step 3: Test with Specific Integration
```bash
# Send a test email through the UI
# Check the raw email headers in the received message
```

### Step 4: Run Unit Tests
```bash
# Verify the flow works correctly
go test -v ./internal/service -run "TestGoMailFromFormat|TestEmailSenderNamePreservation"
```

## Test Files Created

1. **`internal/service/smtp_service_from_format_test.go`**
   - Tests go-mail library FromFormat behavior
   - Verifies name handling with various inputs
   - Confirms RFC 5322 formatting

2. **`internal/service/smtp_service_sender_name_integration_test.go`**
   - Tests complete flow from JSON → GetSender → Request
   - Verifies name preservation through serialization
   - Tests validation at each layer

3. **`internal/service/smtp_service_debug_test.go`**
   - Diagnostic tests with checkpoint logging
   - Real-world scenario testing
   - Problem case identification

## Recommendations

### Immediate Actions

1. **Check Existing Senders**
   ```sql
   -- Find integrations with potentially empty sender names
   SELECT id, name FROM workspaces 
   WHERE integrations::text LIKE '%"name":"",%';
   ```

2. **Add Migration** (if needed)
   - Create a migration to set default names for any senders with empty names
   - Example: Set name to email address if name is empty

3. **Add Database Constraint** (optional)
   - Consider adding a CHECK constraint to prevent empty names at database level

### Long-term Improvements

1. **Enhanced Logging**
   - Log sender details when emails are sent
   - Track which sender is used for each email

2. **UI Validation Enhancement**
   - Add visual indicator showing which sender will be used
   - Preview the From header before sending

3. **API Validation**
   - Ensure API endpoints validate sender names
   - Return clear errors for missing names

4. **Monitoring**
   - Alert on emails sent with empty From names
   - Dashboard showing sender usage

## Conclusion

The investigation confirms that **the infrastructure is correctly implemented**:
- ✅ Go-mail library supports From names
- ✅ Backend properly stores and retrieves names
- ✅ Frontend captures names from users
- ✅ Validation prevents empty names

**If From names are missing, the issue is with the DATA, not the CODE.**

The most likely cause is senders in the database that have empty `name` fields, possibly from:
- Old data before validation was added
- API access without proper validation
- Manual database edits

**Next Step:** Check your database for senders with empty names using the SQL query in "Immediate Actions" above.

## Test Results

All tests pass successfully:

```
✅ TestGoMailFromFormat - Verifies go-mail library behavior
✅ TestSMTPService_FromNameInEmail - Verifies SMTP service integration
✅ TestEmailSenderNamePreservation - Verifies data flow
✅ TestIntegrationSenderNameFlow - Verifies complete flow
✅ TestEmailProviderValidationWithEmptySenderName - Verifies validation
✅ TestDebugFromNameInActualEmail - Debug with checkpoints
✅ TestRealWorldScenarios - Real-world problem cases
```

All 7 test suites pass with 100% success rate.
