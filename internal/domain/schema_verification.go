package domain

import "time"

// SchemaVerificationResult represents the complete verification result
type SchemaVerificationResult struct {
	VerifiedAt   time.Time               `json:"verified_at"`
	SystemDB     DatabaseVerification    `json:"system_db"`
	WorkspaceDBs []WorkspaceVerification `json:"workspace_dbs"`
	Summary      VerificationSummary     `json:"summary"`
}

// VerificationSummary provides aggregate counts
type VerificationSummary struct {
	TotalDatabases  int `json:"total_databases"`
	PassedDatabases int `json:"passed_databases"`
	FailedDatabases int `json:"failed_databases"`
	TotalIssues     int `json:"total_issues"`
}

// DatabaseVerification holds verification results for a single database
type DatabaseVerification struct {
	Status           string                 `json:"status"` // "passed", "failed", "error"
	Error            string                 `json:"error,omitempty"`
	Tables           []TableVerification    `json:"tables"`
	TriggerFunctions []FunctionVerification `json:"trigger_functions"`
	Triggers         []TriggerVerification  `json:"triggers"`
	MissingTables    []string               `json:"missing_tables,omitempty"`
}

// WorkspaceVerification extends DatabaseVerification with workspace info
type WorkspaceVerification struct {
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	DatabaseVerification
}

// TableVerification holds verification results for a table
type TableVerification struct {
	Name           string               `json:"name"`
	Exists         bool                 `json:"exists"`
	Columns        []ColumnVerification `json:"columns,omitempty"`
	Indexes        []IndexVerification  `json:"indexes,omitempty"`
	MissingColumns []string             `json:"missing_columns,omitempty"`
}

// ColumnVerification holds verification results for a column
type ColumnVerification struct {
	Name         string `json:"name"`
	ExpectedType string `json:"expected_type"`
	ActualType   string `json:"actual_type"`
	Matches      bool   `json:"matches"`
}

// IndexVerification holds verification results for an index
type IndexVerification struct {
	Name   string `json:"name"`
	Exists bool   `json:"exists"`
}

// TriggerVerification holds verification results for a trigger
type TriggerVerification struct {
	Name      string `json:"name"`
	TableName string `json:"table_name"`
	Exists    bool   `json:"exists"`
	Function  string `json:"function,omitempty"`
}

// FunctionVerification holds verification results for a function
type FunctionVerification struct {
	Name   string `json:"name"`
	Exists bool   `json:"exists"`
}

// SchemaRepairRequest represents a request to repair schema issues
type SchemaRepairRequest struct {
	WorkspaceIDs    []string `json:"workspace_ids"`
	RepairTriggers  bool     `json:"repair_triggers"`
	RepairFunctions bool     `json:"repair_functions"`
}

// SchemaRepairResult represents the result of a repair operation
type SchemaRepairResult struct {
	RepairedAt   time.Time               `json:"repaired_at"`
	WorkspaceDBs []WorkspaceRepairResult `json:"workspace_dbs"`
	Summary      RepairSummary           `json:"summary"`
}

// RepairSummary provides aggregate counts for repair operations
type RepairSummary struct {
	TotalWorkspaces    int `json:"total_workspaces"`
	SuccessfulRepairs  int `json:"successful_repairs"`
	FailedRepairs      int `json:"failed_repairs"`
	FunctionsRecreated int `json:"functions_recreated"`
	TriggersRecreated  int `json:"triggers_recreated"`
}

// WorkspaceRepairResult holds repair results for a single workspace
type WorkspaceRepairResult struct {
	WorkspaceID        string   `json:"workspace_id"`
	WorkspaceName      string   `json:"workspace_name"`
	Status             string   `json:"status"` // "success", "partial", "failed"
	Error              string   `json:"error,omitempty"`
	FunctionsRecreated []string `json:"functions_recreated"`
	TriggersRecreated  []string `json:"triggers_recreated"`
	FunctionsFailed    []string `json:"functions_failed,omitempty"`
	TriggersFailed     []string `json:"triggers_failed,omitempty"`
}

// ExpectedFunction represents an expected trigger function definition
type ExpectedFunction struct {
	Name string
	SQL  string
}

// ExpectedTrigger represents an expected trigger definition
type ExpectedTrigger struct {
	Name      string
	TableName string
	DropSQL   string
	CreateSQL string
}
