-- Audit Script: Find Senders with Missing Names
-- This script helps identify integrations with senders that have empty or missing names

-- 1. Find all workspaces with integrations
SELECT 
    'Total workspaces with integrations' as check_type,
    COUNT(*) as count
FROM workspaces
WHERE integrations IS NOT NULL 
  AND jsonb_array_length(integrations) > 0;

-- 2. Find integrations with empty sender names
SELECT 
    'Integrations with empty sender names' as check_type,
    COUNT(*) as count
FROM workspaces w,
    jsonb_array_elements(w.integrations) as integration,
    jsonb_array_elements(integration->'email_provider'->'senders') as sender
WHERE sender->>'name' = '' OR sender->>'name' IS NULL;

-- 3. Detailed list of workspaces with problematic senders
SELECT 
    w.id as workspace_id,
    w.name as workspace_name,
    integration->>'id' as integration_id,
    integration->>'name' as integration_name,
    sender->>'id' as sender_id,
    sender->>'email' as sender_email,
    sender->>'name' as sender_name,
    CASE 
        WHEN sender->>'name' = '' THEN 'EMPTY STRING'
        WHEN sender->>'name' IS NULL THEN 'NULL'
        ELSE 'OK'
    END as name_status,
    sender->>'is_default' as is_default
FROM workspaces w,
    jsonb_array_elements(w.integrations) as integration,
    jsonb_array_elements(integration->'email_provider'->'senders') as sender
WHERE sender->>'name' = '' OR sender->>'name' IS NULL
ORDER BY w.id, integration->>'id', sender->>'email';

-- 4. Count senders by status
SELECT 
    CASE 
        WHEN sender->>'name' = '' THEN 'Empty String'
        WHEN sender->>'name' IS NULL THEN 'NULL'
        WHEN length(sender->>'name') > 0 THEN 'Has Name'
        ELSE 'Unknown'
    END as name_status,
    COUNT(*) as sender_count
FROM workspaces w,
    jsonb_array_elements(w.integrations) as integration,
    jsonb_array_elements(integration->'email_provider'->'senders') as sender
GROUP BY name_status
ORDER BY sender_count DESC;

-- 5. Find default senders with missing names (HIGH PRIORITY)
SELECT 
    w.id as workspace_id,
    w.name as workspace_name,
    integration->>'id' as integration_id,
    integration->>'name' as integration_name,
    sender->>'email' as sender_email,
    sender->>'name' as sender_name,
    '⚠️ DEFAULT SENDER WITH NO NAME' as warning
FROM workspaces w,
    jsonb_array_elements(w.integrations) as integration,
    jsonb_array_elements(integration->'email_provider'->'senders') as sender
WHERE (sender->>'name' = '' OR sender->>'name' IS NULL)
  AND (sender->>'is_default')::boolean = true
ORDER BY w.id;

-- 6. Check if marketing/transactional providers have senders with names
SELECT 
    w.id as workspace_id,
    w.name as workspace_name,
    'Marketing Provider' as provider_type,
    w.settings->>'marketing_email_provider_id' as provider_id,
    integration->>'name' as integration_name,
    COUNT(sender) FILTER (WHERE sender->>'name' = '' OR sender->>'name' IS NULL) as senders_without_names,
    COUNT(sender) as total_senders
FROM workspaces w
JOIN LATERAL (
    SELECT jsonb_array_elements(w.integrations) as integration
) i ON true
JOIN LATERAL (
    SELECT jsonb_array_elements(i.integration->'email_provider'->'senders') as sender
) s ON true
WHERE w.settings->>'marketing_email_provider_id' = i.integration->>'id'
GROUP BY w.id, w.name, provider_id, integration_name

UNION ALL

SELECT 
    w.id as workspace_id,
    w.name as workspace_name,
    'Transactional Provider' as provider_type,
    w.settings->>'transactional_email_provider_id' as provider_id,
    integration->>'name' as integration_name,
    COUNT(sender) FILTER (WHERE sender->>'name' = '' OR sender->>'name' IS NULL) as senders_without_names,
    COUNT(sender) as total_senders
FROM workspaces w
JOIN LATERAL (
    SELECT jsonb_array_elements(w.integrations) as integration
) i ON true
JOIN LATERAL (
    SELECT jsonb_array_elements(i.integration->'email_provider'->'senders') as sender
) s ON true
WHERE w.settings->>'transactional_email_provider_id' = i.integration->>'id'
GROUP BY w.id, w.name, provider_id, integration_name

ORDER BY workspace_id, provider_type;
