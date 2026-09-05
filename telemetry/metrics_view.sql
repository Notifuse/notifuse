-- notifusev3.telemetry.metrics_view — the view analysts read over the Log Router shards.
-- This file is the source of truth for its definition; apply with:
--   bq update --use_legacy_sql=false --view "$(sed '/^--/d' telemetry/metrics_view.sql)" notifusev3:telemetry.metrics_view
-- Last applied: 2026-09-05.
SELECT 
  timestamp,
  jsonPayload.workspace_id_sha1 as workspace_id_sha1,
  CASE 
    WHEN JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.workspace_created_at') IS NOT NULL 
    THEN PARSE_TIMESTAMP('%Y-%m-%dT%H:%M:%SZ', JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.workspace_created_at'))
    ELSE NULL 
  END as workspace_created_at,
  CASE 
    WHEN JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.workspace_updated_at') IS NOT NULL 
    THEN PARSE_TIMESTAMP('%Y-%m-%dT%H:%M:%SZ', JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.workspace_updated_at'))
    ELSE NULL 
  END as workspace_updated_at,
  CASE 
    WHEN JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.last_message_at') IS NOT NULL 
    THEN PARSE_TIMESTAMP('%Y-%m-%dT%H:%M:%SZ', JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.last_message_at'))
    ELSE NULL 
  END as last_message_at,
  CAST(jsonPayload.contacts_count AS INT64) as contacts_count,
  CAST(jsonPayload.broadcasts_count AS INT64) as broadcasts_count,
  CAST(jsonPayload.transactional_count AS INT64) as transactional_count,
  CAST(jsonPayload.messages_count AS INT64) as messages_count,
  CAST(jsonPayload.lists_count AS INT64) as lists_count,
  CASE 
    WHEN JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.users_count') IS NOT NULL 
    THEN CAST(JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.users_count') AS INT64)
    ELSE NULL 
  END as users_count,
  jsonPayload.api_endpoint as api_endpoint,
  jsonPayload.source as source,
  jsonPayload.event_type as event_type,
  CAST(jsonPayload.mailgun AS BOOL) as mailgun,
  CAST(jsonPayload.amazonses AS BOOL) as amazonses,
  CAST(jsonPayload.mailjet AS BOOL) as mailjet,
  CAST(jsonPayload.sendgrid AS BOOL) as sendgrid,
  CAST(jsonPayload.postmark AS BOOL) as postmark,
  CAST(jsonPayload.smtp AS BOOL) as smtp,
  CAST(jsonPayload.s3 AS BOOL) as s3,
  CAST(jsonPayload.blog_posts_count AS INT64) as blog_posts_count,
  CAST(jsonPayload.web_analytics AS BOOL) as web_analytics,
  -- Read through JSON rather than as jsonPayload.<field>: a date shard is created from the first
  -- entry of its day and grows as fields arrive, so a direct reference to a field the newest shard
  -- does not carry yet fails the whole view. JSON_EXTRACT_SCALAR answers NULL instead.
  SAFE_CAST(JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.segments_count') AS INT64) as segments_count,
  SAFE_CAST(JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.sparkpost') AS BOOL) as sparkpost,
  SAFE_CAST(JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.anthropic') AS BOOL) as anthropic,
  SAFE_CAST(JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.openai') AS BOOL) as openai,
  SAFE_CAST(JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.gemini') AS BOOL) as gemini,
  SAFE_CAST(JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.supabase') AS BOOL) as supabase,
  SAFE_CAST(JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.firecrawl') AS BOOL) as firecrawl,
  -- v40: absent from every row a pre-v40 instance sends, by design (see telemetry/README.md)
  SAFE_CAST(JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.ses_tenant') AS BOOL) as ses_tenant,
  SAFE_CAST(JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.rbac_custom') AS BOOL) as rbac_custom,
  JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.version') as version,
  SAFE_CAST(JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.oidc_enabled') AS BOOL) as oidc_enabled,
  JSON_EXTRACT_SCALAR(TO_JSON_STRING(jsonPayload), '$.license_tier') as license_tier
FROM `notifusev3.telemetry.telemetry_*`
WHERE jsonPayload.event_type = 'telemetry_metrics'