-- Fix Script: Set Default Names for Senders with Empty Names
-- This script updates senders with empty names to use their email as the name
-- 
-- CAUTION: This modifies data. Test in a non-production environment first!
-- 
-- Usage:
--   1. Review the audit results from audit_sender_names.sql
--   2. Backup your database: pg_dump -d notifuse > backup_before_fix.sql
--   3. Run this script
--   4. Verify the changes with audit_sender_names.sql again

BEGIN;

-- Create a backup table (optional but recommended)
CREATE TEMP TABLE workspace_integrations_backup AS
SELECT id, name, integrations, updated_at
FROM workspaces
WHERE integrations IS NOT NULL;

-- Update function to fix sender names
DO $$
DECLARE
    workspace_record RECORD;
    updated_integrations JSONB;
    integration JSONB;
    sender JSONB;
    senders_array JSONB;
    new_senders JSONB;
    updated_count INTEGER := 0;
BEGIN
    -- Loop through all workspaces with integrations
    FOR workspace_record IN 
        SELECT id, name, integrations 
        FROM workspaces 
        WHERE integrations IS NOT NULL 
          AND jsonb_array_length(integrations) > 0
    LOOP
        updated_integrations := '[]'::jsonb;
        
        -- Loop through each integration
        FOR integration IN SELECT * FROM jsonb_array_elements(workspace_record.integrations)
        LOOP
            -- Check if this integration has an email_provider with senders
            IF integration ? 'email_provider' AND integration->'email_provider' ? 'senders' THEN
                new_senders := '[]'::jsonb;
                
                -- Loop through each sender
                FOR sender IN SELECT * FROM jsonb_array_elements(integration->'email_provider'->'senders')
                LOOP
                    -- If sender name is empty or null, set it to the email address
                    IF sender->>'name' = '' OR sender->>'name' IS NULL THEN
                        sender := jsonb_set(
                            sender,
                            '{name}',
                            to_jsonb(sender->>'email'),
                            true
                        );
                        
                        RAISE NOTICE 'Fixed sender in workspace % (integration %): email=%, set name=%',
                            workspace_record.name,
                            integration->>'name',
                            sender->>'email',
                            sender->>'name';
                        
                        updated_count := updated_count + 1;
                    END IF;
                    
                    -- Add sender to new array
                    new_senders := new_senders || sender;
                END LOOP;
                
                -- Update the senders array in the email_provider
                integration := jsonb_set(
                    integration,
                    '{email_provider,senders}',
                    new_senders,
                    true
                );
            END IF;
            
            -- Add integration to updated array
            updated_integrations := updated_integrations || integration;
        END LOOP;
        
        -- Update the workspace with fixed integrations
        IF updated_integrations != workspace_record.integrations THEN
            UPDATE workspaces
            SET 
                integrations = updated_integrations,
                updated_at = NOW()
            WHERE id = workspace_record.id;
        END IF;
    END LOOP;
    
    RAISE NOTICE 'Total senders updated: %', updated_count;
END $$;

-- Verify the changes
SELECT 
    'After Fix: Senders without names' as status,
    COUNT(*) as count
FROM workspaces w,
    jsonb_array_elements(w.integrations) as integration,
    jsonb_array_elements(integration->'email_provider'->'senders') as sender
WHERE sender->>'name' = '' OR sender->>'name' IS NULL;

-- If everything looks good, commit the transaction
-- Otherwise, use ROLLBACK;

-- COMMIT;
ROLLBACK; -- Change to COMMIT after testing
