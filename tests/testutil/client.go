package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// APIClient provides HTTP client functionality for integration tests
type APIClient struct {
	baseURL     string
	client      *http.Client
	token       string
	workspaceID string
}

// NewAPIClient creates a new API client for testing
func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetToken sets the authentication token
func (c *APIClient) SetToken(token string) {
	c.token = token
}

// SetWorkspaceID sets the default workspace ID for requests
func (c *APIClient) SetWorkspaceID(workspaceID string) {
	c.workspaceID = workspaceID
}

// GetWorkspaceID returns the current workspace ID
func (c *APIClient) GetWorkspaceID() string {
	return c.workspaceID
}

// Login authenticates using the magic code flow and sets the token
func (c *APIClient) Login(email, password string) error {
	// Step 1: Sign in to get magic code
	signinReq := map[string]string{
		"email": email,
	}

	signinResp, err := c.Post("/api/user.signin", signinReq)
	if err != nil {
		return fmt.Errorf("signin request failed: %w", err)
	}
	defer signinResp.Body.Close()

	if signinResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(signinResp.Body)
		return fmt.Errorf("signin failed with status %d: %s", signinResp.StatusCode, string(body))
	}

	var signinResponse map[string]interface{}
	if err := json.NewDecoder(signinResp.Body).Decode(&signinResponse); err != nil {
		return fmt.Errorf("failed to decode signin response: %w", err)
	}

	code, ok := signinResponse["code"].(string)
	if !ok {
		return fmt.Errorf("magic code not returned in signin response")
	}

	// Step 2: Verify the magic code to get auth token
	verifyReq := map[string]string{
		"email": email,
		"code":  code,
	}

	verifyResp, err := c.Post("/api/user.verify", verifyReq)
	if err != nil {
		return fmt.Errorf("verify request failed: %w", err)
	}
	defer verifyResp.Body.Close()

	if verifyResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(verifyResp.Body)
		return fmt.Errorf("verify failed with status %d: %s", verifyResp.StatusCode, string(body))
	}

	var authResponse struct {
		Token string `json:"token"`
		User  struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
	}

	if err := json.NewDecoder(verifyResp.Body).Decode(&authResponse); err != nil {
		return fmt.Errorf("failed to decode auth response: %w", err)
	}

	c.token = authResponse.Token
	return nil
}

// Get makes a GET request
func (c *APIClient) Get(endpoint string, params ...map[string]string) (*http.Response, error) {
	return c.request(http.MethodGet, endpoint, nil, params...)
}

// Post makes a POST request
func (c *APIClient) Post(endpoint string, body interface{}, params ...map[string]string) (*http.Response, error) {
	return c.request(http.MethodPost, endpoint, body, params...)
}

// Put makes a PUT request
func (c *APIClient) Put(endpoint string, body interface{}, params ...map[string]string) (*http.Response, error) {
	return c.request(http.MethodPut, endpoint, body, params...)
}

// Delete makes a DELETE request
func (c *APIClient) Delete(endpoint string, params ...map[string]string) (*http.Response, error) {
	return c.request(http.MethodDelete, endpoint, nil, params...)
}

// request makes an HTTP request
func (c *APIClient) request(method, endpoint string, body interface{}, params ...map[string]string) (*http.Response, error) {
	// Build URL with query parameters
	reqURL := c.baseURL + endpoint
	if len(params) > 0 && params[0] != nil {
		urlParams := url.Values{}
		for key, value := range params[0] {
			urlParams.Add(key, value)
		}
		if len(urlParams) > 0 {
			reqURL += "?" + urlParams.Encode()
		}
	}

	// Prepare request body
	var reqBody io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(bodyBytes)
	}

	// Create request
	req, err := http.NewRequest(method, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Add authentication token if available
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	// Add workspace ID if available and not already in params
	if c.workspaceID != "" && !strings.Contains(reqURL, "workspace_id=") {
		q := req.URL.Query()
		q.Add("workspace_id", c.workspaceID)
		req.URL.RawQuery = q.Encode()
	}

	// Make request
	return c.client.Do(req)
}

// GetJSON makes a GET request and decodes JSON response
func (c *APIClient) GetJSON(endpoint string, result interface{}, params ...map[string]string) error {
	resp, err := c.Get(endpoint, params...)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// PostJSON makes a POST request and decodes JSON response
func (c *APIClient) PostJSON(endpoint string, reqBody interface{}, result interface{}, params ...map[string]string) error {
	resp, err := c.Post(endpoint, reqBody, params...)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}

	return nil
}

// ExpectStatus checks if response has expected status code
func (c *APIClient) ExpectStatus(resp *http.Response, expectedStatus int) error {
	if resp.StatusCode != expectedStatus {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("expected status %d, got %d: %s", expectedStatus, resp.StatusCode, string(body))
	}
	return nil
}

// ReadBody reads and returns response body as string
func (c *APIClient) ReadBody(resp *http.Response) (string, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// DecodeJSON decodes response body as JSON
func (c *APIClient) DecodeJSON(resp *http.Response, result interface{}) error {
	return json.NewDecoder(resp.Body).Decode(result)
}

// Broadcast API helpers
func (c *APIClient) CreateBroadcast(broadcast map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/broadcasts.create", broadcast)
}

func (c *APIClient) GetBroadcast(broadcastID string) (*http.Response, error) {
	params := map[string]string{
		"id": broadcastID,
	}
	return c.Get("/api/broadcasts.get", params)
}

func (c *APIClient) ListBroadcasts(params map[string]string) (*http.Response, error) {
	return c.Get("/api/broadcasts.list", params)
}

// UpdateBroadcast updates an existing broadcast
func (c *APIClient) UpdateBroadcast(broadcast map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/broadcasts.update", broadcast)
}

// ScheduleBroadcast schedules a broadcast for sending
func (c *APIClient) ScheduleBroadcast(request map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/broadcasts.schedule", request)
}

// PauseBroadcast pauses a sending broadcast
func (c *APIClient) PauseBroadcast(request map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/broadcasts.pause", request)
}

// ResumeBroadcast resumes a paused broadcast
func (c *APIClient) ResumeBroadcast(request map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/broadcasts.resume", request)
}

// CancelBroadcast cancels a scheduled broadcast
func (c *APIClient) CancelBroadcast(request map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/broadcasts.cancel", request)
}

// DeleteBroadcast deletes a broadcast
func (c *APIClient) DeleteBroadcast(request map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/broadcasts.delete", request)
}

// SendBroadcastToIndividual sends a broadcast to an individual recipient
func (c *APIClient) SendBroadcastToIndividual(request map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/broadcasts.sendToIndividual", request)
}

// GetBroadcastTestResults retrieves A/B test results for a broadcast
func (c *APIClient) GetBroadcastTestResults(workspaceID, broadcastID string) (*http.Response, error) {
	params := map[string]string{
		"workspace_id": workspaceID,
		"id":           broadcastID,
	}
	return c.Get("/api/broadcasts.getTestResults", params)
}

// SelectBroadcastWinner manually selects the winning variation for an A/B test
func (c *APIClient) SelectBroadcastWinner(request map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/broadcasts.selectWinner", request)
}

// Contact API helpers
func (c *APIClient) CreateContact(contact map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/contacts.upsert", contact)
}

func (c *APIClient) GetContactByEmail(email string) (*http.Response, error) {
	params := map[string]string{
		"email": email,
	}
	return c.Get("/api/contacts.getByEmail", params)
}

func (c *APIClient) ListContacts(params map[string]string) (*http.Response, error) {
	return c.Get("/api/contacts.list", params)
}

// Template API helpers
func (c *APIClient) CreateTemplate(template map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/templates.create", template)
}

func (c *APIClient) GetTemplate(templateID string) (*http.Response, error) {
	params := map[string]string{
		"id": templateID,
	}
	return c.Get("/api/templates.get", params)
}

func (c *APIClient) UpdateTemplate(template map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/templates.update", template)
}

func (c *APIClient) DeleteTemplate(workspaceID, templateID string) (*http.Response, error) {
	deleteReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"id":           templateID,
	}
	return c.Post("/api/templates.delete", deleteReq)
}

func (c *APIClient) CompileTemplate(compileReq map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/templates.compile", compileReq)
}

func (c *APIClient) ListTemplates(params map[string]string) (*http.Response, error) {
	return c.Get("/api/templates.list", params)
}

// Workspace API helpers
func (c *APIClient) CreateWorkspace(workspace map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/workspaces.create", workspace)
}

func (c *APIClient) GetWorkspace(workspaceID string) (*http.Response, error) {
	params := map[string]string{
		"id": workspaceID,
	}
	return c.Get("/api/workspaces.get", params)
}

// List API helpers
func (c *APIClient) CreateList(list map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/lists.create", list)
}

func (c *APIClient) GetList(listID string) (*http.Response, error) {
	params := map[string]string{
		"id": listID,
	}
	return c.Get("/api/lists.get", params)
}

func (c *APIClient) ListLists(params map[string]string) (*http.Response, error) {
	return c.Get("/api/lists.list", params)
}

// ContactList API methods
func (c *APIClient) GetContactListByIDs(workspaceID, email, listID string) (*http.Response, error) {
	params := map[string]string{
		"workspace_id": workspaceID,
		"email":        email,
		"list_id":      listID,
	}
	return c.Get("/api/contactLists.getByIDs", params)
}

func (c *APIClient) GetContactsByList(workspaceID, listID string) (*http.Response, error) {
	params := map[string]string{
		"workspace_id": workspaceID,
		"list_id":      listID,
	}
	return c.Get("/api/contactLists.getContactsByList", params)
}

func (c *APIClient) GetListsByContact(workspaceID, email string) (*http.Response, error) {
	params := map[string]string{
		"workspace_id": workspaceID,
		"email":        email,
	}
	return c.Get("/api/contactLists.getListsByContact", params)
}

func (c *APIClient) UpdateContactListStatus(workspaceID, email, listID, status string) (*http.Response, error) {
	updateReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"email":        email,
		"list_id":      listID,
		"status":       status,
	}
	return c.Post("/api/contactLists.updateStatus", updateReq)
}

func (c *APIClient) RemoveContactFromList(workspaceID, email, listID string) (*http.Response, error) {
	removeReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"email":        email,
		"list_id":      listID,
	}
	return c.Post("/api/contactLists.removeContact", removeReq)
}

// Task-related API methods

// CreateTask creates a new task
func (c *APIClient) CreateTask(request map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/tasks.create", request)
}

// GetTask retrieves a task by ID
func (c *APIClient) GetTask(workspaceID, taskID string) (*http.Response, error) {
	params := map[string]string{
		"workspace_id": workspaceID,
		"id":           taskID,
	}
	return c.Get("/api/tasks.get", params)
}

// ListTasks retrieves tasks with optional filtering
func (c *APIClient) ListTasks(params map[string]string) (*http.Response, error) {
	return c.Get("/api/tasks.list", params)
}

// DeleteTask deletes a task
func (c *APIClient) DeleteTask(workspaceID, taskID string) (*http.Response, error) {
	params := map[string]string{
		"workspace_id": workspaceID,
		"id":           taskID,
	}
	return c.Post("/api/tasks.delete", nil, params)
}

// ExecuteTask executes a specific task
func (c *APIClient) ExecuteTask(request map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/tasks.execute", request)
}

// ExecutePendingTasks executes pending tasks
func (c *APIClient) ExecutePendingTasks(maxTasks int) (*http.Response, error) {
	params := map[string]string{
		"max_tasks": fmt.Sprintf("%d", maxTasks),
	}
	return c.Get("/api/cron", params)
}

// Webhook registration API methods

// RegisterWebhooks registers webhooks with an email provider
func (c *APIClient) RegisterWebhooks(request map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/webhooks.register", request)
}

// GetWebhookStatus gets the status of webhooks for an email provider
func (c *APIClient) GetWebhookStatus(workspaceID, integrationID string) (*http.Response, error) {
	params := map[string]string{
		"workspace_id":   workspaceID,
		"integration_id": integrationID,
	}
	return c.Get("/api/webhooks.status", params)
}

// Transactional API methods

// CreateTransactionalNotification creates a transactional notification
func (c *APIClient) CreateTransactionalNotification(notification map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/transactional.create", notification)
}

// GetTransactionalNotification gets a transactional notification by ID
func (c *APIClient) GetTransactionalNotification(notificationID string) (*http.Response, error) {
	params := map[string]string{
		"id": notificationID,
	}
	return c.Get("/api/transactional.get", params)
}

// ListTransactionalNotifications lists transactional notifications
func (c *APIClient) ListTransactionalNotifications(params map[string]string) (*http.Response, error) {
	return c.Get("/api/transactional.list", params)
}

// UpdateTransactionalNotification updates a transactional notification
func (c *APIClient) UpdateTransactionalNotification(notificationID string, updates map[string]interface{}) (*http.Response, error) {
	payload := map[string]interface{}{
		"workspace_id": c.workspaceID,
		"id":           notificationID,
		"updates":      updates,
	}
	return c.Post("/api/transactional.update", payload)
}

// DeleteTransactionalNotification deletes a transactional notification
func (c *APIClient) DeleteTransactionalNotification(notificationID string) (*http.Response, error) {
	payload := map[string]interface{}{
		"workspace_id": c.workspaceID,
		"id":           notificationID,
	}
	return c.Post("/api/transactional.delete", payload)
}

// SendTransactionalNotification sends a transactional notification
func (c *APIClient) SendTransactionalNotification(notification map[string]interface{}) (*http.Response, error) {
	return c.Post("/api/transactional.send", map[string]interface{}{
		"workspace_id": c.workspaceID,
		"notification": notification,
	})
}

// TestTransactionalTemplate tests a transactional template
func (c *APIClient) TestTransactionalTemplate(request map[string]interface{}) (*http.Response, error) {
	// Add workspace_id if not already present
	if request["workspace_id"] == nil {
		request["workspace_id"] = c.workspaceID
	}
	return c.Post("/api/transactional.testTemplate", request)
}
