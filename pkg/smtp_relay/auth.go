package smtp_relay

// AuthHandler is a function type that authenticates SMTP credentials
// It receives the username (workspace_id) and password (api_key)
// Returns the workspace_id if authentication succeeds, or an error if it fails
type AuthHandler func(username, password string) (workspaceID string, err error)
