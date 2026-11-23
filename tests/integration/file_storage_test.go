package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// fileStorageEndpoint is the selfhost_s3 service of tests/compose.test.yaml and of the CI
// workflow: an S3-compatible server reachable both from the API under test, which lists
// and deletes, and from the test itself, which plays the browser sending presigned PUTs.
func fileStorageEndpoint() string {
	if endpoint := os.Getenv("FILE_STORAGE_TEST_ENDPOINT"); endpoint != "" {
		return endpoint
	}
	return "http://localhost:9100"
}

const (
	fileStorageBucket    = "test-bucket"
	fileStorageAccessKey = "testkey"
	fileStorageSecret    = "testsecret"
)

func fsReadBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(data)
}

func fsDecodeBody(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	body := fsReadBody(t, resp)
	require.NoError(t, json.Unmarshal([]byte(body), v), body)
}

// putPresigned sends a body to a presigned URL as a browser does: a plain PUT with the
// headers the API returned, and no credentials.
func fsPutPresigned(t *testing.T, url string, headers map[string]string, body string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
	require.NoError(t, err)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = fsReadBody(t, resp)
	return resp.StatusCode
}

func TestFileStorage(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	health, err := http.Get(fileStorageEndpoint() + "/health")
	require.NoError(t, err, "selfhost_s3 is not running: docker compose -f tests/compose.test.yaml up -d")
	_ = fsReadBody(t, health)

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	factory := suite.DataFactory
	client := suite.APIClient

	workspace, err := factory.CreateWorkspace(testutil.WithWorkspaceDefaultLanguage("en", []string{"en"}))
	require.NoError(t, err)
	workspaceRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()
	require.NoError(t, workspaceRepo.AddUserToWorkspace(context.Background(), &domain.UserWorkspace{
		UserID:      "550e8400-e29b-41d4-a716-446655440000", // testuser@example.com
		WorkspaceID: workspace.ID,
		Role:        "owner",
		Permissions: domain.FullPermissions,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}))
	asOwner := func() { require.NoError(t, client.Login("testuser@example.com", "")) }
	asMember := func(perms domain.UserPermissions) {
		member, err := factory.CreateUser()
		require.NoError(t, err)
		require.NoError(t, factory.AddUserToWorkspaceWithPermissions(member.ID, workspace.ID, "member", perms))
		require.NoError(t, client.Login(member.Email, "password"))
	}

	fileManager := func(secret string) map[string]interface{} {
		settings := map[string]interface{}{
			"provider":         "other",
			"endpoint":         fileStorageEndpoint(),
			"bucket":           fileStorageBucket,
			"access_key":       fileStorageAccessKey,
			"force_path_style": true,
		}
		if secret != "" {
			settings["secret_key"] = secret
		}
		return settings
	}
	// As the console does: read the workspace, replace the file manager block, send the
	// settings back whole.
	updateFileManager := func(fileManager map[string]interface{}) *http.Response {
		resp, err := client.GetWorkspace(workspace.ID)
		require.NoError(t, err)
		var current struct {
			Workspace struct {
				Settings map[string]interface{} `json:"settings"`
			} `json:"workspace"`
		}
		fsDecodeBody(t, resp, &current)
		current.Workspace.Settings["file_manager"] = fileManager
		resp, err = client.Post("/api/workspaces.update", map[string]interface{}{
			"id":       workspace.ID,
			"name":     workspace.Name,
			"settings": current.Workspace.Settings,
		})
		require.NoError(t, err)
		return resp
	}

	asOwner()
	resp := updateFileManager(fileManager(fileStorageSecret))
	body := fsReadBody(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, body)
	assert.NotContains(t, body, fileStorageSecret, "the update's own response carries no secret")

	presign := func(key, contentType string) (int, domain.PresignedUpload) {
		t.Helper()
		resp, err := client.Post("/api/files.presignUpload", map[string]interface{}{
			"workspace_id": workspace.ID, "key": key, "content_type": contentType,
		})
		require.NoError(t, err)
		var upload domain.PresignedUpload
		if resp.StatusCode != http.StatusOK {
			_ = fsReadBody(t, resp)
			return resp.StatusCode, upload
		}
		fsDecodeBody(t, resp, &upload)
		return resp.StatusCode, upload
	}
	list := func(prefix string) (int, domain.StoredObjectList) {
		t.Helper()
		resp, err := client.Get("/api/files.list", map[string]string{"workspace_id": workspace.ID, "prefix": prefix})
		require.NoError(t, err)
		var result domain.StoredObjectList
		if resp.StatusCode != http.StatusOK {
			_ = fsReadBody(t, resp)
			return resp.StatusCode, result
		}
		fsDecodeBody(t, resp, &result)
		return resp.StatusCode, result
	}
	// The bucket outlives a run: every key lives under this run's own workspace ID, so
	// what a failed run left behind cannot meet the next one.
	k := func(key string) string { return workspace.ID + "/" + key }
	keys := func(result domain.StoredObjectList) []string {
		out := []string{}
		for _, object := range result.Objects {
			out = append(out, object.Key)
		}
		return out
	}
	// selfhost_s3 lists a directory as its folder marker; files leaves those out.
	files := func(result domain.StoredObjectList) []string {
		out := []string{}
		for _, key := range keys(result) {
			if !strings.HasSuffix(key, "/") {
				out = append(out, key)
			}
		}
		return out
	}
	post := func(path string, payload map[string]interface{}) *http.Response {
		t.Helper()
		payload["workspace_id"] = workspace.ID
		resp, err := client.Post(path, payload)
		require.NoError(t, err)
		return resp
	}

	// selfhost_s3 keeps a folder once its files are gone: remove this run's folder at the
	// end, while the server is still up (this defer runs before the suite's).
	defer func() {
		asOwner()
		_ = fsReadBody(t, post("/api/files.deleteFolder", map[string]interface{}{"prefix": k("")}))
	}()

	t.Run("a member without rights receives no secret and no file access", func(t *testing.T) {
		asMember(domain.UserPermissions{})
		defer asOwner()

		for _, path := range []string{"/api/user.me", "/api/workspaces.list"} {
			resp, err := client.Get(path)
			require.NoError(t, err)
			body := fsReadBody(t, resp)
			require.Equal(t, http.StatusOK, resp.StatusCode, body)
			assert.Contains(t, body, workspace.ID, path)
			assert.NotContains(t, body, fileStorageSecret, path)
			assert.NotContains(t, body, "encrypted_secret_key", path)
		}

		status, _ := list("")
		assert.Equal(t, http.StatusForbidden, status)
		status, _ = presign("x.png", "image/png")
		assert.Equal(t, http.StatusForbidden, status)
	})

	t.Run("the owner sees that a secret is set, not the secret", func(t *testing.T) {
		resp, err := client.Get("/api/user.me")
		require.NoError(t, err)
		body := fsReadBody(t, resp)
		assert.NotContains(t, body, fileStorageSecret)
		assert.Contains(t, body, `"has_secret_key":true`)
		assert.Contains(t, body, `"secret_key_hint":"cret"`)
	})

	t.Run("a templates writer uploads through a presigned URL, lists and deletes", func(t *testing.T) {
		asMember(domain.UserPermissions{domain.PermissionResourceTemplates: {Read: true, Write: true}})
		defer asOwner()

		for _, key := range []string{k("upload/logo@2x.png"), k("upload/capture-(1).png"), k("upload/a+b.png")} {
			status, upload := presign(key, "image/png")
			require.Equal(t, http.StatusOK, status, key)
			assert.NotContains(t, upload.URL, fileStorageSecret)
			assert.Equal(t, http.StatusOK, fsPutPresigned(t, upload.URL, upload.Headers, "png bytes"), key)
		}

		status, result := list(k("upload/"))
		require.Equal(t, http.StatusOK, status)
		assert.ElementsMatch(t, []string{k("upload/logo@2x.png"), k("upload/capture-(1).png"), k("upload/a+b.png")}, files(result))

		resp := post("/api/files.delete", map[string]interface{}{"keys": files(result)})
		var deleted domain.DeleteFilesResult
		fsDecodeBody(t, resp, &deleted)
		assert.Equal(t, domain.DeleteFilesResult{Deleted: 3}, deleted)
		_, result = list(k("upload/"))
		assert.Empty(t, files(result))
	})

	t.Run("a presigned URL refuses another content type", func(t *testing.T) {
		status, upload := presign(k("typed/logo.png"), "image/png")
		require.Equal(t, http.StatusOK, status)

		assert.Equal(t, http.StatusForbidden, fsPutPresigned(t, upload.URL, map[string]string{"Content-Type": "text/html"}, "<script></script>"))
		_, result := list(k("typed/"))
		assert.Empty(t, files(result))
	})

	t.Run("a file the browser has no type for", func(t *testing.T) {
		status, upload := presign(k("untyped/font.woff2"), "")
		require.Equal(t, http.StatusOK, status)
		assert.Equal(t, "application/octet-stream", upload.Headers["Content-Type"])
		assert.Equal(t, http.StatusOK, fsPutPresigned(t, upload.URL, upload.Headers, "font"))

		resp := post("/api/files.delete", map[string]interface{}{"keys": []string{k("untyped/font.woff2")}})
		_ = fsReadBody(t, resp)
	})

	t.Run("deleting a nested folder removes its folders too", func(t *testing.T) {
		for _, folder := range []string{k("nested/"), k("nested/inner/")} {
			resp := post("/api/files.createFolder", map[string]interface{}{"key": folder})
			var created domain.CreateFolderResult
			fsDecodeBody(t, resp, &created)
			assert.Equal(t, domain.FolderCreated, created.Status, folder)
		}
		_, upload := presign(k("nested/inner/a.txt"), "text/plain")
		require.Equal(t, http.StatusOK, fsPutPresigned(t, upload.URL, upload.Headers, "a"))

		resp := post("/api/files.createFolder", map[string]interface{}{"key": k("nested/")})
		var again domain.CreateFolderResult
		fsDecodeBody(t, resp, &again)
		assert.Equal(t, domain.FolderExists, again.Status)

		resp = post("/api/files.deleteFolder", map[string]interface{}{"prefix": k("nested/")})
		var deleted domain.DeleteFolderResult
		fsDecodeBody(t, resp, &deleted)
		assert.Equal(t, domain.DeleteFolderResult{Deleted: 3, DeletedFiles: 1}, deleted)

		_, result := list(k("nested/"))
		assert.Empty(t, keys(result), "a folder deleted before its content would stay on selfhost_s3")
	})

	t.Run("saving the settings without the secret keeps it", func(t *testing.T) {
		resp := updateFileManager(fileManager(""))
		body := fsReadBody(t, resp)
		require.Equal(t, http.StatusOK, resp.StatusCode, body)

		status, _ := list("")
		assert.Equal(t, http.StatusOK, status, "the stored secret still signs the calls")
	})

	t.Run("moving the settings to another host needs the secret again", func(t *testing.T) {
		settings := fileManager("")
		settings["endpoint"] = "http://127.0.0.2:9100"
		resp := updateFileManager(settings)
		body := fsReadBody(t, resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, body)
	})

	t.Run("testing the settings", func(t *testing.T) {
		resp := post("/api/workspaces.testFileManager", map[string]interface{}{"settings": fileManager("")})
		body := fsReadBody(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode, body)

		wrong := fileManager("not-the-secret")
		resp = post("/api/workspaces.testFileManager", map[string]interface{}{"settings": wrong})
		body = fsReadBody(t, resp)
		assert.Equal(t, http.StatusBadGateway, resp.StatusCode, body)

		elsewhere := fileManager("")
		elsewhere["endpoint"] = "http://127.0.0.2:9100"
		resp = post("/api/workspaces.testFileManager", map[string]interface{}{"settings": elsewhere})
		body = fsReadBody(t, resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, body)
	})

	t.Run("a member may not test the settings", func(t *testing.T) {
		asMember(domain.UserPermissions{domain.PermissionResourceTemplates: {Read: true, Write: true}})
		defer asOwner()
		resp := post("/api/workspaces.testFileManager", map[string]interface{}{"settings": fileManager("")})
		_ = fsReadBody(t, resp)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

}
