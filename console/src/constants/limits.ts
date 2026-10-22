// Lengths the API accepts, in characters. Each mirrors a check in internal/domain; the
// console enforces them on its fields so a value too long is caught while it is typed.

// Template name and ID (template.go: validateTemplateName, validateTemplateID).
export const TEMPLATE_NAME_MAX_LENGTH = 255
export const TEMPLATE_ID_MAX_LENGTH = 32

// Transactional notification name and API identifier (transactional.go: CreateTransactionalRequest).
export const TRANSACTIONAL_NAME_MAX_LENGTH = 255
export const TRANSACTIONAL_ID_MAX_LENGTH = 32

// Workspace name (workspace.go: CreateWorkspaceRequest, UpdateWorkspaceRequest).
export const WORKSPACE_NAME_MAX_LENGTH = 32
