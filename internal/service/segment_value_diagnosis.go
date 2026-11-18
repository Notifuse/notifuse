package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/lib/pq"
)

// segmentDiagnosisTimeout bounds the diagnosis of a failed segment query. Past it, the error
// carries the database's message alone.
const segmentDiagnosisTimeout = 5 * time.Second

// segmentDiagnosisDB is what the diagnosis reads contacts through.
type segmentDiagnosisDB interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// segmentValueCandidate is a filter of a segment that casts a custom JSON value directly, and so
// can raise on a contact's value.
type segmentValueCandidate struct {
	column    string
	path      []string
	valueType string
}

// field is the candidate's column and path, joined with dots.
func (c segmentValueCandidate) field() string {
	return c.column + "." + strings.Join(c.path, ".")
}

// segmentValueCandidates lists, in tree order, the contact filters compiled with castDirectly: a
// custom JSON column compared as a number or a date, relative-day operators, array-index paths and
// the whole column included. Existence checks and in_array cast nothing. A column and path read
// as the same type twice is listed once.
func segmentValueCandidates(qb *QueryBuilder, tree *domain.TreeNode) []segmentValueCandidate {
	var candidates []segmentValueCandidate
	seen := map[string]bool{}
	var walk func(node *domain.TreeNode)
	walk = func(node *domain.TreeNode) {
		if node == nil {
			return
		}
		if node.Branch != nil {
			for _, child := range node.Branch.Leaves {
				walk(child)
			}
		}
		leaf := node.Leaf
		if leaf == nil || leaf.Source != "contacts" || leaf.Contact == nil {
			return
		}
		for _, filter := range leaf.Contact.Filters {
			if filter == nil {
				continue
			}
			cfg, ok := qb.allowedFields[filter.FieldName]
			if !ok || cfg.fieldType != "json" {
				continue
			}
			switch filter.Operator {
			case "is_set", "is_not_set", "in_array":
				continue
			}
			var valueType string
			switch filter.FieldType {
			case "time":
				valueType = domain.SegmentValueTypeDate
			case "number":
				valueType = domain.SegmentValueTypeNumber
			default:
				continue
			}
			candidate := segmentValueCandidate{column: cfg.dbColumn, path: append([]string{}, filter.JSONPath...), valueType: valueType}
			key := valueType + ":" + candidate.field()
			if seen[key] {
				continue
			}
			seen[key] = true
			candidates = append(candidates, candidate)
		}
	}
	walk(tree)
	return candidates
}

// segmentValueTypeOfCode is the type a conversion error code comes from: the date codes from a
// timestamptz cast, the others from a numeric one.
func segmentValueTypeOfCode(code pq.ErrorCode) string {
	switch code {
	case "22007", "22008":
		return domain.SegmentValueTypeDate
	case "22P02", "22003":
		return domain.SegmentValueTypeNumber
	}
	return ""
}

// quotedValue is the value PostgreSQL quotes in a conversion error, such as
// `date/time field value out of range: "2026-02-30"`: the text between the first `: "` and the
// last `"`. ok is false when the message quotes nothing (a numeric overflow does not).
func quotedValue(message string) (string, bool) {
	start := strings.Index(message, `: "`)
	if start < 0 {
		return "", false
	}
	end := strings.LastIndex(message, `"`)
	if end < start+3 {
		return "", false
	}
	return message[start+3 : end], true
}

// diagnoseSegmentValue finds which contact value made a segment query raise cause, a conversion
// error, by looking for a value of a direct-cast JSON filter that does not convert. emails limits
// the search to the contacts the failing query read (a build batch, the queue's contact); none
// searches every contact.
//
// It returns nil when the tree casts no custom JSON value as the type the error comes from: the
// error is then not about a contact value. With a culprit, the error names it; without one, or
// when a query fails (the context's deadline included), it carries the database's message alone,
// and the column, path and type only when a single filter could have raised.
//
// Each filter is read as its compiled query reads it (buildJSONPath, jsonValueText), with the
// whole column limited to objects as guardWholeColumnCast does. When the message quotes the value,
// every filter is first searched for that very value, so that the contact named is the one that
// raised; then for any value that does not convert. No ORDER BY: any culprit will do. It runs only
// once a query has failed.
func diagnoseSegmentValue(ctx context.Context, db segmentDiagnosisDB, tree *domain.TreeNode, cause *pq.Error, emails []string) *domain.SegmentValueError {
	if cause == nil {
		return nil
	}
	qb := NewQueryBuilder()
	causeType := segmentValueTypeOfCode(cause.Code)
	var candidates []segmentValueCandidate
	for _, candidate := range segmentValueCandidates(qb, tree) {
		if candidate.valueType == causeType {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	causeOnly := &domain.SegmentValueError{Cause: cause.Message}
	if len(candidates) == 1 {
		causeOnly.Column = candidates[0].column
		causeOnly.Path = append([]string{}, candidates[0].path...)
		causeOnly.Type = candidates[0].valueType
	}

	value, quoted := quotedValue(cause.Message)
	passes := []bool{false}
	if quoted {
		passes = []bool{true, false}
	}
	for _, matchValue := range passes {
		for _, candidate := range candidates {
			query, args := segmentDiagnosisQuery(qb, candidate, matchValue, value, emails)
			var email, held string
			err := db.QueryRowContext(ctx, query, args...).Scan(&email, &held)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return causeOnly
			}
			return &domain.SegmentValueError{
				Column: candidate.column,
				Path:   append([]string{}, candidate.path...),
				Type:   candidate.valueType,
				Email:  email,
				Value:  domain.TruncateSegmentValue(held),
				Cause:  cause.Message,
			}
		}
	}
	return causeOnly
}

// segmentDiagnosisQuery reads one contact holding, for candidate, a value that does not convert to
// its type: that value when matchValue is set, any one otherwise.
func segmentDiagnosisQuery(qb *QueryBuilder, candidate segmentValueCandidate, matchValue bool, value string, emails []string) (string, []interface{}) {
	text := jsonValueText(qb.buildJSONPath(candidate.column, candidate.path))
	pgType := "numeric"
	if candidate.valueType == domain.SegmentValueTypeDate {
		pgType = "timestamptz"
	}
	conditions := []string{
		text + " IS NOT NULL",
		fmt.Sprintf("NOT pg_input_is_valid(%s, '%s')", text, pgType),
	}
	if len(candidate.path) == 0 {
		conditions = append(conditions, fmt.Sprintf("jsonb_typeof(%s) = 'object'", candidate.column))
	}
	var args []interface{}
	if matchValue {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf("%s = $%d", text, len(args)))
	}
	if len(emails) > 0 {
		args = append(args, pq.Array(emails))
		conditions = append(conditions, fmt.Sprintf("email = ANY($%d)", len(args)))
	}
	return fmt.Sprintf("SELECT email, %s FROM contacts WHERE %s LIMIT 1", text, strings.Join(conditions, " AND ")), args
}
