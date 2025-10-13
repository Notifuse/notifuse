# ✓ Sender Name Fix - Action Checklist

Print this page and check off each step as you complete it.

---

## Phase 1: Investigation & Diagnosis

### Understanding the Problem
- [ ] Read `SENDER_NAME_FIX_README.md` (5 min)
- [ ] Read `SESSION_SUMMARY.md` for overview (3 min)
- [ ] Understand that this is a DATA issue, not a CODE issue

### Running Diagnostics
- [ ] Check if you have the issue:
  ```bash
  make -f Makefile.audit quick-check
  ```
  Result: _____ senders with empty names

- [ ] Run detailed audit:
  ```bash
  make -f Makefile.audit audit-senders
  ```
  
- [ ] Note critical issues:
  - [ ] Marketing provider affected? Yes ☐ No ☐
  - [ ] Transactional provider affected? Yes ☐ No ☐
  - [ ] Number of critical issues: _____

### Review Results
- [ ] Identify which workspaces are affected
- [ ] Determine if this explains your email issues
- [ ] Decision: Proceed with fix? Yes ☐ No ☐

---

## Phase 2: Preparation

### Backup (CRITICAL - DO NOT SKIP)
- [ ] Create database backup:
  ```bash
  pg_dump $DATABASE_URL > backup_$(date +%Y%m%d_%H%M%S).sql
  ```
  Backup file: ________________________________

- [ ] Verify backup file exists and has content:
  ```bash
  ls -lh backup_*.sql
  ```
  File size: _______ (should be > 0 KB)

- [ ] Store backup in safe location
  Location: ________________________________

### Test Environment (Optional but Recommended)
- [ ] Create test database
- [ ] Copy production data to test
- [ ] Run fix on test database
- [ ] Verify results on test database
- [ ] If satisfied, proceed to production

---

## Phase 3: Applying the Fix

### Run the Fix
- [ ] Double-check backup exists
- [ ] Run automated fix:
  ```bash
  make -f Makefile.audit fix-senders
  ```

- [ ] Confirm when prompted (2 confirmations required)

- [ ] Review the output logs
  - [ ] Number of senders updated: _____
  - [ ] Any errors? Yes ☐ No ☐
  - [ ] If errors, note them: ________________________________

### Verify the Fix
- [ ] Run audit again:
  ```bash
  make -f Makefile.audit audit-senders
  ```

- [ ] Confirm results:
  - [ ] Empty sender names remaining: _____
  - [ ] Should be 0 if fix was successful

- [ ] Check a few examples in the database:
  ```sql
  SELECT id, name, integrations 
  FROM workspaces 
  LIMIT 5;
  ```

---

## Phase 4: Testing

### Send Test Emails
- [ ] Go to Settings → Integrations in UI
- [ ] Select an integration that was fixed
- [ ] Click "Test Email Provider"
- [ ] Enter your email address
- [ ] Send test email

- [ ] Check received email:
  - [ ] From header shows name? Yes ☐ No ☐
  - [ ] Format is: "Name" <email@example.com>? Yes ☐ No ☐

### Run Unit Tests
- [ ] Run the test suite:
  ```bash
  make -f Makefile.audit test-from-name
  ```
  Result: PASS ☐ FAIL ☐

---

## Phase 5: Cleanup & Improvement

### Update Sender Names (Optional)
For each sender that was auto-fixed with its email as the name:

- [ ] Workspace: _________________
  - [ ] Integration: _________________
  - [ ] Update sender name to something user-friendly
  - [ ] Save changes

### Documentation
- [ ] Document what was fixed
  Date: ________________
  Senders fixed: ________
  Issues found: _________

- [ ] Share findings with team if applicable

### Monitoring Setup (Optional)
- [ ] Add monitoring query (see `DIAGNOSTIC_TOOLS.md`)
- [ ] Set up alert for empty sender names
- [ ] Schedule periodic audits

---

## Phase 6: Verification & Sign-off

### Final Checks
- [ ] No critical issues remain:
  ```bash
  make -f Makefile.audit audit-senders
  ```
  Exit code: _____ (should be 0)

- [ ] Test emails show From names correctly
- [ ] No errors in application logs
- [ ] All tests passing

### Archive & Document
- [ ] Keep backup file for 30 days
  Location: ________________________________

- [ ] Document the fix in your change log
- [ ] Update any relevant documentation

- [ ] Mark as resolved in issue tracker (if applicable)
  Issue #: ________________

---

## ✅ Completion

### Sign-off
- [ ] Investigation complete
- [ ] Fix applied successfully
- [ ] Tests passing
- [ ] No outstanding issues

**Completed by:** ____________________  
**Date:** ____________________  
**Time spent:** ____________________

### Notes & Observations
```
[Space for any notes, issues encountered, or lessons learned]







```

---

## 📞 Emergency Rollback

If something goes wrong:

### Step 1: Stop the Application
```bash
# Stop your application
systemctl stop notifuse  # or however you stop it
```

### Step 2: Restore from Backup
```bash
# Restore database
psql $DATABASE_URL < backup_YYYYMMDD_HHMMSS.sql
```

### Step 3: Verify Restoration
```bash
# Check data is restored
psql $DATABASE_URL -c "SELECT COUNT(*) FROM workspaces;"
```

### Step 4: Restart Application
```bash
# Restart your application
systemctl start notifuse
```

### Step 5: Report Issue
- [ ] Note what went wrong: ________________________________
- [ ] Review logs: ________________________________
- [ ] Contact support if needed

---

## 📚 Reference Quick Links

- **Start Here:** `SENDER_NAME_FIX_README.md`
- **Investigation Report:** `SMTP_FROM_NAME_INVESTIGATION.md`
- **Tool Guide:** `DIAGNOSTIC_TOOLS.md`
- **Session Summary:** `SESSION_SUMMARY.md`
- **File Index:** `INDEX_SENDER_NAME_FIX.md`

---

## 🎯 Success Criteria

Check all that apply:
- [ ] No senders with empty names in active providers
- [ ] Test emails show proper From names
- [ ] All unit tests passing
- [ ] No errors in application logs
- [ ] Team aware of the fix
- [ ] Documentation updated
- [ ] Monitoring in place (optional)

**If all boxes checked: SUCCESS! 🎉**

---

*Print this checklist and check off items as you complete them*
