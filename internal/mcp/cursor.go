package mcp

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
)

// Page sizes of every list tool.
const (
	DefaultPageLimit = 20
	MaxPageLimit     = 50
)

// cursorVersion is the "v" of the cursor envelope.
const cursorVersion = 1

// cursorEnvelope is what a cursor carries, base64url-encoded: the tool that
// issued it, a hash of the workspace and the filters it was issued for, and
// the inner position (the wrapped API's own cursor, or an offset).
type cursorEnvelope struct {
	V int             `json:"v"`
	T string          `json:"t"`
	H string          `json:"h"`
	K json.RawMessage `json:"k"`
}

// filterHash is the first 12 hex characters of SHA-256 over the workspace ID
// and the canonical JSON of the filter arguments (everything but limit and
// cursor). encoding/json writes map keys sorted, which makes it canonical.
func filterHash(workspaceID string, filters map[string]any) string {
	if filters == nil {
		filters = map[string]any{}
	}
	canonical, err := json.Marshal(filters)
	if err != nil {
		canonical = []byte("null")
	}
	sum := sha256.New()
	sum.Write([]byte(workspaceID))
	sum.Write([]byte{0})
	sum.Write(canonical)
	return hex.EncodeToString(sum.Sum(nil))[:12]
}

// EncodeCursor builds the next_cursor a tool returns.
func EncodeCursor(tool, workspaceID string, filters map[string]any, inner any) (string, error) {
	k, err := json.Marshal(inner)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(cursorEnvelope{V: cursorVersion, T: tool, H: filterHash(workspaceID, filters), K: k})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// DecodeCursor reads a cursor back into inner. A cursor that does not decode,
// or was issued by another tool, for another workspace or other filters, is a
// validation error: the client is told to start again without it.
func DecodeCursor(cursor, tool, workspaceID string, filters map[string]any, inner any) *ToolError {
	invalid := validationError("cursor is not valid for this call: drop it to start from the first page")
	data, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return invalid
	}
	var envelope cursorEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return invalid
	}
	if envelope.V != cursorVersion || envelope.T != tool || envelope.H != filterHash(workspaceID, filters) || len(envelope.K) == 0 {
		return invalid
	}
	if err := json.Unmarshal(envelope.K, inner); err != nil {
		return invalid
	}
	return nil
}

// PageLimit applies the default and the bounds to a tool's limit argument.
func PageLimit(limit *int) (int, *ToolError) {
	if limit == nil {
		return DefaultPageLimit, nil
	}
	if *limit < 1 || *limit > MaxPageLimit {
		return 0, validationError("limit must be between 1 and %d", MaxPageLimit)
	}
	return *limit, nil
}
