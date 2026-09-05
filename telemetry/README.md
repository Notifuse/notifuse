# Notifuse Telemetry Cloud Function

This Google Cloud Function receives anonymous telemetry data from the Notifuse platform and logs it to Google Cloud Logging for further analysis and monitoring.

## Overview

The telemetry function is designed to:

- Receive HTTP POST requests with JSON telemetry data
- Validate and parse the incoming payload
- Log structured data to Google Cloud Logging with appropriate labels
- Provide CORS support for web-based requests
- Return appropriate HTTP responses

The telemetry data includes integration usage as boolean flags for each email provider, extracted directly from workspace configuration rather than database queries for improved performance. `web_analytics` is the exception: it reports whether the workspace actually recorded a web analytics session in the last 30 days, since the feature can be switched on and never wired up. Only the boolean is sent, never the date.

## Telemetry Data Structure

The function expects JSON payloads matching the following structure:

```json
{
  "workspace_id_sha1": "a1b2c3d4e5f6...",
  "workspace_created_at": "2023-01-15T10:30:00Z",
  "workspace_updated_at": "2025-08-15T14:22:30Z",
  "last_message_at": "2025-08-20T09:45:12Z",
  "contacts_count": 1500,
  "broadcasts_count": 25,
  "transactional_count": 150,
  "messages_count": 5000,
  "lists_count": 10,
  "segments_count": 8,
  "users_count": 5,
  "blog_posts_count": 12,
  "api_endpoint": "https://api.example.com",
  "mailgun": true,
  "amazonses": true,
  "mailjet": false,
  "sparkpost": false,
  "postmark": false,
  "smtp": false,
  "s3": false,
  "anthropic": true,
  "openai": false,
  "gemini": false,
  "supabase": false,
  "firecrawl": true,
  "web_analytics": true,
  "ses_tenant": true,
  "rbac_custom": true,
  "version": "40.0",
  "oidc_enabled": true,
  "license_tier": "agency"
}
```

`ses_tenant` and `rbac_custom` describe the workspace; `version`, `oidc_enabled` and `license_tier` describe the sending installation and therefore repeat identically on every row an instance sends, the same way `api_endpoint` already does. None of them carries an identifier: no tenant name, no issuer, no licence key, no organisation.

## Adding a field

A field has to survive four declarations to reach an analyst — the sender's struct in `internal/service/telemetry_service.go`, `TelemetryMetrics` here, `LogEntry` here, and a column in `bigquery_schema.json` — and nothing links them at compile time. A field missing from either of the last two is discarded in silence: the sender still gets its 200 and the function logs no error. `TestLogEntryMatchesBigQuerySchema` in `main_test.go` compares the struct against the schema file in both directions so that omission is a red test instead of a column of NULLs found months later.

`bigquery_schema.json` is the desired shape of the table, not something applied automatically. Adding columns to an existing BigQuery table is additive and non-destructive, but somebody has to run it:

```bash
bq update --schema bigquery_schema.json <project>:<dataset>.<table>
```

Existing rows keep NULL for the new columns, which is correct — those instances never sent the field. So do the rows that keep arriving from senders older than the column, which is the part that took a fix rather than a sentence: a field absent from the payload has to stay absent from the row.

That is why the five v40 fields — `ses_tenant`, `rbac_custom`, `version`, `oidc_enabled`, `license_tier` — are pointers with `omitempty` on both structs in `main.go`. As plain `bool`/`string` they decoded to `false`/`""` and were **written** as `false`/`""`, indistinguishable from an upgraded instance that genuinely has SSO off and no custom RBAC. During a rollout most senders are on the old version, so a query like `countif(oidc_enabled)` would have counted every un-upgraded instance — including every one running SSO — in the `false` bucket, and the number would have read as small.

The sender never omits these fields (`internal/service/telemetry_service.go` carries no `omitempty`), so an unlicensed v40 instance still reports `license_tier: ""` explicitly and still lands as an empty string. **NULL means the sender never spoke; a value means it answered.** `TestPreV40PayloadLeavesTheNewColumnsNull` and `TestExplicitFalseFromAV40SenderIsNotDroppedAsAbsent` pin both halves — the second matters because the tempting shortcut, `omitempty` on plain values, would sweep a genuine `false` away with the absent ones.

A new field added later inherits the same rule: pointer, `omitempty`, both structs.

## Deployment

### Prerequisites

1. Google Cloud SDK (`gcloud`) installed and configured
2. Go 1.21 or later for local development
3. Appropriate Google Cloud permissions for Cloud Functions and Logging

### Using the Deploy Script

```bash
# Make the script executable (if not already)
chmod +x deploy.sh

# Deploy to your project
./deploy.sh YOUR_PROJECT_ID us-central1

# Or use default values and update them in the script
./deploy.sh
```

### Manual Deployment

```bash
gcloud functions deploy notifuse-telemetry \
  --gen2 \
  --runtime=go121 \
  --region=us-central1 \
  --source=. \
  --entry-point=ReceiveTelemetry \
  --trigger=http \
  --allow-unauthenticated \
  --memory=256MB \
  --timeout=30s \
  --max-instances=10 \
  --project=YOUR_PROJECT_ID
```

## Testing

### Using curl

```bash
# Get the function URL
FUNCTION_URL=$(gcloud functions describe notifuse-telemetry \
  --region=us-central1 \
  --project=YOUR_PROJECT_ID \
  --format="value(serviceConfig.uri)")

# Send test data
curl -X POST "$FUNCTION_URL" \
  -H "Content-Type: application/json" \
  -d '{
    "workspace_id_sha1": "test123abc",
    "contacts_count": 100,
    "broadcasts_count": 5,
    "transactional_count": 20,
    "messages_count": 500,
    "lists_count": 3,
    "segments_count": 6,
    "users_count": 2,
    "blog_posts_count": 8,
    "api_endpoint": "https://api.test.com",
    "mailgun": true,
    "amazonses": false,
    "mailjet": false,
    "sparkpost": false,
    "postmark": false,
    "smtp": false,
    "s3": false,
    "anthropic": true,
    "openai": false,
    "gemini": false,
    "supabase": false,
    "firecrawl": true,
    "web_analytics": true
  }'
```

### Expected Response

```json
{
  "status": "success",
  "message": "Telemetry data received and logged",
  "timestamp": "2024-01-15T10:30:45Z"
}
```

## Monitoring and Logs

### Cloud Logging

The function creates structured logs in Google Cloud Logging with:

- **Log Name**: `telemetry`
- **Severity**: `INFO`
- **Labels**: `workspace_id_sha1`, `event_type`, `source`

### Viewing Logs

```bash
# View function logs
gcloud logging read "resource.type=cloud_function AND resource.labels.function_name=notifuse-telemetry" \
  --project=YOUR_PROJECT_ID \
  --limit=10 \
  --format=json

# View telemetry-specific logs
gcloud logging read "logName:telemetry AND labels.event_type=telemetry_metrics" \
  --project=YOUR_PROJECT_ID \
  --limit=10 \
  --format=json
```

### Log Analysis Queries

Use these Cloud Logging queries for analysis:

```sql
-- Count telemetry events by workspace
resource.type="cloud_function"
logName="projects/YOUR_PROJECT_ID/logs/telemetry"
jsonPayload.event_type="telemetry_metrics"

-- Find high-activity workspaces
resource.type="cloud_function"
logName="projects/YOUR_PROJECT_ID/logs/telemetry"
jsonPayload.contacts_count > 1000

-- Monitor integration usage
resource.type="cloud_function"
logName="projects/YOUR_PROJECT_ID/logs/telemetry"
jsonPayload.mailgun=true

-- Count workspaces using multiple integrations
resource.type="cloud_function"
logName="projects/YOUR_PROJECT_ID/logs/telemetry"
(jsonPayload.mailgun=true OR jsonPayload.amazonses=true OR jsonPayload.mailjet=true)

-- Find workspaces actively collecting web analytics
resource.type="cloud_function"
logName="projects/YOUR_PROJECT_ID/logs/telemetry"
jsonPayload.web_analytics=true
```

## Security

- The function accepts unauthenticated requests (suitable for telemetry)
- All workspace IDs are SHA1 hashed for anonymity
- CORS headers are configured to allow cross-origin requests
- No sensitive data is logged or stored

## Configuration

### Environment Variables

- `GCP_PROJECT`: Set automatically during deployment

### Function Settings

- **Runtime**: Go 1.21
- **Memory**: 256MB
- **Timeout**: 30 seconds
- **Max Instances**: 10
- **Trigger**: HTTP

## Development

### Local Testing

```bash
# Install dependencies
go mod tidy

# Run tests (if any)
go test ./...

# Local development with Functions Framework
go run cmd/main.go
```

### File Structure

```
telemetry/
├── main.go              # Main function code
├── go.mod              # Go module definition
├── deploy.sh           # Deployment script
├── function.yaml       # Configuration reference
├── .gcloudignore      # Files to ignore during deployment
└── README.md          # This documentation
```

## Troubleshooting

### Common Issues

1. **Deployment fails**: Check that you have the necessary IAM permissions
2. **Function times out**: Increase timeout in deploy script
3. **Logs not appearing**: Verify the logging client initialization
4. **CORS errors**: Check that CORS headers are properly set

### Debug Commands

```bash
# Check function status
gcloud functions describe notifuse-telemetry --region=us-central1

# View function logs
gcloud functions logs read notifuse-telemetry --region=us-central1

# Test function locally
curl -X POST http://localhost:8080 -H "Content-Type: application/json" -d '{"test": "data"}'
```

## Contributing

When modifying this function:

1. Update the telemetry data structure if needed
2. Ensure proper error handling and logging
3. Test with sample data before deployment
4. Update this README with any configuration changes
