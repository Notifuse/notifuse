package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	_ "github.com/lib/pq"
)

// AuditResult holds the results of the sender audit
type AuditResult struct {
	WorkspaceID      string
	WorkspaceName    string
	IntegrationID    string
	IntegrationName  string
	SenderEmail      string
	SenderName       string
	IsDefault        bool
	HasEmptyName     bool
	IsActiveProvider bool
	ProviderType     string
}

func main() {
	// Load configuration
	cfg, err := config.LoadConfig(".")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Connect to database
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Run audit
	results, err := auditSenders(ctx, db)
	if err != nil {
		log.Fatalf("Audit failed: %v", err)
	}

	// Display results
	displayResults(results)
}

func auditSenders(ctx context.Context, db *sql.DB) ([]AuditResult, error) {
	query := `
		SELECT 
			w.id,
			w.name,
			w.settings,
			w.integrations
		FROM workspaces w
		WHERE w.integrations IS NOT NULL
		  AND jsonb_array_length(w.integrations) > 0
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query workspaces: %w", err)
	}
	defer rows.Close()

	var results []AuditResult

	for rows.Next() {
		var workspaceID, workspaceName string
		var settingsJSON, integrationsJSON []byte

		err := rows.Scan(&workspaceID, &workspaceName, &settingsJSON, &integrationsJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Parse settings
		var settings domain.WorkspaceSettings
		if err := json.Unmarshal(settingsJSON, &settings); err != nil {
			log.Printf("Warning: Failed to parse settings for workspace %s: %v", workspaceID, err)
			continue
		}

		// Parse integrations
		var integrations []domain.Integration
		if err := json.Unmarshal(integrationsJSON, &integrations); err != nil {
			log.Printf("Warning: Failed to parse integrations for workspace %s: %v", workspaceID, err)
			continue
		}

		// Audit each integration
		for _, integration := range integrations {
			if integration.Type != domain.IntegrationTypeEmail {
				continue
			}

			// Check if this integration is actively used
			isMarketingProvider := settings.MarketingEmailProviderID != nil && 
				*settings.MarketingEmailProviderID == integration.ID
			isTransactionalProvider := settings.TransactionalEmailProviderID != nil && 
				*settings.TransactionalEmailProviderID == integration.ID

			// Audit each sender
			for _, sender := range integration.EmailProvider.Senders {
				result := AuditResult{
					WorkspaceID:      workspaceID,
					WorkspaceName:    workspaceName,
					IntegrationID:    integration.ID,
					IntegrationName:  integration.Name,
					SenderEmail:      sender.Email,
					SenderName:       sender.Name,
					IsDefault:        sender.IsDefault,
					HasEmptyName:     sender.Name == "",
					IsActiveProvider: isMarketingProvider || isTransactionalProvider,
				}

				if isMarketingProvider {
					result.ProviderType = "Marketing"
				} else if isTransactionalProvider {
					result.ProviderType = "Transactional"
				} else {
					result.ProviderType = "Unused"
				}

				// Only include problematic senders or active providers
				if result.HasEmptyName || result.IsActiveProvider {
					results = append(results, result)
				}
			}
		}
	}

	return results, nil
}

func displayResults(results []AuditResult) {
	if len(results) == 0 {
		fmt.Println("✅ No issues found! All senders have names.")
		return
	}

	// Count issues
	emptyNameCount := 0
	criticalIssues := 0 // Empty names in active providers

	for _, result := range results {
		if result.HasEmptyName {
			emptyNameCount++
			if result.IsActiveProvider {
				criticalIssues++
			}
		}
	}

	// Print summary
	fmt.Println("=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=")
	fmt.Println("SENDER NAME AUDIT RESULTS")
	fmt.Println("=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=" + "=")
	fmt.Printf("\nTotal issues found: %d\n", emptyNameCount)
	fmt.Printf("Critical issues (active providers): %d\n\n", criticalIssues)

	// Print critical issues first
	if criticalIssues > 0 {
		fmt.Println("🚨 CRITICAL: Active Providers with Empty Sender Names")
		fmt.Println("-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-")
		for _, result := range results {
			if result.HasEmptyName && result.IsActiveProvider {
				fmt.Printf("❌ Workspace: %s (%s)\n", result.WorkspaceName, result.WorkspaceID)
				fmt.Printf("   Integration: %s (%s) [%s]\n", result.IntegrationName, result.IntegrationID, result.ProviderType)
				fmt.Printf("   Sender: %s\n", result.SenderEmail)
				fmt.Printf("   Name: '%s' (EMPTY)\n", result.SenderName)
				if result.IsDefault {
					fmt.Printf("   ⚠️  This is the DEFAULT sender\n")
				}
				fmt.Println()
			}
		}
	}

	// Print non-critical issues
	nonCriticalCount := emptyNameCount - criticalIssues
	if nonCriticalCount > 0 {
		fmt.Println("\n⚠️  Unused Integrations with Empty Sender Names")
		fmt.Println("-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-")
		for _, result := range results {
			if result.HasEmptyName && !result.IsActiveProvider {
				fmt.Printf("⚠️  Workspace: %s (%s)\n", result.WorkspaceName, result.WorkspaceID)
				fmt.Printf("   Integration: %s (%s) [%s]\n", result.IntegrationName, result.IntegrationID, result.ProviderType)
				fmt.Printf("   Sender: %s (Name: '%s')\n", result.SenderEmail, result.SenderName)
				fmt.Println()
			}
		}
	}

	// Print recommendations
	fmt.Println("\n📋 RECOMMENDATIONS")
	fmt.Println("-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-" + "-")
	if criticalIssues > 0 {
		fmt.Println("1. Fix critical issues immediately - these affect actively used email providers")
		fmt.Println("2. Run: scripts/fix_empty_sender_names.sql (after backup!)")
	}
	if nonCriticalCount > 0 {
		fmt.Println("3. Consider fixing unused integrations for consistency")
	}
	fmt.Println("4. Verify all senders have meaningful names in the UI")
	fmt.Println("5. Re-run this audit after making changes")

	// Exit with error code if critical issues found
	if criticalIssues > 0 {
		os.Exit(1)
	}
}
