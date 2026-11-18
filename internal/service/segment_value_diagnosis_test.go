package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func diagLeaf(filters ...*domain.DimensionFilter) *domain.TreeNode {
	return &domain.TreeNode{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{Source: "contacts",
		Contact: &domain.ContactCondition{Filters: filters}}}
}

func diagBranch(operator string, leaves ...*domain.TreeNode) *domain.TreeNode {
	return &domain.TreeNode{Kind: "branch", Branch: &domain.TreeNodeBranch{Operator: operator, Leaves: leaves}}
}

var renewedAfter = &domain.DimensionFilter{FieldName: "custom_json_1", FieldType: "time", Operator: "after_date",
	JSONPath: []string{"renewed_at"}, StringValues: []string{"2026-01-01T00:00:00Z"}}

var outOfRange = &pq.Error{Code: "22008", Message: `date/time field value out of range: "2026-02-30"`}

const renewedText = `(custom_json_1['renewed_at'] #>> '{}')`

func TestSegmentValueCandidates(t *testing.T) {
	qb := NewQueryBuilder()
	tree := diagBranch("and",
		diagLeaf(
			&domain.DimensionFilter{FieldName: "custom_json_1", FieldType: "string", Operator: "equals", JSONPath: []string{"plan"}, StringValues: []string{"gold"}},
			&domain.DimensionFilter{FieldName: "custom_number_1", FieldType: "number", Operator: "gt", NumberValues: []float64{1}},
			&domain.DimensionFilter{FieldName: "custom_json_1", FieldType: "time", Operator: "is_set", JSONPath: []string{"renewed_at"}},
			&domain.DimensionFilter{FieldName: "custom_json_3", FieldType: "string", Operator: "in_array", JSONPath: []string{"tags"}, StringValues: []string{"a"}},
			renewedAfter,
		),
		diagBranch("or",
			diagLeaf(&domain.DimensionFilter{FieldName: "custom_json_2", FieldType: "number", Operator: "gt",
				JSONPath: []string{"orders", "0", "total"}, NumberValues: []float64{10}}),
			diagLeaf(&domain.DimensionFilter{FieldName: "custom_json_1", FieldType: "time", Operator: "in_the_last_days",
				JSONPath: []string{"seen_at"}, StringValues: []string{"7"}}),
			&domain.TreeNode{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{Source: "contact_timeline", ContactTimeline: &domain.ContactTimelineCondition{
				Kind: "email.opened", CountOperator: "at_least", CountValue: 1}}},
		),
		diagLeaf(&domain.DimensionFilter{FieldName: "custom_json_4", FieldType: "number", Operator: "lt", NumberValues: []float64{3}}),
		// The same column, path and type again is not a second candidate.
		diagLeaf(&domain.DimensionFilter{FieldName: "custom_json_1", FieldType: "time", Operator: "before_date",
			JSONPath: []string{"renewed_at"}, StringValues: []string{"2027-01-01T00:00:00Z"}}),
	)

	assert.Equal(t, []segmentValueCandidate{
		{column: "custom_json_1", path: []string{"renewed_at"}, valueType: domain.SegmentValueTypeDate},
		{column: "custom_json_2", path: []string{"orders", "0", "total"}, valueType: domain.SegmentValueTypeNumber},
		{column: "custom_json_1", path: []string{"seen_at"}, valueType: domain.SegmentValueTypeDate},
		{column: "custom_json_4", path: []string{}, valueType: domain.SegmentValueTypeNumber},
	}, segmentValueCandidates(qb, tree))
}

func TestQuotedValue(t *testing.T) {
	v, ok := quotedValue(`date/time field value out of range: "2026-02-30"`)
	assert.True(t, ok)
	assert.Equal(t, "2026-02-30", v)

	v, ok = quotedValue(`invalid input syntax for type numeric: "say "hi""`)
	assert.True(t, ok)
	assert.Equal(t, `say "hi"`, v, "up to the last quote")

	v, ok = quotedValue(`invalid input syntax for type timestamp with time zone: ""`)
	assert.True(t, ok)
	assert.Equal(t, "", v)

	_, ok = quotedValue(`value overflows numeric format`)
	assert.False(t, ok)
}

func newDiagnosisMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, mock
}

func TestDiagnoseSegmentValue(t *testing.T) {
	ctx := context.Background()
	valueMatched := `SELECT email, ` + renewedText + ` FROM contacts WHERE ` + renewedText + ` IS NOT NULL AND NOT pg_input_is_valid(` + renewedText + `, 'timestamptz') AND ` + renewedText + ` = $1 LIMIT 1`
	anyValue := `SELECT email, ` + renewedText + ` FROM contacts WHERE ` + renewedText + ` IS NOT NULL AND NOT pg_input_is_valid(` + renewedText + `, 'timestamptz') LIMIT 1`

	t.Run("the value the message quotes is searched first", func(t *testing.T) {
		db, mock := newDiagnosisMock(t)
		mock.ExpectQuery(valueMatched).WithArgs("2026-02-30").
			WillReturnRows(sqlmock.NewRows([]string{"email", "v"}).AddRow("jane@example.com", "2026-02-30"))

		got := diagnoseSegmentValue(ctx, db, diagLeaf(renewedAfter), outOfRange, nil)
		assert.Equal(t, &domain.SegmentValueError{Column: "custom_json_1", Path: []string{"renewed_at"},
			Type: domain.SegmentValueTypeDate, Email: "jane@example.com", Value: "2026-02-30", Cause: outOfRange.Message}, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no contact holds that value: any value that does not convert", func(t *testing.T) {
		db, mock := newDiagnosisMock(t)
		mock.ExpectQuery(valueMatched).WithArgs("2026-02-30").WillReturnRows(sqlmock.NewRows([]string{"email", "v"}))
		mock.ExpectQuery(anyValue).WithArgs().
			WillReturnRows(sqlmock.NewRows([]string{"email", "v"}).AddRow("joe@example.com", "soon"))

		got := diagnoseSegmentValue(ctx, db, diagLeaf(renewedAfter), outOfRange, nil)
		require.NotNil(t, got)
		assert.Equal(t, "joe@example.com", got.Email)
		assert.Equal(t, "soon", got.Value)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("limited to the emails given", func(t *testing.T) {
		db, mock := newDiagnosisMock(t)
		mock.ExpectQuery(`SELECT email, `+renewedText+` FROM contacts WHERE `+renewedText+` IS NOT NULL AND NOT pg_input_is_valid(`+renewedText+`, 'timestamptz') AND `+renewedText+` = $1 AND email = ANY($2) LIMIT 1`).
			WithArgs("2026-02-30", `{"a@example.com","b@example.com"}`).
			WillReturnRows(sqlmock.NewRows([]string{"email", "v"}).AddRow("b@example.com", "2026-02-30"))

		got := diagnoseSegmentValue(ctx, db, diagLeaf(renewedAfter), outOfRange, []string{"a@example.com", "b@example.com"})
		require.NotNil(t, got)
		assert.Equal(t, "b@example.com", got.Email)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("every filter in tree order, an array index and a relative-day operator", func(t *testing.T) {
		db, mock := newDiagnosisMock(t)
		items := `(custom_json_1['items'][0]['due'] #>> '{}')`
		seen := `(custom_json_1['seen_at'] #>> '{}')`
		tree := diagBranch("or",
			diagLeaf(&domain.DimensionFilter{FieldName: "custom_json_1", FieldType: "time", Operator: "after_date",
				JSONPath: []string{"items", "0", "due"}, StringValues: []string{"2026-01-01T00:00:00Z"}}),
			diagLeaf(&domain.DimensionFilter{FieldName: "custom_json_1", FieldType: "time", Operator: "not_in_the_last_days",
				JSONPath: []string{"seen_at"}, StringValues: []string{"30"}}),
		)
		// A numeric overflow quotes nothing: the search for any value runs alone.
		overflow := &pq.Error{Code: "22008", Message: "timestamp out of range"}
		mock.ExpectQuery(`SELECT email, ` + items + ` FROM contacts WHERE ` + items + ` IS NOT NULL AND NOT pg_input_is_valid(` + items + `, 'timestamptz') LIMIT 1`).
			WillReturnRows(sqlmock.NewRows([]string{"email", "v"}))
		mock.ExpectQuery(`SELECT email, ` + seen + ` FROM contacts WHERE ` + seen + ` IS NOT NULL AND NOT pg_input_is_valid(` + seen + `, 'timestamptz') LIMIT 1`).
			WillReturnRows(sqlmock.NewRows([]string{"email", "v"}).AddRow("jane@example.com", "300000-01-01"))

		got := diagnoseSegmentValue(ctx, db, tree, overflow, nil)
		require.NotNil(t, got)
		assert.Equal(t, []string{"seen_at"}, got.Path)
		assert.Equal(t, "300000-01-01", got.Value)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("the whole column is read only when it is an object", func(t *testing.T) {
		db, mock := newDiagnosisMock(t)
		whole := `(custom_json_2 #>> '{}')`
		tree := diagLeaf(&domain.DimensionFilter{FieldName: "custom_json_2", FieldType: "number", Operator: "gt", NumberValues: []float64{1}})
		invalid := &pq.Error{Code: "22P02", Message: `invalid input syntax for type numeric: "{"a": 1}"`}
		mock.ExpectQuery(`SELECT email, ` + whole + ` FROM contacts WHERE ` + whole + ` IS NOT NULL AND NOT pg_input_is_valid(` + whole + `, 'numeric') AND jsonb_typeof(custom_json_2) = 'object' AND ` + whole + ` = $1 LIMIT 1`).
			WithArgs(`{"a": 1}`).
			WillReturnRows(sqlmock.NewRows([]string{"email", "v"}).AddRow("jane@example.com", `{"a": 1}`))

		got := diagnoseSegmentValue(ctx, db, tree, invalid, nil)
		require.NotNil(t, got)
		assert.Equal(t, "custom_json_2", got.Column)
		assert.Empty(t, got.Path)
		assert.Equal(t, domain.SegmentValueTypeNumber, got.Type)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a long value is cut", func(t *testing.T) {
		db, mock := newDiagnosisMock(t)
		long := ""
		for i := 0; i < 200; i++ {
			long += "x"
		}
		mock.ExpectQuery(anyValue).WillReturnRows(sqlmock.NewRows([]string{"email", "v"}).AddRow("jane@example.com", long))
		got := diagnoseSegmentValue(ctx, db, diagLeaf(renewedAfter), &pq.Error{Code: "22007", Message: "invalid"}, nil)
		require.NotNil(t, got)
		assert.Equal(t, domain.TruncateSegmentValue(long), got.Value)
	})

	t.Run("no direct-cast JSON filter: not a contact value", func(t *testing.T) {
		db, mock := newDiagnosisMock(t)
		tree := diagLeaf(&domain.DimensionFilter{FieldName: "custom_datetime_1", FieldType: "time", Operator: "after_date",
			StringValues: []string{"2026-01-01T00:00:00Z"}})
		assert.Nil(t, diagnoseSegmentValue(ctx, db, tree, outOfRange, nil))
		require.NoError(t, mock.ExpectationsWereMet(), "no query is run")
	})

	t.Run("a date error with only number filters: not a contact value", func(t *testing.T) {
		db, _ := newDiagnosisMock(t)
		tree := diagLeaf(&domain.DimensionFilter{FieldName: "custom_json_2", FieldType: "number", Operator: "gt",
			JSONPath: []string{"score"}, NumberValues: []float64{1}})
		assert.Nil(t, diagnoseSegmentValue(ctx, db, tree, outOfRange, nil))
	})

	t.Run("nothing found with one candidate keeps its field", func(t *testing.T) {
		db, mock := newDiagnosisMock(t)
		mock.ExpectQuery(valueMatched).WithArgs("2026-02-30").WillReturnRows(sqlmock.NewRows([]string{"email", "v"}))
		mock.ExpectQuery(anyValue).WillReturnRows(sqlmock.NewRows([]string{"email", "v"}))

		got := diagnoseSegmentValue(ctx, db, diagLeaf(renewedAfter), outOfRange, nil)
		assert.Equal(t, &domain.SegmentValueError{Column: "custom_json_1", Path: []string{"renewed_at"},
			Type: domain.SegmentValueTypeDate, Cause: outOfRange.Message}, got)
		assert.False(t, got.HasCulprit())
	})

	t.Run("a timeout gives the cause alone", func(t *testing.T) {
		db, mock := newDiagnosisMock(t)
		other := &domain.DimensionFilter{FieldName: "custom_json_3", FieldType: "time", Operator: "after_date",
			JSONPath: []string{"born"}, StringValues: []string{"2000-01-01T00:00:00Z"}}
		mock.ExpectQuery(valueMatched).WithArgs("2026-02-30").WillDelayFor(time.Second).
			WillReturnRows(sqlmock.NewRows([]string{"email", "v"}).AddRow("late@example.com", "2026-02-30"))

		timeoutCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
		defer cancel()
		got := diagnoseSegmentValue(timeoutCtx, db, diagLeaf(renewedAfter, other), outOfRange, nil)
		assert.Equal(t, &domain.SegmentValueError{Cause: outOfRange.Message}, got,
			"two filters could have raised: no field is named")
	})

	t.Run("a failed query gives the cause alone", func(t *testing.T) {
		db, mock := newDiagnosisMock(t)
		mock.ExpectQuery(valueMatched).WithArgs("2026-02-30").WillReturnError(errors.New("boom"))
		got := diagnoseSegmentValue(ctx, db, diagLeaf(renewedAfter), outOfRange, nil)
		require.NotNil(t, got)
		assert.False(t, got.HasCulprit())
	})

	t.Run("no cause", func(t *testing.T) {
		db, _ := newDiagnosisMock(t)
		assert.Nil(t, diagnoseSegmentValue(ctx, db, diagLeaf(renewedAfter), nil, nil))
	})
}
