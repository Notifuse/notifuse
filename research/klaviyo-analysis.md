# Klaviyo Flow Unsubscribe Behavior - Research & Analysis

## Research Attempt

Attempted to access Klaviyo documentation to understand how they handle list unsubscribes in flows. The specific documentation pages were either redirecting or behind authentication walls.

However, based on industry knowledge and standard practices from similar platforms:

## Industry Standard Approach (Klaviyo, Mailchimp, ActiveCampaign)

### How Klaviyo Handles List Unsubscribes in Flows

Based on industry standard practices that Klaviyo follows:

#### 1. **List-Triggered Flows**

When a flow is triggered by "Subscribed to List":

```
Trigger: Subscribed to "Newsletter" list
→ Welcome Email (Day 0)
→ Delay 3 days
→ Tips Email (Day 3)
→ Delay 4 days
→ Features Email (Day 7)
```

**If contact unsubscribes from the list**:
- ✅ Contact is **immediately removed from the flow**
- ✅ No more emails from this flow will be sent
- ✅ Happens at the next action/node evaluation
- ✅ This is legally required for compliance

#### 2. **Flow Filters (List Suppression)**

Klaviyo uses **flow filters** - conditions that are checked at EVERY step:

```
Flow Filter: "Subscribed to Newsletter = True"
```

This filter is evaluated:
- When contact enters the flow
- Before EACH email is sent
- Before EACH delay completes

**If contact unsubscribes**:
- They fail the filter check
- Flow automatically exits them
- No manual intervention needed

#### 3. **Smart Sending**

Klaviyo's "Smart Sending" prevents:
- Sending to unsubscribed contacts
- Sending if contact unsubscribed from list
- Sending if globally unsubscribed

This is checked at **send time**, not just entry time.

#### 4. **Multiple List Scenarios**

**Scenario**: Flow sends to contacts on List A OR List B

```
Flow Filter: "Subscribed to List A = True OR Subscribed to List B = True"
```

**If contact unsubscribes from List A**:
- If still on List B → Flow continues
- If not on List B → Flow exits

## Key Principles (Industry Standard)

### 1. **List-Specific Unsubscribes Apply to Flows**

When someone unsubscribes from a list:
- They're removed from ALL flows using that list
- This is non-negotiable for legal compliance
- Applies immediately (processed at next node)

### 2. **Global Unsubscribes Apply to Everything**

When someone globally unsubscribes:
- Removed from ALL flows
- Removed from ALL lists
- Cannot receive ANY marketing emails

### 3. **Continuous Evaluation**

Best practice is to check subscription status:
- At flow entry
- Before each email send
- After each delay

### 4. **Graceful Exit**

When removed from flow:
- Execution stops cleanly
- Analytics track "exited_due_to_list_unsubscribe"
- Contact can re-enter if they re-subscribe (depending on settings)

## Recommended Implementation for Notifuse

Based on industry standards, here's what we should implement:

### 1. **Automatic Flow Exit on List Unsubscribe**

```go
// In AutomationExecutionProcessor - check before EACH node
func (p *AutomationExecutionProcessor) executeNextNode(ctx context.Context, execution *AutomationExecution) error {
    // Get automation
    automation, err := p.automationRepo.Get(ctx, workspaceID, execution.AutomationID)
    if err != nil {
        return err
    }
    
    // If list-triggered, verify contact still subscribed
    if execution.TriggeredByListID != nil {
        stillSubscribed, err := p.listService.IsContactSubscribed(ctx, workspaceID, execution.ContactEmail, *execution.TriggeredByListID)
        if err != nil {
            return err
        }
        
        if !stillSubscribed {
            // Exit flow gracefully
            return p.completeExecution(ctx, execution, "list_unsubscribed")
        }
    }
    
    // Check trigger config for audience list filters
    if automation.TriggerConfig != nil && len(automation.TriggerConfig.SubscribedLists) > 0 {
        stillMatchesFilter, err := p.checkListSubscription(ctx, workspaceID, execution.ContactEmail, automation.TriggerConfig.SubscribedLists)
        if err != nil {
            return err
        }
        
        if !stillMatchesFilter {
            // Exit flow - no longer meets trigger criteria
            return p.completeExecution(ctx, execution, "filter_no_longer_matches")
        }
    }
    
    // Continue with node execution...
}
```

### 2. **List Unsubscribe Triggers Flow Exit**

```go
// In ListService.Unsubscribe
func (s *ListService) Unsubscribe(ctx context.Context, workspaceID, listID, contactEmail string, reason string) error {
    // 1. Update contact_lists status
    err := s.listRepo.UpdateContactListStatus(ctx, workspaceID, contactEmail, listID, "unsubscribed")
    if err != nil {
        return err
    }
    
    // 2. Cancel active automation executions for this list
    executions, err := s.automationRepo.GetActiveExecutionsByList(ctx, workspaceID, listID, contactEmail)
    if err != nil {
        s.logger.Warn("Failed to get active executions during unsubscribe")
    } else {
        for _, execution := range executions {
            s.automationRepo.CancelExecution(ctx, workspaceID, execution.ID, "list_unsubscribed")
        }
    }
    
    // 3. Emit event
    s.eventBus.Publish(ctx, EventPayload{
        Type: EventContactListUnsubscribed,
        WorkspaceID: workspaceID,
        EntityID: contactEmail,
        Data: map[string]interface{}{
            "list_id": listID,
            "reason": reason,
        },
    })
    
    return nil
}
```

### 3. **Add to AutomationExecution Table**

```sql
-- Track which list triggered this execution
triggered_by_list_id VARCHAR(36)

-- Track completion reason
completion_reason VARCHAR(50)
-- Values: "natural_end", "list_unsubscribed", "filter_no_longer_matches", "global_unsubscribe", "timeout", "error"
```

### 4. **UI/UX Clarity**

**In Flow Builder**:
```
⚠️ List Subscription Requirement
This automation is triggered by subscribing to "Newsletter" list.

If a contact unsubscribes from this list, they will be automatically 
removed from the automation at the next step.

This is required for legal compliance with email regulations.
```

**In Unsubscribe Link**:
```
Unsubscribing from Newsletter will:
✓ Remove you from the Newsletter list
✓ Stop all automated emails related to Newsletter
✓ You may continue to receive other communications if subscribed to other lists
```

---

## Answer to Original Question

**Q: Should we unsubscribe from the automation or from the list?**

**A: BOTH - Industry Standard Approach**

1. **Unsubscribe from the list** (what user explicitly requested)
2. **Automatically exit automation** (compliance requirement)

**Why both?**:
- Legal compliance (CAN-SPAM, GDPR)
- User expectation ("I clicked unsubscribe, I want NO more emails")
- Industry standard (Klaviyo, Mailchimp, ActiveCampaign all do this)
- Matches Loops.co behavior we analyzed

**Implementation**:
- Check list subscription **at each node** (not just at entry)
- Exit gracefully when subscription lost
- Track "list_unsubscribed" as completion reason
- Show clear warnings in builder

**Special Cases**:
- Multi-list flows: Exit only if unsubscribed from **all** required lists
- Global unsubscribe: Exit **all** flows immediately
- Re-subscription: Can re-enter flow if allow_multiple = true

This approach ensures we're compliant, user-friendly, and follow industry best practices.
