package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	http_handler "github.com/Notifuse/notifuse/internal/http"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

type blogThemeEndpoint struct {
	name   string
	method string
	path   string
	body   string // a valid body for the endpoints that read one
	call   func(h *http_handler.BlogThemeHandler, w http.ResponseWriter, r *http.Request)
	// refuse makes the service answer err for this endpoint.
	refuse func(service *mocks.MockBlogService, err error)
}

var blogThemeEndpoints = []blogThemeEndpoint{
	{
		name: "create", method: http.MethodPost, path: "/api/blogThemes.create", body: `{"files": {}}`,
		call: (*http_handler.BlogThemeHandler).HandleCreate,
		refuse: func(s *mocks.MockBlogService, err error) {
			s.EXPECT().CreateTheme(gomock.Any(), gomock.Any()).Return(nil, err)
		},
	},
	{
		name: "get", method: http.MethodGet, path: "/api/blogThemes.get?version=1",
		call: (*http_handler.BlogThemeHandler).HandleGet,
		refuse: func(s *mocks.MockBlogService, err error) {
			s.EXPECT().GetTheme(gomock.Any(), 1).Return(nil, err)
		},
	},
	{
		name: "getPublished", method: http.MethodGet, path: "/api/blogThemes.getPublished",
		call: (*http_handler.BlogThemeHandler).HandleGetPublished,
		refuse: func(s *mocks.MockBlogService, err error) {
			s.EXPECT().GetPublishedTheme(gomock.Any()).Return(nil, err)
		},
	},
	{
		name: "update", method: http.MethodPost, path: "/api/blogThemes.update", body: `{"version": 1, "files": {}}`,
		call: (*http_handler.BlogThemeHandler).HandleUpdate,
		refuse: func(s *mocks.MockBlogService, err error) {
			s.EXPECT().UpdateTheme(gomock.Any(), gomock.Any()).Return(nil, err)
		},
	},
	{
		name: "publish", method: http.MethodPost, path: "/api/blogThemes.publish", body: `{"version": 1}`,
		call: (*http_handler.BlogThemeHandler).HandlePublish,
		refuse: func(s *mocks.MockBlogService, err error) {
			s.EXPECT().PublishTheme(gomock.Any(), gomock.Any()).Return(err)
		},
	},
	{
		name: "list", method: http.MethodGet, path: "/api/blogThemes.list",
		call: (*http_handler.BlogThemeHandler).HandleList,
		refuse: func(s *mocks.MockBlogService, err error) {
			s.EXPECT().ListThemes(gomock.Any(), gomock.Any()).Return(nil, err)
		},
	},
}

// newBlogThemeHandlerForRefusals builds a handler whose service has no expectation
// unless the test sets one: a refused request that reached it would fail the test.
func newBlogThemeHandlerForRefusals(t *testing.T) (*http_handler.BlogThemeHandler, *mocks.MockBlogService) {
	ctrl := gomock.NewController(t)
	service := mocks.NewMockBlogService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	return http_handler.NewBlogThemeHandler(service, func() ([]byte, error) { return []byte("secret"), nil }, log), service
}

// withWorkspace adds workspace_id to an endpoint path.
func withWorkspace(path string) string {
	if strings.Contains(path, "?") {
		return path + "&workspace_id=ws-1"
	}
	return path + "?workspace_id=ws-1"
}

func TestBlogThemeHandler_RefusedBeforeTheService(t *testing.T) {
	for _, e := range blogThemeEndpoints {
		t.Run(e.name+": wrong method", func(t *testing.T) {
			handler, _ := newBlogThemeHandlerForRefusals(t)
			wrong := http.MethodPost
			if e.method == http.MethodPost {
				wrong = http.MethodGet
			}
			rec := httptest.NewRecorder()
			e.call(handler, rec, httptest.NewRequest(wrong, withWorkspace(e.path), strings.NewReader(e.body)))

			assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		})

		t.Run(e.name+": no workspace_id", func(t *testing.T) {
			handler, _ := newBlogThemeHandlerForRefusals(t)
			rec := httptest.NewRecorder()
			e.call(handler, rec, httptest.NewRequest(e.method, e.path, strings.NewReader(e.body)))

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "workspace_id is required")
		})

		if e.body != "" {
			t.Run(e.name+": body that is not JSON", func(t *testing.T) {
				handler, _ := newBlogThemeHandlerForRefusals(t)
				rec := httptest.NewRecorder()
				e.call(handler, rec, httptest.NewRequest(e.method, withWorkspace(e.path), strings.NewReader("{not json")))

				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "Invalid request body")
			})
		}
	}

	for _, param := range []string{"limit", "offset"} {
		t.Run("list: "+param+" that is not a number", func(t *testing.T) {
			handler, _ := newBlogThemeHandlerForRefusals(t)
			rec := httptest.NewRecorder()
			handler.HandleList(rec, httptest.NewRequest(http.MethodGet, "/api/blogThemes.list?workspace_id=ws-1&"+param+"=ten", nil))

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), param+" must be a valid integer")
		})
	}
}

// A member without blog permission gets a 403, whatever status the endpoint gives
// its other failures.
func TestBlogThemeHandler_PermissionErrorIsForbidden(t *testing.T) {
	refusal := domain.NewPermissionError(domain.PermissionResourceBlog, domain.PermissionTypeWrite, "no blog access")
	for _, e := range blogThemeEndpoints {
		t.Run(e.name, func(t *testing.T) {
			handler, service := newBlogThemeHandlerForRefusals(t)
			e.refuse(service, refusal)

			rec := httptest.NewRecorder()
			e.call(handler, rec, httptest.NewRequest(e.method, withWorkspace(e.path), strings.NewReader(e.body)))

			assert.Equal(t, http.StatusForbidden, rec.Code)
		})
	}
}
