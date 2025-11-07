# Web Publications v17 - Unit Testing Plan

**Date**: November 7, 2025  
**Status**: Testing Required

---

## Overview

This document outlines the unit testing requirements for the Web Publications v17 implementation. Files are prioritized by importance and complexity.

---

## 🔴 Priority 1: Critical New Files (No Tests Yet)

### 1. `internal/migrations/v17.go` ✅ HAS TESTS

**Status**: Migration tests already exist in `v17_test.go`  
**Coverage**: System validated by migration framework

### 2. `pkg/notifuse_mjml/filter.go` ⚠️ NEEDS TESTS

**Status**: No tests  
**Complexity**: Medium  
**Priority**: High

**Required Tests**:

```go
// pkg/notifuse_mjml/filter_test.go

TestFilterBlocksByChannel_EmailOnly
  - Section with visibility="email_only"
  - Should appear in email channel
  - Should be filtered out in web channel

TestFilterBlocksByChannel_WebOnly
  - Section with visibility="web_only"
  - Should appear in web channel
  - Should be filtered out in email channel

TestFilterBlocksByChannel_All
  - Section with visibility="all" or nil
  - Should appear in both channels

TestFilterBlocksByChannel_Nested
  - Nested sections with different visibility
  - Should recursively filter correctly

TestFilterBlocksByChannel_DeepCopy
  - Original tree should not be modified
  - Filtered tree should be independent copy

TestFilterBlocksByChannel_InvalidChannel
  - Handle invalid channel parameter gracefully

TestFilterBlocksByChannel_EmptyTree
  - Handle nil or empty tree
```

**Estimated**: 7 test cases, ~150 lines

---

### 3. `internal/service/dns_verification_service.go` ⚠️ NEEDS TESTS

**Status**: No tests  
**Complexity**: High (network I/O)  
**Priority**: Critical (security)

**Required Tests**:

```go
// internal/service/dns_verification_service_test.go

TestVerifyDomainOwnership_ValidCNAME
  - Domain with correct CNAME pointing to expected target
  - Should return nil (success)

TestVerifyDomainOwnership_InvalidCNAME
  - Domain pointing to wrong target
  - Should return error with clear message

TestVerifyDomainOwnership_NoCNAME
  - Domain with no CNAME record
  - Should return error asking to configure CNAME

TestVerifyDomainOwnership_InvalidURL
  - Malformed domain URL
  - Should return parse error

TestVerifyDomainOwnership_DNSLookupFailure
  - Network error during DNS lookup
  - Should return descriptive error

TestVerifyTXTRecord_Valid
  - Domain with correct verification TXT record
  - Should return nil

TestVerifyTXTRecord_Invalid
  - Domain without matching TXT record
  - Should return error

TestVerifyTXTRecord_NoRecords
  - Domain with no TXT records
  - Should return error
```

**Mocking Strategy**: Use test DNS resolver or mock net.LookupCNAME  
**Estimated**: 8 test cases, ~200 lines

---

### 4. `internal/service/web_publication_service.go` ⚠️ NEEDS TESTS

**Status**: No tests  
**Complexity**: High  
**Priority**: Critical

**Required Tests**:

```go
// internal/service/web_publication_service_test.go

TestNewPostCache
  - Create cache
  - Verify cleanup goroutine starts

TestPostCache_GetSet
  - Set cached post
  - Get within TTL -> returns cached
  - Get after TTL -> returns false

TestPostCache_Invalidate
  - Set cached post
  - Invalidate
  - Get -> returns false

TestGetPublishedPosts_Success
  - Returns paginated posts
  - Returns list menu
  - Pagination metadata correct

TestGetPublishedPosts_Empty
  - No published posts
  - Returns empty array, not error

TestGetPublishedPostsByList_Success
  - Returns posts for specific list
  - Filters correctly by list ID

TestGetPublishedPostsByList_ListNotFound
  - Invalid list slug
  - Returns error

TestGetPostBySlug_CacheHit
  - Post in cache
  - Returns from cache without DB query

TestGetPostBySlug_CacheMiss
  - Post not in cache
  - Fetches from DB, caches result

TestGetPostBySlug_NotPublished
  - Broadcast with web_published_at = NULL
  - Returns error

TestGetPostBySlug_Future
  - Broadcast with web_published_at in future
  - Returns error

TestRenderWebContent_Success
  - Compiles template with web channel
  - Returns HTML

TestInvalidateCache
  - Clears specific post from cache

TestGetPublishedLists_WithCounts
  - Returns all published lists
  - Post counts correct (batch query)

TestGetPublishedLists_Empty
  - No published lists
  - Returns empty array
```

**Estimated**: 15 test cases, ~400 lines

---

### 5. `internal/http/web_publication_handler.go` ⚠️ NEEDS TESTS

**Status**: No tests  
**Complexity**: High  
**Priority**: High

**Required Tests**:

```go
// internal/http/web_publication_handler_test.go

TestNewWebPublicationHandler
  - Creates handler
  - Templates loaded

TestHandleList_Success
  - GET /
  - Returns 200
  - HTML contains posts

TestHandleListByList_Success
  - GET /{list-slug}
  - Returns posts for that list

TestHandleListByList_ListNotFound
  - GET /invalid-list
  - Returns 404

TestHandlePost_Success
  - GET /{list-slug}/{post-slug}
  - Returns 200
  - HTML contains post content

TestHandlePost_NotFound
  - GET /{list-slug}/invalid-post
  - Returns 404

TestHandleRobots
  - GET /robots.txt
  - Returns plain text
  - Contains sitemap URL

TestHandleSitemap_Success
  - GET /sitemap.xml
  - Returns XML
  - Contains all lists and posts

TestHandleSitemap_Empty
  - No published lists
  - Returns minimal sitemap

TestHandle404
  - Invalid path
  - Returns 404 page

TestHandle_Routing
  - Correct method dispatched for each path
  - / -> handleListAll
  - /{list} -> handleListByList
  - /{list}/{post} -> handlePost
  - /robots.txt -> handleRobots
  - /sitemap.xml -> handleSitemap
```

**Estimated**: 12 test cases, ~300 lines

---

## 🟡 Priority 2: Modified Files Needing Test Updates

### 6. `internal/domain/broadcast.go` ⚠️ UPDATE TESTS

**Status**: Tests exist in `broadcast_test.go`  
**Changes**: Added WebPublishedAt, renamed WebSettings, updated Validate()

**Required Test Updates**:

```go
// internal/domain/broadcast_test.go

TestBroadcast_Validate_SingleListRequired ✅ FROM CLARIFICATIONS
  - Broadcast with 0 lists -> error "exactly one list must be specified"
  - Broadcast with 2+ lists -> error "exactly one list must be specified"
  - Broadcast with 1 list -> success
  - Validates that Broadcast.Validate() enforces single list rule

TestWebPublicationSettings_Value
  - Serialize to JSON for database

TestWebPublicationSettings_Scan
  - Deserialize from JSON

TestWebPublicationSettings_MergeWithDefaults
  - Merge post settings with list defaults
  - Null handling
  - Empty string handling
```

**Estimated**: 4 test cases, ~100 lines

---

### 7. `internal/domain/list.go` ⚠️ UPDATE TESTS

**Status**: Tests exist in `list_test.go`  
**Changes**: Added Slug, WebPublicationEnabled, WebPublicationSettings

**Required Test Updates**:

```go
// internal/domain/list_test.go

TestScanList_WithWebPublicationFields
  - Scan list with slug, web_publication_enabled, web_publication_settings
  - Verify all fields populated

TestScanList_WithoutWebPublicationFields
  - Scan list with NULL web publication fields
  - Should handle gracefully

TestList_Validate_WithWebPublication
  - List with web enabled but no slug -> still valid
  - Service layer handles slug requirement
```

**Estimated**: 3 test cases, ~80 lines

---

### 8. `internal/repository/broadcast_postgres.go` ⚠️ UPDATE TESTS

**Status**: Tests exist in `broadcast_postgres_test.go`  
**Changes**: Added web_published_at column, new list-scoped methods

**Required Test Updates**:

```go
// internal/repository/broadcast_postgres_test.go

TestCreateBroadcast_WithWebPublicationSettings
  - Create with web_publication_settings and web_published_at
  - Verify saved to DB

TestUpdateBroadcast_WebPublishedAt
  - Update web_published_at
  - Verify column updated

TestGetPublishedWebBroadcasts_UsesWebPublishedAt
  - Query uses web_published_at column, not JSONB
  - Returns only published broadcasts

TestGetByListAndSlug_Success
  - Find broadcast by list ID and slug
  - Returns correct broadcast

TestGetByListAndSlug_NotFound
  - Invalid list or slug
  - Returns error

TestGetPublishedWebBroadcastsByList_Success
  - Returns broadcasts for specific list
  - Ordered by web_published_at DESC

TestGetPublishedWebBroadcastsByList_Empty
  - List with no published broadcasts
  - Returns empty array

TestGetPublishedCountsByList_BatchQuery
  - Returns map of list ID -> count
  - Single query (verify no N+1)

TestGetPublishedCountsByList_Empty
  - No published broadcasts
  - Returns empty map
```

**Estimated**: 9 test cases, ~250 lines

---

### 9. `internal/repository/list_postgres.go` ⚠️ UPDATE TESTS

**Status**: Tests may exist in `list_postgres_test.go`  
**Changes**: Added slug columns, new methods

**Required Test Updates**:

```go
// internal/repository/list_postgres_test.go

TestCreateList_WithSlugAndWebPublication
  - Create list with slug and web_publication_enabled
  - Verify saved to DB

TestUpdateList_WebPublicationFields
  - Update slug and web_publication_enabled
  - Verify updated

TestGetBySlug_Success
  - Find list by slug
  - Returns correct list

TestGetBySlug_NotFound
  - Invalid slug
  - Returns error

TestGetPublishedLists_Success
  - Returns only lists with web_publication_enabled=true and slug IS NOT NULL
  - Ordered by name

TestGetPublishedLists_Empty
  - No published lists
  - Returns empty array

TestSlugExistsInWorkspace_Exists
  - Slug already used
  - Returns true

TestSlugExistsInWorkspace_NotExists
  - Slug available
  - Returns false

TestSlugExistsInWorkspace_ExcludeList
  - Same list updating its own slug
  - Returns false (exclude works)

TestUnpublishBroadcastsInList_Success
  - Clears web_published_at for broadcasts in list
  - Verify broadcasts unpublished
```

**Estimated**: 10 test cases, ~280 lines

---

### 10. `internal/service/broadcast_service.go` ✅ TESTS UPDATED

**Status**: Tests updated with listService mock  
**Additional Tests Needed**:

```go
// internal/service/broadcast_service_test.go

TestCreateBroadcast_WebChannel_WorkspaceNotEnabled
  - Web channel requested
  - Workspace web_publications_enabled = false
  - Returns error

TestCreateBroadcast_WebChannel_ListNotEnabled
  - Web channel requested
  - List web_publication_enabled = false
  - Returns error

TestCreateBroadcast_WebChannel_NoListSlug
  - Web channel requested
  - List has no slug
  - Returns error

TestCreateBroadcast_WebChannel_NoPostSlug
  - Web channel requested
  - No post slug provided
  - Returns error

TestCreateBroadcast_WebChannel_InvalidSlug
  - Web channel requested
  - Invalid slug format
  - Returns validation error

TestCreateBroadcast_SingleListRequired ✅ FROM CLARIFICATIONS
  - Zero lists -> error "exactly one list must be specified"
  - Two lists -> error "exactly one list must be specified"
  - One list -> success

TestUpdateBroadcast_WebChannelEnableOnNonDraft
  - Try to enable web on sent broadcast
  - Returns error

TestScheduleBroadcast_SetsWebPublishedAt
  - Web channel enabled
  - SendNow = true
  - Sets web_published_at to now

TestCreateBroadcast_SlugUniquePerList ✅ FROM CLARIFICATIONS
  - List has existing broadcast with slug "my-post"
  - Create new broadcast in same list with slug "my-post"
  - Should fail with slug uniqueness error
  - Same slug in different list should succeed
```

**Estimated**: 9 new test cases, ~230 lines

---

### 11. `internal/service/list_service.go` ⚠️ UPDATE TESTS

**Status**: Tests may exist  
**Changes**: Added deletion cascade

**Required Test Updates**:

```go
// internal/service/list_service_test.go

TestDeleteList_UnpublishesBroadcasts ✅ FROM CLARIFICATIONS
  - List has published broadcasts with web_published_at set
  - Delete list
  - Verify UnpublishBroadcastsInList called
  - Verify web_published_at cleared (broadcasts unpublished)
  - List deleted successfully

TestDeleteList_UnpublishFails
  - UnpublishBroadcastsInList returns error
  - Warning logged
  - List still deleted (best-effort)
```

**Estimated**: 2 test cases, ~60 lines

---

## 🟢 Priority 3: Supporting Tests

### 12. `pkg/notifuse_mjml/template_compilation.go` ⚠️ UPDATE TESTS

**Status**: Tests exist in `template_compilation_test.go`  
**Changes**: Added Channel field

**Required Test Updates**:

```go
// pkg/notifuse_mjml/template_compilation_test.go

TestCompileTemplate_WithEmailChannel
  - channel = "email"
  - Applies email_only sections
  - Filters web_only sections

TestCompileTemplate_WithWebChannel
  - channel = "web"
  - Applies web_only sections
  - Filters email_only sections
  - No tracking pixel
  - No link wrapping

TestCompileTemplate_NoChannel
  - channel empty
  - No filtering applied
  - All sections included
```

**Estimated**: 3 test cases, ~100 lines

---

### 13. `pkg/notifuse_mjml/slug.go` ⚠️ NEEDS TESTS (if file exists)

**Status**: No tests  
**Priority**: Low (simple validation logic)

**Required Tests**:

```go
// pkg/notifuse_mjml/slug_test.go

TestValidateSlug_Valid
  - Valid slugs: "newsletter", "product-updates", "abc123"
  - Returns nil

TestValidateSlug_Invalid
  - Invalid: "Newsletter" (uppercase)
  - Invalid: "news_letter" (underscore)
  - Invalid: "news letter" (space)
  - Invalid: "" (empty)
  - Invalid: string > 100 chars
  - Returns appropriate errors
```

**Estimated**: 2 test cases, ~60 lines

---

## 📊 Test Coverage Summary

| File                        | Status    | Priority | Lines | Est. Effort |
| --------------------------- | --------- | -------- | ----- | ----------- |
| filter.go                   | ⚠️ Needed | High     | 150   | 2 hours     |
| dns_verification_service.go | ⚠️ Needed | Critical | 200   | 3 hours     |
| web_publication_service.go  | ⚠️ Needed | Critical | 400   | 5 hours     |
| web_publication_handler.go  | ⚠️ Needed | High     | 300   | 4 hours     |
| broadcast.go (domain)       | ⚠️ Update | Medium   | 100   | 1 hour      |
| list.go (domain)            | ⚠️ Update | Medium   | 80    | 1 hour      |
| broadcast_postgres.go       | ⚠️ Update | High     | 250   | 3 hours     |
| list_postgres.go            | ⚠️ Update | High     | 280   | 3 hours     |
| broadcast_service.go        | ⚠️ Update | High     | 200   | 2 hours     |
| list_service.go             | ⚠️ Update | Low      | 60    | 1 hour      |
| template_compilation.go     | ⚠️ Update | Medium   | 100   | 1 hour      |
| slug.go (if exists)         | ⚠️ Needed | Low      | 60    | 30 min      |

**Total Estimated Effort**: ~26-28 hours

---

## 🎯 Testing Strategy

### Phase 1: Critical Path (Priority 1)

Focus on files that impact functionality and security:

1. `dns_verification_service_test.go` - Security critical
2. `web_publication_service_test.go` - Core functionality
3. `filter_test.go` - Content filtering
4. `web_publication_handler_test.go` - HTTP layer

**Target**: 90% coverage of new files  
**Timeline**: 1-2 days

### Phase 2: Repository Layer (Priority 2)

Update existing tests for schema changes:

1. `broadcast_postgres_test.go` - New columns and methods
2. `list_postgres_test.go` - New columns and methods

**Target**: Cover new methods, update existing tests  
**Timeline**: 1 day

### Phase 3: Service Layer (Priority 3)

Update service tests for validation changes:

1. `broadcast_service_test.go` - Web channel validation
2. `list_service_test.go` - Deletion cascade
3. `template_compilation_test.go` - Channel filtering

**Target**: Cover new validation logic  
**Timeline**: 1 day

---

## 🔧 Testing Tools & Mocks

### Mocks Already Available:

- ✅ `MockBroadcastRepository` - Regenerated
- ✅ `MockListRepository` - Regenerated
- ✅ `MockBroadcastService`
- ✅ `MockListService`
- ✅ `MockWorkspaceRepository`

### Mocks Needed:

- DNS resolver mock (for dns_verification_service)
- HTTP test fixtures for web_publication_handler

### Test Databases:

- Use existing test database pattern
- Transaction rollback for isolation

---

## 📝 Test File Creation Checklist

### New Test Files to Create:

- [ ] `pkg/notifuse_mjml/filter_test.go`
- [ ] `internal/service/dns_verification_service_test.go`
- [ ] `internal/service/web_publication_service_test.go`
- [ ] `internal/http/web_publication_handler_test.go`

### Existing Test Files to Update:

- [ ] `internal/domain/broadcast_test.go`
- [ ] `internal/domain/list_test.go`
- [ ] `internal/repository/broadcast_postgres_test.go`
- [ ] `internal/repository/list_postgres_test.go`
- [ ] `internal/service/broadcast_service_test.go`
- [ ] `internal/service/list_service_test.go`
- [ ] `pkg/notifuse_mjml/template_compilation_test.go`

---

## 🎓 Test Examples

### Example: DNS Verification Test

```go
func TestVerifyDomainOwnership_ValidCNAME(t *testing.T) {
    logger := logger.NewLogger()
    service := NewDNSVerificationService(logger, "api.notifuse.com")

    // This test requires actual DNS or mocking net.LookupCNAME
    // For real test, setup test domain with CNAME
    // For unit test, mock the DNS lookup

    ctx := context.Background()
    err := service.VerifyDomainOwnership(ctx, "https://blog.test.com")

    // With mock: expect no error
    assert.NoError(t, err)
}
```

### Example: Filter Test

```go
func TestFilterBlocksByChannel_EmailOnly(t *testing.T) {
    // Create tree with email_only section
    tree := createTestTree()
    section := tree.GetChildren()[0]
    section.GetAttributes()["visibility"] = "email_only"

    // Filter for email channel
    emailFiltered := FilterBlocksByChannel(tree, "email")
    assert.Equal(t, 1, len(emailFiltered.GetChildren()))

    // Filter for web channel
    webFiltered := FilterBlocksByChannel(tree, "web")
    assert.Equal(t, 0, len(webFiltered.GetChildren()))
}
```

---

## 🚀 Quick Wins (Start Here)

1. **slug.go validation** - Simple, fast to test
2. **Domain model updates** - Straightforward serialization tests
3. **Filter tests** - Clear input/output

These can be completed quickly and boost coverage.

---

## 🔍 Coverage Goals

| Category       | Current | Target |
| -------------- | ------- | ------ |
| New Files      | 0%      | 80%+   |
| Modified Files | Varies  | 90%+   |
| Overall        | ~60%    | 85%+   |

---

## 💡 Recommendations

### Immediate Actions:

1. Start with `filter_test.go` - Critical for content rendering
2. Add `web_publication_service_test.go` - Core business logic
3. Update `broadcast_service_test.go` - Validation changes

### Can Defer:

- `web_publication_handler_test.go` - HTTP layer (integration test more valuable)
- `slug.go` tests - Trivial validation
- Template compilation updates - Existing tests likely sufficient

### Integration Tests More Valuable Than Unit Tests:

- Full web publication flow
- DNS verification with real domain
- Multi-level SEO rendering
- List deletion cascade

---

## 📅 Suggested Timeline

**Week 1**:

- Day 1-2: Critical new files (filter, web service)
- Day 3: Repository updates
- Day 4: Service updates
- Day 5: Integration tests

**Success Criteria**:

- [ ] All new files have >80% coverage
- [ ] All modified files tested
- [ ] CI/CD passes
- [ ] No regressions in existing tests

---

## ✅ Feature Checklist Testing Coverage

### Currently Tested ✅

1. ✅ **Console accessible at `/console` path**

   - File: `internal/http/root_handler_test.go`
   - Tests: `TestRootHandler_ServeConsole`
   - Status: PASSING

2. ✅ **Root `/` redirects to `/console` when no workspace**

   - File: `internal/http/root_handler_test.go`
   - Test: `TestRootHandler_Handle_Comprehensive/RootRedirectsToConsole`
   - Status: PASSING

3. ✅ **All console navigation updated**
   - Manual verification: All routes use `/console` prefix
   - Status: VERIFIED

### Needs Testing ⚠️

4. ⚠️ **DNS CNAME verification works**

   - File: `internal/service/dns_verification_service.go`
   - **Missing**: `dns_verification_service_test.go`
   - Required tests:
     ```go
     TestVerifyDomainOwnership_ValidCNAME
     TestVerifyDomainOwnership_InvalidCNAME
     TestVerifyDomainOwnership_NoCNAME
     ```

5. ⚠️ **Domain squatting prevented**

   - File: `internal/service/workspace_service.go`
   - **Missing**: Test in `workspace_service_test.go`
   - Required test:
     ```go
     TestUpdateWorkspace_DuplicateDomain
       - Two workspaces try to claim same domain
       - Second one should fail
     ```

6. ⚠️ **Root `/` redirects to `/console` when web disabled**

   - File: `internal/http/root_handler.go`
   - **Missing**: Test in `root_handler_test.go`
   - Required test:
     ```go
     TestRootHandler_RootWithWorkspaceButNoWebPublications
       - Workspace found by host
       - HasWebPublications() returns false
       - Should redirect to /console
     ```

7. ⚠️ **Root `/` shows publications when web enabled**

   - File: `internal/http/root_handler.go` + `web_publication_handler.go`
   - **Missing**: Integration test
   - Required test:
     ```go
     TestRootHandler_RootWithWebPublications
       - Workspace found by host
       - HasWebPublications() returns true
       - Should serve web publication list
     ```

8. ⚠️ **Channel filtering works (email/web)**

   - File: `pkg/notifuse_mjml/filter.go`
   - **Missing**: `filter_test.go`
   - Required tests:
     ```go
     TestFilterBlocksByChannel_EmailOnly
     TestFilterBlocksByChannel_WebOnly
     TestFilterBlocksByChannel_All
     TestFilterBlocksByChannel_Nested
     ```

9. ⚠️ **Post caching works with 5-min TTL**

   - File: `internal/service/web_publication_service.go`
   - **Missing**: `web_publication_service_test.go`
   - Required tests:

     ```go
     TestPostCache_GetSet
       - Cache hit within TTL
       - Cache miss after TTL
       - Cleanup works

     TestGetPostBySlug_CacheHit
     TestGetPostBySlug_CacheMiss
     ```

10. ⚠️ **Slug format (NO NANOID - Updated)**

- **CHANGED**: Slugs are now clean (no nanoid)
- File: `internal/service/broadcast_service.go`
- Test: `ValidateSlug()` - simple validation
- Required test:
  ```go
  TestValidateSlug_CleanFormat
    - Valid: "newsletter", "product-launch"
    - Invalid: "News_Letter", "news letter", ""
  ```

11. ⚠️ **robots.txt and sitemap.xml work**

- File: `internal/http/web_publication_handler.go`
- **Missing**: `web_publication_handler_test.go`
- Required tests:

  ```go
  TestHandleRobots
    - Returns text/plain
    - Contains User-agent
    - Contains Sitemap URL

  TestHandleSitemap
    - Returns application/xml
    - Contains all published lists
    - Contains all published posts
    - Proper XML structure
  ```

12. ⚠️ **SEO meta tags render correctly**

- File: `internal/http/templates/*.html`
- **Missing**: Template rendering tests
- Required tests:

  ```go
  TestHandlePost_SEOMetaTags
    - Meta title rendered
    - Meta description rendered
    - OG tags rendered
    - Keywords rendered

  TestHandleList_SEOMetaTags
    - List page meta tags
    - Workspace defaults if list has none
  ```

---

## 🎯 Updated Priority List

### Critical for Launch:

1. **DNS verification tests** - Security critical
2. **Channel filtering tests** - Content correctness
3. **Caching tests** - Performance validation
4. **Root handler routing tests** - User experience

### Important for Quality:

5. **Repository method tests** - Data integrity
6. **SEO rendering tests** - Marketing effectiveness
7. **Sitemap/robots tests** - SEO crawlability

### Nice to Have:

8. **Slug validation tests** - Simple, low risk
9. **Domain model serialization** - Framework handles most

---

## 📝 Test Implementation Order

1. Add missing root_handler tests (routing scenarios)
2. Create filter_test.go (content filtering)
3. Create dns_verification_service_test.go (security)
4. Create web_publication_service_test.go (caching + core logic)
5. Create web_publication_handler_test.go (SEO + sitemap)
6. Update repository tests (new methods)
7. Update service tests (validation changes)

**Estimated Time**: 3-4 days for complete coverage

---

---

## 🧪 End-to-End Test ✅ FROM CLARIFICATIONS

### E2E Test: Complete Web Publication Flow

**File**: `tests/e2e/web_publication_test.go` (create new)

```go
TestWebPublicationE2E_CompleteFlow
  1. Create workspace
  2. Enable workspace.settings.web_publications_enabled = true
  3. Configure custom domain with DNS verification
  4. Create list with slug="newsletter"
  5. Enable list.web_publication_enabled = true
  6. Create broadcast targeting that list
  7. Enable broadcast.channels.web = true
  8. Set broadcast.web_publication_settings.slug = "my-post"
  9. Send broadcast (sets web_published_at)
  10. Verify broadcast visible at /{list-slug}/{post-slug}
  11. Verify SEO meta tags rendered
  12. Verify list menu shows
  13. Verify sitemap includes post
  14. Update list slug
  15. Verify URLs still work (cache cleared)
  16. Delete list
  17. Verify web_published_at cleared
  18. Verify post no longer visible on web
```

**Estimated**: 1 comprehensive E2E test, ~300 lines

---

## ✅ Mocks Status FROM CLARIFICATIONS

- [x] **Regenerate mocks** - COMPLETED
  - `MockBroadcastRepository` - Regenerated with new methods
  - `MockListRepository` - Regenerated with new methods
  - All new interface methods included

---

**Status**: Plan updated with ALL items from CLARIFICATIONS  
**Next Step**: Implement tests in priority order

### From CLARIFICATIONS - Mapping to Tests:

1. ✅ Update Broadcast.Validate() tests → `TestBroadcast_Validate_SingleListRequired`
2. ✅ Test single list enforcement → `TestCreateBroadcast_SingleListRequired`
3. ✅ Test slug uniqueness per list → `TestCreateBroadcast_SlugUniquePerList`
4. ✅ Test list deletion clears PublishedAt → `TestDeleteList_UnpublishesBroadcasts`
5. ✅ E2E test → `TestWebPublicationE2E_CompleteFlow`
6. ✅ Regenerate mocks → COMPLETED
