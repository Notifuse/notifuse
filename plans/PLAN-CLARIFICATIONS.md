# Web Publications List Structure - Key Decisions

**Version**: 17.0 (Rewrite)  
**Date**: November 7, 2025

---

## ✅ All Critical Decisions Made

### 1. Data Structure - **No Breaking Changes**
- `AudienceSettings.Lists` stays as `[]string` (array)
- Just enforce `len(lists) == 1` in validation
- Broadcast.Validate() updated to require exactly one list
- **No data migration needed**

### 2. Slug State Management - **Always Preserve**
- Slug stays in DB even when `web_publication_enabled = false`
- Slug is "reserved" for that list
- Prevents slug conflicts
- User doesn't lose slug if they toggle off/on

### 3. List Deletion - **Clear PublishedAt**
```go
func DeleteList(ctx, listID) error {
    // Get all web broadcasts in this list
    broadcasts := GetWebBroadcastsInList(listID)
    
    // Clear PublishedAt to unpublish from web
    for _, b := range broadcasts {
        b.WebPublicationSettings.PublishedAt = nil
        Update(b)
    }
    
    // Now delete list
    Delete(listID)
}
```
Posts become invisible on web but data preserved.

### 4. Cache Strategy - **Clear All**
- List update → `ClearAllCache()`
- Workspace settings update → `ClearAllCache()`
- Broadcast update → `InvalidateCache(broadcastID)`
- Simple, safe, no stale data

### 5. Performance - **Single Batch Query**
```go
// One query for all list post counts
func GetPublishedCountsByList(workspaceID string) map[string]int {
    // Returns map[listID]count
    // No N+1 problem
}
```

### 6. SEO Hierarchy - **Simple Per-Page**
- `/` uses `workspace.web_publication_settings`
- `/{list}` uses `list.web_publication_settings`
- `/{list}/{post}` uses `broadcast.web_publication_settings`
- No complex fallback cascade
- Each page independent

### 7. Slug Entry - **Preset from Name**
```tsx
// UI presets slug input when list name entered
useEffect(() => {
  if (listName && !listSlug) {
    form.setFieldValue('slug', generateSlugFromName(listName))
  }
}, [listName])

// Below input, show URL preview:
// "notifuse.website.com/newsletter"
```
User can modify preset value before saving.

### 8-18. Future Enhancements
- RSS Feed - Future
- Social Share Buttons - Future
- Post Ordering - By published_at (implemented)
- List Menu - Shows lists not posts ✅
- Breadcrumbs - Future
- Search - Future  
- Type Naming - WebPublicationSettings everywhere ✅
- Pagination Info - Future
- Post Excerpts - Future
- Featured Image - Using OG image
- E2E Tests - Included in plan ✅

---

## 🎯 Reusable WebPublicationSettings

**Key Decision**: Reuse existing `WebSettings` struct, rename to `WebPublicationSettings`

```go
// Already exists in broadcast.go - just reuse it!
type WebPublicationSettings struct {
    Slug            string     `json:"slug,omitempty"`
    MetaTitle       string     `json:"meta_title,omitempty"`
    MetaDescription string     `json:"meta_description,omitempty"`
    OGTitle         string     `json:"og_title,omitempty"`
    OGDescription   string     `json:"og_description,omitempty"`
    OGImage         string     `json:"og_image,omitempty"`
    CanonicalURL    string     `json:"canonical_url,omitempty"`
    Keywords        []string   `json:"keywords,omitempty"`
    PublishedAt     *time.Time `json:"published_at,omitempty"`
}
```

**Usage**:
- Workspace: `web_publication_settings` (no slug/publishedAt)
- List: `web_publication_settings` (with slug, no publishedAt)
- Broadcast: `web_publication_settings` (with slug + publishedAt)

**Frontend Component**:
```tsx
<SEOSettingsForm 
  namePrefix={['web_publication_settings']}
  showCanonical={forBroadcast}
/>
```

---

## 📋 Updated Implementation Checklist

### Backend
- [ ] Rename `WebSettings` → `WebPublicationSettings` everywhere
- [ ] Add `WebPublicationSettings` to List model
- [ ] Add `WebPublicationSettings` to WorkspaceSettings
- [ ] Add `MergeWithDefaults()` method
- [ ] Update Broadcast.Validate() - require exactly 1 list
- [ ] Remove `generateNanoID()` function
- [ ] Add batch query `GetPublishedCountsByList()`
- [ ] Add `DeleteList()` cascade - clear PublishedAt
- [ ] Add `ClearAllCache()` method
- [ ] Update migration v17 with lists table changes

### Frontend
- [ ] Create `console/src/types/seo.ts`
- [ ] Create `console/src/components/seo/SEOSettingsForm.tsx`
- [ ] Change list selector from tag mode → single select
- [ ] Add slug field to list form (conditional on web toggle)
- [ ] Preset slug from list name
- [ ] Show URL preview below slug input
- [ ] Add workspace web pub toggle + SEO form
- [ ] Use `SEOSettingsForm` in broadcast form
- [ ] Update all types to use `WebPublicationSettings`

### Tests
- [ ] Update Broadcast.Validate() tests
- [ ] Test single list enforcement
- [ ] Test slug uniqueness per list
- [ ] Test list deletion clears PublishedAt
- [ ] E2E test: workspace toggle → list setup → broadcast → web view
- [ ] Regenerate mocks

---

## 🚀 Implementation Ready

**Plan Status**: ✅ Complete  
**Critical Issues**: ✅ All Resolved  
**Performance**: ✅ Optimized  
**UX**: ✅ Good  
**Breaking Changes**: ✅ Minimal  

**Ready to implement!**

