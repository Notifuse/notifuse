package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

// PostCache stores rendered HTML for published posts with TTL
type PostCache struct {
	sync.RWMutex
	posts map[string]*CachedPost
}

// CachedPost represents a cached rendered post
type CachedPost struct {
	HTML      string
	ExpiresAt time.Time
}

// NewPostCache creates a new post cache
func NewPostCache() *PostCache {
	cache := &PostCache{
		posts: make(map[string]*CachedPost),
	}
	
	// Start cleanup goroutine
	go cache.cleanupExpired()
	
	return cache
}

// Get retrieves a cached post if it exists and hasn't expired
func (c *PostCache) Get(key string) (string, bool) {
	c.RLock()
	defer c.RUnlock()
	
	post, exists := c.posts[key]
	if !exists {
		return "", false
	}
	
	// Check if expired
	if time.Now().After(post.ExpiresAt) {
		return "", false
	}
	
	return post.HTML, true
}

// Set stores a rendered post in the cache with 5-minute TTL
func (c *PostCache) Set(key, html string) {
	c.Lock()
	defer c.Unlock()
	
	c.posts[key] = &CachedPost{
		HTML:      html,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
}

// Invalidate removes a post from the cache
func (c *PostCache) Invalidate(key string) {
	c.Lock()
	defer c.Unlock()
	
	delete(c.posts, key)
}

// cleanupExpired periodically removes expired entries
func (c *PostCache) cleanupExpired() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		c.Lock()
		now := time.Now()
		for key, post := range c.posts {
			if now.After(post.ExpiresAt) {
				delete(c.posts, key)
			}
		}
		c.Unlock()
	}
}

// WebPublicationService handles web publication operations
type WebPublicationService struct {
	broadcastRepo domain.BroadcastRepository
	templateRepo  domain.TemplateRepository
	workspaceRepo domain.WorkspaceRepository
	listRepo      domain.ListRepository
	logger        logger.Logger
	cache         *PostCache
}

// NewWebPublicationService creates a new web publication service
func NewWebPublicationService(
	broadcastRepo domain.BroadcastRepository,
	templateRepo domain.TemplateRepository,
	workspaceRepo domain.WorkspaceRepository,
	logger logger.Logger,
) *WebPublicationService {
	return &WebPublicationService{
		broadcastRepo: broadcastRepo,
		templateRepo:  templateRepo,
		workspaceRepo: workspaceRepo,
		logger:        logger,
		cache:         NewPostCache(),
	}
}

// PublishedPost represents a post for web display
type PublishedPost struct {
	ID              string
	Slug            string
	Name            string
	Content         string // Rendered HTML
	PublishedAt     time.Time
	ListID          string
	ListSlug        string
	ListName        string
	MetaTitle       string
	MetaDescription string
	OGTitle         string
	OGDescription   string
	OGImage         string
	CanonicalURL    string
	Keywords        []string
}

// PublishedList represents a list for web navigation menu
type PublishedList struct {
	ID        string
	Name      string
	Slug      string
	PostCount int
}

// PostListResponse contains paginated list of published posts
type PostListResponse struct {
	Posts       []*PublishedPost
	Lists       []*PublishedList // For menu navigation
	TotalCount  int
	Page        int
	PageSize    int
	TotalPages  int
	HasPrevious bool
	HasNext     bool
}

// GetPublishedPosts retrieves a paginated list of published web broadcasts
func (s *WebPublicationService) GetPublishedPosts(ctx context.Context, workspaceID string, page, pageSize int) (*PostListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	
	offset := (page - 1) * pageSize
	
	broadcasts, totalCount, err := s.broadcastRepo.GetPublishedWebBroadcasts(ctx, workspaceID, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get published broadcasts: %w", err)
	}
	
	posts := make([]*PublishedPost, 0, len(broadcasts))
	for _, broadcast := range broadcasts {
		if broadcast.WebPublicationSettings == nil || broadcast.WebPublishedAt == nil {
			continue
		}
		
		post := &PublishedPost{
			ID:              broadcast.ID,
			Slug:            broadcast.WebPublicationSettings.Slug,
			Name:            broadcast.Name,
			PublishedAt:     *broadcast.WebPublishedAt,
			MetaTitle:       broadcast.WebPublicationSettings.MetaTitle,
			MetaDescription: broadcast.WebPublicationSettings.MetaDescription,
			OGTitle:         broadcast.WebPublicationSettings.OGTitle,
			OGDescription:   broadcast.WebPublicationSettings.OGDescription,
			OGImage:         broadcast.WebPublicationSettings.OGImage,
			CanonicalURL:    broadcast.WebPublicationSettings.CanonicalURL,
			Keywords:        broadcast.WebPublicationSettings.Keywords,
		}
		
		posts = append(posts, post)
	}
	
	totalPages := (totalCount + pageSize - 1) / pageSize
	
	// Get list menu
	publishedLists, _ := s.GetPublishedLists(ctx, workspaceID)

	return &PostListResponse{
		Posts:       posts,
		Lists:       publishedLists,
		TotalCount:  totalCount,
		Page:        page,
		PageSize:    pageSize,
		TotalPages:  totalPages,
		HasPrevious: page > 1,
		HasNext:     page < totalPages,
	}, nil
}

// GetPublishedPostsByList retrieves published posts for a specific list
func (s *WebPublicationService) GetPublishedPostsByList(ctx context.Context, workspaceID, listSlug string, page, pageSize int) (*PostListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Get list by slug
	list, err := s.listRepo.GetBySlug(ctx, workspaceID, listSlug)
	if err != nil {
		return nil, fmt.Errorf("list not found: %w", err)
	}

	offset := (page - 1) * pageSize

	broadcasts, totalCount, err := s.broadcastRepo.GetPublishedWebBroadcastsByList(ctx, workspaceID, list.ID, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get published broadcasts: %w", err)
	}

	posts := make([]*PublishedPost, 0, len(broadcasts))
	for _, broadcast := range broadcasts {
		if broadcast.WebPublicationSettings == nil || broadcast.WebPublishedAt == nil {
			continue
		}

		post := &PublishedPost{
			ID:              broadcast.ID,
			Slug:            broadcast.WebPublicationSettings.Slug,
			Name:            broadcast.Name,
			PublishedAt:     *broadcast.WebPublishedAt,
			ListID:          list.ID,
			ListSlug:        *list.Slug,
			ListName:        list.Name,
			MetaTitle:       broadcast.WebPublicationSettings.MetaTitle,
			MetaDescription: broadcast.WebPublicationSettings.MetaDescription,
			OGTitle:         broadcast.WebPublicationSettings.OGTitle,
			OGDescription:   broadcast.WebPublicationSettings.OGDescription,
			OGImage:         broadcast.WebPublicationSettings.OGImage,
			CanonicalURL:    broadcast.WebPublicationSettings.CanonicalURL,
			Keywords:        broadcast.WebPublicationSettings.Keywords,
		}

		posts = append(posts, post)
	}

	totalPages := (totalCount + pageSize - 1) / pageSize

	// Get list menu
	publishedLists, _ := s.GetPublishedLists(ctx, workspaceID)

	return &PostListResponse{
		Posts:       posts,
		Lists:       publishedLists,
		TotalCount:  totalCount,
		Page:        page,
		PageSize:    pageSize,
		TotalPages:  totalPages,
		HasPrevious: page > 1,
		HasNext:     page < totalPages,
	}, nil
}

// GetPostBySlug retrieves a single published post by slug
func (s *WebPublicationService) GetPostBySlug(ctx context.Context, workspaceID, slug string) (*PublishedPost, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("%s:%s", workspaceID, slug)
	if cachedHTML, found := s.cache.Get(cacheKey); found {
		s.logger.WithFields(map[string]interface{}{
			"workspace_id": workspaceID,
			"slug":         slug,
		}).Debug("Returning cached post")
		
		// Get broadcast to populate metadata
		broadcast, err := s.broadcastRepo.GetBySlug(ctx, workspaceID, slug)
		if err != nil {
			return nil, fmt.Errorf("failed to get broadcast: %w", err)
		}
		
		if broadcast.WebPublicationSettings == nil {
			return nil, fmt.Errorf("broadcast has no web publication settings")
		}
		
		if broadcast.WebPublishedAt == nil {
			return nil, fmt.Errorf("broadcast not published")
		}
		
		return &PublishedPost{
			ID:              broadcast.ID,
			Slug:            broadcast.WebPublicationSettings.Slug,
			Name:            broadcast.Name,
			Content:         cachedHTML,
			PublishedAt:     *broadcast.WebPublishedAt,
			MetaTitle:       broadcast.WebPublicationSettings.MetaTitle,
			MetaDescription: broadcast.WebPublicationSettings.MetaDescription,
			OGTitle:         broadcast.WebPublicationSettings.OGTitle,
			OGDescription:   broadcast.WebPublicationSettings.OGDescription,
			OGImage:         broadcast.WebPublicationSettings.OGImage,
			CanonicalURL:    broadcast.WebPublicationSettings.CanonicalURL,
			Keywords:        broadcast.WebPublicationSettings.Keywords,
		}, nil
	}
	
	// Get broadcast from database
	broadcast, err := s.broadcastRepo.GetBySlug(ctx, workspaceID, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get broadcast: %w", err)
	}
	
	// Verify web publication settings exist
	if broadcast.WebPublicationSettings == nil {
		return nil, fmt.Errorf("broadcast has no web publication settings")
	}
	
	// Verify web channel is enabled
	if !broadcast.Channels.Web {
		return nil, fmt.Errorf("web channel not enabled for this broadcast")
	}
	
	// Verify published
	if broadcast.WebPublishedAt == nil {
		return nil, fmt.Errorf("broadcast not published")
	}
	
	if broadcast.WebPublishedAt.After(time.Now()) {
		return nil, fmt.Errorf("broadcast not yet published")
	}
	
	// Render content
	html, err := s.RenderWebContent(ctx, broadcast)
	if err != nil {
		return nil, fmt.Errorf("failed to render content: %w", err)
	}
	
	// Cache the rendered HTML
	s.cache.Set(cacheKey, html)
	
	return &PublishedPost{
		ID:              broadcast.ID,
		Slug:            broadcast.WebPublicationSettings.Slug,
		Name:            broadcast.Name,
		Content:         html,
		PublishedAt:     *broadcast.WebPublishedAt,
		MetaTitle:       broadcast.WebPublicationSettings.MetaTitle,
		MetaDescription: broadcast.WebPublicationSettings.MetaDescription,
		OGTitle:         broadcast.WebPublicationSettings.OGTitle,
		OGDescription:   broadcast.WebPublicationSettings.OGDescription,
		OGImage:         broadcast.WebPublicationSettings.OGImage,
		CanonicalURL:    broadcast.WebPublicationSettings.CanonicalURL,
		Keywords:        broadcast.WebPublicationSettings.Keywords,
	}, nil
}

// RenderWebContent renders the broadcast content for web display
func (s *WebPublicationService) RenderWebContent(ctx context.Context, broadcast *domain.Broadcast) (string, error) {
	// Get the template
	var templateID string
	if broadcast.TestSettings.Enabled && len(broadcast.TestSettings.Variations) > 0 {
		// Use first variation for web (or winning template if A/B test completed)
		if broadcast.WinningTemplate != "" {
			templateID = broadcast.WinningTemplate
		} else {
			templateID = broadcast.TestSettings.Variations[0].TemplateID
		}
	} else {
		return "", fmt.Errorf("broadcast has no template")
	}
	
	// Get latest version of template
	version, err := s.templateRepo.GetTemplateLatestVersion(ctx, broadcast.WorkspaceID, templateID)
	if err != nil {
		return "", fmt.Errorf("failed to get template version: %w", err)
	}
	
	template, err := s.templateRepo.GetTemplateByID(ctx, broadcast.WorkspaceID, templateID, version)
	if err != nil {
		return "", fmt.Errorf("failed to get template: %w", err)
	}
	
	if template.Email == nil {
		return "", fmt.Errorf("template has no email configuration")
	}
	
	// Compile template with web channel filtering
	req := notifuse_mjml.CompileTemplateRequest{
		WorkspaceID:      broadcast.WorkspaceID,
		MessageID:        broadcast.ID,
		VisualEditorTree: template.Email.VisualEditorTree,
		Channel:          "web", // Filter for web channel
		// No template data for web (no contact personalization)
		// No tracking settings for web
	}
	
	resp, err := notifuse_mjml.CompileTemplate(req)
	if err != nil {
		return "", fmt.Errorf("failed to compile template: %w", err)
	}
	
	if !resp.Success {
		return "", fmt.Errorf("template compilation failed: %s", resp.Error.Message)
	}
	
	return *resp.HTML, nil
}

// InvalidateCache invalidates the cache for a specific broadcast
func (s *WebPublicationService) InvalidateCache(workspaceID, broadcastID string) {
	// We need to invalidate by slug, but we don't have it here
	// So we'll need to get the broadcast first
	broadcast, err := s.broadcastRepo.GetBroadcast(context.Background(), workspaceID, broadcastID)
	if err != nil {
		s.logger.WithField("error", err.Error()).Error("Failed to get broadcast for cache invalidation")
		return
	}
	
	if broadcast.WebPublicationSettings != nil && broadcast.WebPublicationSettings.Slug != "" {
		cacheKey := fmt.Sprintf("%s:%s", workspaceID, broadcast.WebPublicationSettings.Slug)
		s.cache.Invalidate(cacheKey)
		s.logger.WithFields(map[string]interface{}{
			"workspace_id": workspaceID,
			"broadcast_id": broadcastID,
			"slug":         broadcast.WebPublicationSettings.Slug,
		}).Debug("Cache invalidated")
	}
}

// IsWebPublicationEnabled checks if a workspace has web publications enabled
func (s *WebPublicationService) IsWebPublicationEnabled(ctx context.Context, workspaceID string) (bool, error) {
	return s.broadcastRepo.HasWebPublications(ctx, workspaceID)
}

// GetPublishedLists retrieves all lists with web publications enabled and their post counts
func (s *WebPublicationService) GetPublishedLists(ctx context.Context, workspaceID string) ([]*PublishedList, error) {
	// Get lists with web publications enabled
	lists, err := s.listRepo.GetPublishedLists(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get published lists: %w", err)
	}

	// Get post counts for all lists in one query (batch query - no N+1)
	counts, err := s.broadcastRepo.GetPublishedCountsByList(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to get post counts")
		counts = make(map[string]int) // Continue with zero counts
	}

	publishedLists := make([]*PublishedList, 0, len(lists))
	for _, list := range lists {
		if list.Slug == nil {
			continue // Skip lists without slugs
		}

		publishedLists = append(publishedLists, &PublishedList{
			ID:        list.ID,
			Name:      list.Name,
			Slug:      *list.Slug,
			PostCount: counts[list.ID], // O(1) lookup from batch query
		})
	}

	return publishedLists, nil
}
