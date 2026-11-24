package analytics

import (
	"database/sql"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// SQLBuilder provides methods to convert analytics queries to SQL
type SQLBuilder struct {
	placeholder squirrel.PlaceholderFormat
}

// NewSQLBuilder creates a new SQL builder with PostgreSQL placeholder format
func NewSQLBuilder() *SQLBuilder {
	return &SQLBuilder{
		placeholder: squirrel.Dollar,
	}
}

// BuildSQL converts an analytics Query to SQL using the provided schema definition
func (sb *SQLBuilder) BuildSQL(query Query, schema SchemaDefinition) (string, []interface{}, error) {
	// Start building the SELECT query
	selectBuilder := squirrel.Select().PlaceholderFormat(sb.placeholder)

	// Add measures to SELECT clause
	for _, measure := range query.Measures {
		measureDef, exists := schema.Measures[measure]
		if !exists {
			return "", nil, fmt.Errorf("measure '%s' not found in schema", measure)
		}

		// Build the SQL for the measure
		var measureSQL string
		if measureDef.SQL != "" {
			// Check if the measure type requires wrapping with aggregate function
			measureSQL = sb.buildMeasureSQL(measureDef.Type, measureDef.SQL, measureDef.Filters)
		} else {
			// Use measure name directly if no SQL provided
			measureSQL = measure
		}

		selectBuilder = selectBuilder.Column(fmt.Sprintf("(%s) AS %s", measureSQL, measure))
	}

	// Add dimensions to SELECT clause
	for _, dimension := range query.Dimensions {
		dimensionDef, exists := schema.Dimensions[dimension]
		if !exists {
			return "", nil, fmt.Errorf("dimension '%s' not found in schema", dimension)
		}

		// Use custom SQL if provided, otherwise use the dimension name
		if dimensionDef.SQL != "" {
			selectBuilder = selectBuilder.Column(fmt.Sprintf("%s AS %s", dimensionDef.SQL, dimension))
		} else {
			selectBuilder = selectBuilder.Column(dimension)
		}
	}

	// Add time dimensions to SELECT clause and GROUP BY
	timeDimensionColumns := make([]string, 0, len(query.TimeDimensions))
	for _, timeDim := range query.TimeDimensions {
		dimensionDef, exists := schema.Dimensions[timeDim.Dimension]
		if !exists {
			return "", nil, fmt.Errorf("time dimension '%s' not found in schema", timeDim.Dimension)
		}

		// Generate time dimension SQL based on granularity
		timeDimSQL, err := sb.buildTimeDimensionSQL(timeDim, dimensionDef, query.GetDefaultTimezone())
		if err != nil {
			return "", nil, fmt.Errorf("failed to build time dimension SQL: %w", err)
		}

		columnAlias := fmt.Sprintf("%s_%s", timeDim.Dimension, timeDim.Granularity)
		selectBuilder = selectBuilder.Column(fmt.Sprintf("(%s) AS %s", timeDimSQL, columnAlias))
		timeDimensionColumns = append(timeDimensionColumns, columnAlias)
	}

	// Set FROM clause
	selectBuilder = selectBuilder.From(schema.Name)

	// Add WHERE clauses for filters
	var pruneStart, pruneEnd *time.Time
	for _, filter := range query.Filters {
		// A range filter on a time dimension prunes partitions just as well as
		// a time dimension does. Breakdowns carry their range this way because
		// a granularity would split every row per bucket, and without this
		// they would scan the whole history instead of one partition.
		if filter.Operator == "inDateRange" && len(filter.Values) == 2 {
			if dimensionDef, exists := schema.Dimensions[filter.Member]; exists && dimensionDef.Type == "time" {
				timezone := query.GetDefaultTimezone()
				start, startErr := parseDateRangeBound(filter.Values[0], timezone)
				end, endErr := parseDateRangeBound(filter.Values[1], timezone)
				if startErr == nil && endErr == nil {
					if pruneStart == nil || start.Before(*pruneStart) {
						pruneStart = &start
					}
					if pruneEnd == nil || end.After(*pruneEnd) {
						pruneEnd = &end
					}
				}
			}
		}

		// A filter applies to rows, before aggregation, so its member is a dimension. A measure's
		// SQL in WHERE is either invalid (* or an aggregate) or, for a column measure, the bare
		// column without its aggregate and its filters: a silently different number.
		var memberSQL string
		if dimensionDef, exists := schema.Dimensions[filter.Member]; exists {
			memberSQL = dimensionDef.SQL
			if memberSQL == "" {
				memberSQL = filter.Member
			}
		} else if _, exists := schema.Measures[filter.Member]; exists {
			return "", nil, invalid(ErrUnsupportedDimension, "filters take dimensions, and %q is a measure: put conditions on measures in having", filter.Member)
		} else {
			return "", nil, fmt.Errorf("filter member '%s' not found in schema", filter.Member)
		}

		// Build WHERE condition based on operator
		condition, err := sb.buildFilterCondition(memberSQL, filter)
		if err != nil {
			return "", nil, fmt.Errorf("failed to build filter condition: %w", err)
		}

		selectBuilder = selectBuilder.Where(condition)
	}

	// Add time dimension date range filters.
	//
	// The bounds are compared against the bare column so indexes (BRIN) and
	// partition pruning stay usable: for non-UTC timezones the local bounds
	// are converted to UTC instants in Go instead of wrapping the column in
	// AT TIME ZONE (which is kept only inside DATE_TRUNC bucketing). The UTC
	// path binds the raw strings, byte-identical to the historical SQL.
	for _, timeDim := range query.TimeDimensions {
		if timeDim.DateRange == nil {
			continue
		}
		dimensionDef := schema.Dimensions[timeDim.Dimension]
		dimensionSQL := dimensionDef.SQL
		if dimensionSQL == "" {
			dimensionSQL = timeDim.Dimension
		}

		timezone := query.GetDefaultTimezone()
		startLocal, startErr := parseDateRangeBound(timeDim.DateRange[0], timezone)
		endLocal, endErr := parseDateRangeBound(timeDim.DateRange[1], timezone)

		// A bare date names a whole day, so the range's last day belongs to
		// it. Anything else would make a time series silently lose its most
		// recent day — and a single-day range ("today") return nothing at all
		// — since the gap filler only accepts date-only bounds and leaves the
		// caller no way to say "up to the end of that day".
		if endErr == nil && isDateOnlyBound(timeDim.DateRange[1]) {
			endLocal = endLocal.AddDate(0, 0, 1).Add(-time.Millisecond)
		}

		if startErr == nil && endErr == nil {
			if pruneStart == nil || startLocal.Before(*pruneStart) {
				pruneStart = &startLocal
			}
			if pruneEnd == nil || endLocal.After(*pruneEnd) {
				pruneEnd = &endLocal
			}

			// Both bounds are instants now, so the column is compared bare and
			// stays sargable whatever the timezone.
			selectBuilder = selectBuilder.Where(squirrel.GtOrEq{dimensionSQL: startLocal.UTC()})
			selectBuilder = selectBuilder.Where(squirrel.LtOrEq{dimensionSQL: endLocal.UTC()})
			continue
		}

		if timezone != "UTC" {
			// Unparseable bounds: keep the legacy (non-sargable) comparison so
			// exotic formats behave as before.
			sanitizedTimezone := sb.sanitizeTimezone(timezone)
			if sanitizedTimezone != "" {
				dimensionSQL = fmt.Sprintf("%s AT TIME ZONE '%s'", dimensionSQL, sanitizedTimezone)
			}
		}

		selectBuilder = selectBuilder.Where(squirrel.GtOrEq{dimensionSQL: timeDim.DateRange[0]})
		selectBuilder = selectBuilder.Where(squirrel.LtOrEq{dimensionSQL: timeDim.DateRange[1]})
	}

	// Partition pruning: widen the requested range by the schema's slacks and
	// constrain the partition key column so the planner can skip partitions.
	if schema.PartitionHint != nil && pruneStart != nil && pruneEnd != nil {
		hint := schema.PartitionHint
		selectBuilder = selectBuilder.Where(squirrel.GtOrEq{hint.Column: pruneStart.UTC().Add(-hint.SlackBefore).Format("2006-01-02")})
		selectBuilder = selectBuilder.Where(squirrel.LtOrEq{hint.Column: pruneEnd.UTC().Add(hint.SlackAfter).Format("2006-01-02")})
	}

	// Add GROUP BY clause
	groupByColumns := make([]string, 0, len(query.Dimensions)+len(timeDimensionColumns))

	// Add regular dimensions to GROUP BY
	for _, dimension := range query.Dimensions {
		dimensionDef := schema.Dimensions[dimension]
		if dimensionDef.SQL != "" {
			groupByColumns = append(groupByColumns, dimensionDef.SQL)
		} else {
			groupByColumns = append(groupByColumns, dimension)
		}
	}

	// Add time dimensions to GROUP BY
	groupByColumns = append(groupByColumns, timeDimensionColumns...)

	if len(groupByColumns) > 0 {
		selectBuilder = selectBuilder.GroupBy(groupByColumns...)
	}

	// Metric filters (HAVING): conditions on fully-aggregated measures.
	for _, having := range query.Having {
		measureDef, exists := schema.Measures[having.Member]
		if !exists {
			return "", nil, fmt.Errorf("having member '%s' is not a measure in schema", having.Member)
		}
		measureSQL := measureDef.SQL
		if measureSQL == "" {
			measureSQL = having.Member
		}
		aggregateSQL := sb.buildMeasureSQL(measureDef.Type, measureSQL, measureDef.Filters)
		condition, err := sb.buildFilterCondition(aggregateSQL, having)
		if err != nil {
			return "", nil, fmt.Errorf("failed to build having condition: %w", err)
		}
		selectBuilder = selectBuilder.Having(condition)
	}

	// Add ORDER BY clause, in the caller's priority (OrderKeys). columnOrder is the same order
	// on the columns the query returns, for the ranking of topGroupCells.
	var orderClauses, columnOrder []string
	for _, field := range query.OrderKeys() {
		direction := query.Order[field]
		orderDirection := strings.ToUpper(direction)
		if orderDirection != "ASC" && orderDirection != "DESC" {
			orderDirection = "ASC"
		}

		// Check if field exists in measures or dimensions. A measure orders by its aggregate,
		// as HAVING filters by it: its raw SQL (* for a count, a column for a max) is not valid
		// outside the aggregate, and the aggregate does not need the measure to be selected.
		var fieldSQL, column string
		// A measure can be NULL (a median or a rate over a group with nothing to count), and
		// PostgreSQL puts NULLs first in a descending sort: a group with no value would top every
		// "best first" ranking. Ascending already sorts them last.
		nulls := ""
		if measureDef, exists := schema.Measures[field]; exists {
			nulls = " NULLS LAST"
			measureSQL := measureDef.SQL
			if measureSQL == "" {
				measureSQL = field
			}
			fieldSQL = sb.buildMeasureSQL(measureDef.Type, measureSQL, measureDef.Filters)
			column = field
		} else if dimensionDef, exists := schema.Dimensions[field]; exists && slices.Contains(query.Dimensions, field) {
			fieldSQL = dimensionDef.SQL
			if fieldSQL == "" {
				fieldSQL = field
			}
			column = field
		} else if bucket, ok := timeDimensionColumn(query, field); ok {
			// The time dimension is grouped by its bucket, so it sorts by the bucket's alias:
			// the bare column is not in GROUP BY.
			fieldSQL = bucket
			column = bucket
		} else if exists {
			return "", nil, invalid(ErrInvalidQuery, "order by dimension %q needs it in dimensions or as the time dimension", field)
		} else {
			return "", nil, fmt.Errorf("order field '%s' not found in schema", field)
		}

		orderClauses = append(orderClauses, fmt.Sprintf("%s %s%s", fieldSQL, orderDirection, nulls))
		columnOrder = append(columnOrder, fmt.Sprintf("%s %s%s", column, orderDirection, nulls))
	}

	if query.topGroupCells != nil && len(query.Dimensions) > 0 {
		return sb.topGroupsSQL(query, selectBuilder, timeDimensionColumns, columnOrder)
	}
	if len(orderClauses) > 0 {
		selectBuilder = selectBuilder.OrderBy(orderClauses...)
	}

	// Add LIMIT and OFFSET
	if query.Limit != nil {
		selectBuilder = selectBuilder.Limit(uint64(*query.Limit))
	}
	if query.Offset != nil {
		selectBuilder = selectBuilder.Offset(uint64(*query.Offset))
	}

	// Build the final SQL
	sql, args, err := selectBuilder.ToSql()
	if err != nil {
		return "", nil, fmt.Errorf("failed to build SQL: %w", err)
	}

	return sql, args, nil
}

// topGroupsSQL wraps the grouped query so that it returns the full series of the groups that
// hold its first topGroupCells rows, in the query's order, and nothing of the other groups.
// The rows of the query are ranked over the whole range (cell_rank), each group takes the
// rank of its best row (group_rank), and the groups ranked within topGroupCells are kept.
// The page the caller asked for is among their rows, and since each kept group comes back
// whole, the gap filling adds zeros only where that group has no row.
func (sb *SQLBuilder) topGroupsSQL(query Query, grouped squirrel.SelectBuilder, timeDimensionColumns []string, columnOrder []string) (string, []interface{}, error) {
	innerSQL, args, err := grouped.ToSql()
	if err != nil {
		return "", nil, fmt.Errorf("failed to build SQL: %w", err)
	}

	columns := make([]string, 0, len(query.Measures)+len(query.Dimensions)+len(timeDimensionColumns))
	columns = append(columns, query.Measures...)
	columns = append(columns, query.Dimensions...)
	columns = append(columns, timeDimensionColumns...)
	selected := strings.Join(columns, ", ")

	orderSQL := strings.Join(columnOrder, ", ")
	outerOrder := ""
	if orderSQL != "" {
		outerOrder = " ORDER BY " + orderSQL
	}
	limit := ""
	if query.Limit != nil {
		limit = fmt.Sprintf(" LIMIT %d", *query.Limit)
	}

	sql := fmt.Sprintf("SELECT %s FROM ("+
		"SELECT ranked.*, MIN(analytics_cell_rank) OVER (PARTITION BY %s) AS analytics_group_rank FROM ("+
		"SELECT cells.*, ROW_NUMBER() OVER (ORDER BY %s) AS analytics_cell_rank FROM (%s) AS cells"+
		") AS ranked"+
		") AS top_groups WHERE analytics_group_rank <= %d%s%s",
		selected, strings.Join(query.Dimensions, ", "), orderSQL, innerSQL, *query.topGroupCells, outerOrder, limit)
	return sql, args, nil
}

// isDateOnlyBound reports whether a bound names a day rather than an instant.
func isDateOnlyBound(value string) bool {
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

// parseDateRangeBound parses a dateRange bound in the query's timezone.
// Accepted formats mirror what the console sends: a bare date (interpreted at
// local midnight, matching the historical text comparison), a date-time, or
// RFC 3339.
func parseDateRangeBound(value string, timezone string) (time.Time, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}
	for _, layout := range []string{"2006-01-02", "2006-01-02 15:04:05", time.RFC3339} {
		if parsed, err := time.ParseInLocation(layout, value, loc); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported dateRange bound: %q", value)
}

// buildTimeDimensionSQL generates SQL for time dimension grouping based on granularity
func (sb *SQLBuilder) buildTimeDimensionSQL(timeDim TimeDimension, dimensionDef DimensionDefinition, timezone string) (string, error) {
	dimensionSQL := dimensionDef.SQL
	if dimensionSQL == "" {
		dimensionSQL = timeDim.Dimension
	}

	// Bucket in the query's timezone — including UTC.
	//
	// DATE_TRUNC on a timestamptz truncates in the *database session's* zone,
	// so omitting this for UTC does not mean "group by UTC days", it means
	// "group by whatever days the server happens to use". On a server set to
	// anything else the buckets come back shifted by its offset, which then
	// fails to line up with the UTC keys the gap filler generates and zeroes
	// out the entire series.
	if sanitizedTimezone := sb.sanitizeTimezone(timezone); sanitizedTimezone != "" {
		dimensionSQL = fmt.Sprintf("%s AT TIME ZONE '%s'", dimensionSQL, sanitizedTimezone)
	}

	switch timeDim.Granularity {
	case "hour":
		return fmt.Sprintf("DATE_TRUNC('hour', %s)", dimensionSQL), nil
	case "day":
		return fmt.Sprintf("DATE_TRUNC('day', %s)", dimensionSQL), nil
	case "week":
		return fmt.Sprintf("DATE_TRUNC('week', %s)", dimensionSQL), nil
	case "month":
		return fmt.Sprintf("DATE_TRUNC('month', %s)", dimensionSQL), nil
	case "year":
		return fmt.Sprintf("DATE_TRUNC('year', %s)", dimensionSQL), nil
	default:
		return "", fmt.Errorf("unsupported granularity: %s", timeDim.Granularity)
	}
}

// buildFilterCondition builds a WHERE condition based on the filter operator
func (sb *SQLBuilder) buildFilterCondition(memberSQL string, filter Filter) (squirrel.Sqlizer, error) {
	switch filter.Operator {
	case "equals":
		if len(filter.Values) == 1 {
			return squirrel.Eq{memberSQL: filter.Values[0]}, nil
		}
		return squirrel.Eq{memberSQL: filter.Values}, nil
	case "notEquals":
		if len(filter.Values) == 1 {
			return squirrel.NotEq{memberSQL: filter.Values[0]}, nil
		}
		return squirrel.NotEq{memberSQL: filter.Values}, nil
	case "contains", "notContains", "startsWith", "notStartsWith", "endsWith", "notEndsWith":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("%s operator requires exactly one value", filter.Operator)
		}
		return likeCondition(memberSQL, filter.Operator, filter.Values[0]), nil
	case "gt":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("gt operator requires exactly one value")
		}
		return squirrel.Gt{memberSQL: filter.Values[0]}, nil
	case "gte":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("gte operator requires exactly one value")
		}
		return squirrel.GtOrEq{memberSQL: filter.Values[0]}, nil
	case "lt":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("lt operator requires exactly one value")
		}
		return squirrel.Lt{memberSQL: filter.Values[0]}, nil
	case "lte":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("lte operator requires exactly one value")
		}
		return squirrel.LtOrEq{memberSQL: filter.Values[0]}, nil
	case "in":
		return squirrel.Eq{memberSQL: filter.Values}, nil
	case "notIn":
		return squirrel.NotEq{memberSQL: filter.Values}, nil
	case "set":
		// Check if field is not null
		return squirrel.NotEq{memberSQL: nil}, nil
	case "notSet":
		// Check if field is null
		return squirrel.Eq{memberSQL: nil}, nil
	case "inDateRange":
		if len(filter.Values) != 2 {
			return nil, fmt.Errorf("inDateRange operator requires exactly two values")
		}
		return squirrel.And{
			squirrel.GtOrEq{memberSQL: filter.Values[0]},
			squirrel.LtOrEq{memberSQL: filter.Values[1]},
		}, nil
	case "notInDateRange":
		if len(filter.Values) != 2 {
			return nil, fmt.Errorf("notInDateRange operator requires exactly two values")
		}
		return squirrel.Or{
			squirrel.Lt{memberSQL: filter.Values[0]},
			squirrel.Gt{memberSQL: filter.Values[1]},
		}, nil
	case "beforeDate":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("beforeDate operator requires exactly one value")
		}
		return squirrel.Lt{memberSQL: filter.Values[0]}, nil
	case "afterDate":
		if len(filter.Values) != 1 {
			return nil, fmt.Errorf("afterDate operator requires exactly one value")
		}
		return squirrel.Gt{memberSQL: filter.Values[0]}, nil
	default:
		return nil, fmt.Errorf("unsupported operator: %s", filter.Operator)
	}
}

// likeEscaper escapes the LIKE wildcards of a value, so that it matches as written: a _ or a
// % in a path or a campaign name would otherwise match any character.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// likeCondition builds the LIKE of a text operator on an escaped value. The ESCAPE clause
// names the escape character rather than relying on PostgreSQL's default.
func likeCondition(memberSQL, operator, value string) squirrel.Sqlizer {
	escaped := likeEscaper.Replace(value)
	var pattern, keyword string
	switch operator {
	case "contains", "notContains":
		pattern = "%" + escaped + "%"
	case "startsWith", "notStartsWith":
		pattern = escaped + "%"
	default: // endsWith, notEndsWith
		pattern = "%" + escaped
	}
	keyword = "LIKE"
	if strings.HasPrefix(operator, "not") {
		keyword = "NOT LIKE"
	}
	return squirrel.Expr(fmt.Sprintf(`%s %s ? ESCAPE '\'`, memberSQL, keyword), pattern)
}

// buildMeasureSQL wraps the SQL expression with the appropriate aggregate function based on measure type
// and applies any measure filters using PostgreSQL FILTER clause
func (sb *SQLBuilder) buildMeasureSQL(measureType, sql string, filters []MeasureFilter) string {
	// If the SQL already contains an aggregate function (has parentheses and common functions),
	// return it as-is to support complex expressions, but still apply filters if present
	upperSQL := strings.ToUpper(sql)
	if strings.Contains(upperSQL, "COUNT(") ||
		strings.Contains(upperSQL, "SUM(") ||
		strings.Contains(upperSQL, "AVG(") ||
		strings.Contains(upperSQL, "MIN(") ||
		strings.Contains(upperSQL, "MAX(") ||
		strings.Contains(upperSQL, "FILTER") {
		return sb.applyMeasureFilters(sql, filters)
	}

	// Apply Cube.js-style automatic wrapping based on measure type
	var baseSQL string
	switch measureType {
	case "count":
		// For count, if it's just a column name, wrap with COUNT()
		if !strings.Contains(upperSQL, "COUNT") {
			baseSQL = fmt.Sprintf("COUNT(%s)", sql)
		} else {
			baseSQL = sql
		}
	case "sum":
		baseSQL = fmt.Sprintf("SUM(%s)", sql)
	case "avg":
		baseSQL = fmt.Sprintf("AVG(%s)", sql)
	case "min":
		baseSQL = fmt.Sprintf("MIN(%s)", sql)
	case "max":
		baseSQL = fmt.Sprintf("MAX(%s)", sql)
	case "count_distinct":
		baseSQL = fmt.Sprintf("COUNT(DISTINCT %s)", sql)
	case "count_distinct_approx":
		// Use HyperLogLog approximation if available, fallback to COUNT DISTINCT
		baseSQL = fmt.Sprintf("COUNT(DISTINCT %s)", sql)
	default:
		// For unknown types or custom expressions, return as-is
		baseSQL = sql
	}

	// Apply filters to the base SQL
	return sb.applyMeasureFilters(baseSQL, filters)
}

// applyMeasureFilters applies measure filters using PostgreSQL FILTER clause
func (sb *SQLBuilder) applyMeasureFilters(baseSQL string, filters []MeasureFilter) string {
	if len(filters) == 0 {
		return baseSQL
	}

	// Collect all filter conditions
	var conditions []string
	for _, filter := range filters {
		if filter.SQL != "" {
			conditions = append(conditions, filter.SQL)
		}
	}

	if len(conditions) == 0 {
		return baseSQL
	}

	// Join conditions with AND and apply FILTER clause
	filterClause := strings.Join(conditions, " AND ")
	return fmt.Sprintf("%s FILTER (WHERE %s)", baseSQL, filterClause)
}

// sanitizeTimezone validates and sanitizes timezone strings to prevent SQL injection
func (sb *SQLBuilder) sanitizeTimezone(timezone string) string {
	return SanitizeTimezone(timezone)
}

// SanitizeTimezone returns a timezone name safe to inline into SQL, or an
// empty string when the input cannot be one. Schemas that bake a timezone into
// a dimension expression need the same guarantee the builder does.
func SanitizeTimezone(timezone string) string {
	// Remove any quotes or dangerous characters
	timezone = strings.ReplaceAll(timezone, "'", "")
	timezone = strings.ReplaceAll(timezone, "\"", "")
	timezone = strings.ReplaceAll(timezone, ";", "")
	timezone = strings.ReplaceAll(timezone, "--", "")
	timezone = strings.ReplaceAll(timezone, "/*", "")
	timezone = strings.ReplaceAll(timezone, "*/", "")

	// Trim whitespace
	timezone = strings.TrimSpace(timezone)

	// Check if it's empty after sanitization
	if timezone == "" {
		return ""
	}

	// Validate against common timezone patterns
	// Allow only alphanumeric, underscore, slash, plus, minus, and colon
	for _, char := range timezone {
		if (char < 'A' || char > 'Z') &&
			(char < 'a' || char > 'z') &&
			(char < '0' || char > '9') &&
			char != '_' && char != '/' && char != '+' && char != '-' && char != ':' {
			return "" // Invalid character found
		}
	}

	// Additional length check
	if len(timezone) > 50 {
		return "" // Too long, likely malicious
	}

	return timezone
}

// ScanRows scans SQL rows and converts them to a slice of maps for JSON serialization
func ScanRows(rows *sql.Rows) ([]map[string]interface{}, error) {
	// Get column information
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Parse results.
	//
	// Non-nil from the start: a nil slice marshals to JSON null, and every
	// consumer's type declares `data` as an array. A breakdown that matched
	// nothing would hand the client null where it promised a list, and the
	// client would call .map on it — an empty workspace, or any filter
	// combination with no matches, is enough to crash the page.
	data := make([]map[string]interface{}, 0)
	for rows.Next() {
		// Create a slice of interface{} to hold the values
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// Scan the row
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Convert to map
		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			// Convert []byte to string for better JSON serialization
			if b, ok := val.([]byte); ok {
				val = string(b)
			}
			// Convert time.Time to string for date dimensions.
			//
			// Normalized to UTC first. A bucket built without AT TIME ZONE
			// stays a timestamptz, which the driver hands back in the database
			// session's zone — so on a server set to anything but UTC the same
			// instant would serialize with an offset while the gap filler
			// generates its keys in UTC. Every real bucket would then miss its
			// key and be replaced by a zero-filled one, flattening the chart.
			if t, ok := val.(time.Time); ok {
				val = t.UTC().Format(time.RFC3339)
			}
			row[col] = val
		}
		data = append(data, row)
	}

	// Check for iteration errors
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during result iteration: %w", err)
	}

	return data, nil
}

// ProcessRows scans SQL rows and fills time series gaps if the query has time dimensions.
// measureTypes maps each measure to its type (SchemaDefinition.MeasureTypes), which says what
// a filled period holds for it; dimensionTypes maps each dimension to its type
// (SchemaDefinition.DimensionTypes), which says how a sort compares its values.
//
// A gap-filled query (a time dimension with a dateRange) is read whole and sorted, offset and
// limited here, after the filling: cut in SQL, a real row would come back as a filled zero
// and the filled rows would land outside the requested order (executionQuery). Any other
// query got its order, offset and limit in SQL. A query without a limit is refused when it
// returns more than MaxRows rows.
func ProcessRows(rows *sql.Rows, query Query, measureTypes, dimensionTypes map[string]string) ([]map[string]interface{}, error) {
	// First scan the rows normally
	data, err := ScanRows(rows)
	if err != nil {
		return nil, err
	}

	if (query.Limit == nil || query.isGapFilled()) && len(data) > MaxRows {
		if query.isGapFilled() && query.Limit != nil && len(query.Dimensions) > 0 {
			return nil, invalid(ErrInvalidQuery, "the series of the groups that hold the first %d rows return more than %d rows: lower the limit or the offset, shorten the date range or use a coarser granularity",
				*query.Limit+query.GetOffset(), MaxRows)
		}
		return nil, invalid(ErrInvalidQuery, "the query returns more than %d rows: set a limit, narrow its filters or group by fewer dimensions", MaxRows)
	}

	// Check if this is a time series query that needs gap filling
	if !query.HasTimeDimensions() {
		// A totals query with nothing to total still has an answer — zero — and
		// a KPI tile would otherwise have to render a blank. A *breakdown* with
		// nothing to break down does not: inventing a row there puts a phantom
		// entry with an empty dimension at the top of every table, which reads
		// as real data and hides the widget's own empty state.
		if len(data) == 0 && len(query.Dimensions) == 0 {
			return generateZeroValueRow(query), nil
		}
		return data, nil
	}

	// For time series queries, fill gaps with zero values
	filled, err := fillTimeSeriesGaps(data, query, measureTypes)
	if err != nil {
		return nil, err
	}
	if !query.isGapFilled() {
		return filled, nil
	}
	sortRows(filled, query, dimensionTypes)
	return pageRows(filled, query.Offset, query.Limit), nil
}

// pageRows applies an offset and a limit to rows already in order.
func pageRows(data []map[string]interface{}, offset, limit *int) []map[string]interface{} {
	if offset != nil {
		if *offset >= len(data) {
			return data[:0]
		}
		data = data[*offset:]
	}
	if limit != nil && *limit < len(data) {
		data = data[:*limit]
	}
	return data
}

// sortRows sorts gap-filled rows by the query's order, as the SQL would have: a measure puts
// its nulls last both ways, as BuildSQL's NULLS LAST does, and a dimension or the time bucket
// puts them where PostgreSQL does by default (last ascending, first descending). The sort is
// stable, so rows equal on every key keep the filler's order: by period, then by combination.
// A measure and a number dimension compare as numbers, even when the database returns them
// as text (NUMERIC). Other text compares by the Unicode collation (collate, language.Und),
// which orders 'apple', 'Banana', 'éclair' as a database with a linguistic collation such as
// en_US.UTF-8 does, where a byte comparison would put 'Banana' first and 'éclair' after 'z'.
func sortRows(data []map[string]interface{}, query Query, dimensionTypes map[string]string) {
	type key struct {
		column  string
		desc    bool
		measure bool
		numeric bool
	}
	var keys []key
	for _, field := range query.OrderKeys() {
		k := key{column: field, desc: strings.EqualFold(query.Order[field], "desc")}
		switch {
		case slices.Contains(query.Measures, field):
			k.measure = true
			k.numeric = true
		case slices.Contains(query.Dimensions, field):
			k.numeric = dimensionTypes[field] == "number"
		default:
			column, ok := timeDimensionColumn(query, field)
			if !ok {
				continue
			}
			k.column = column
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return
	}
	// A Collator is not safe for concurrent use: one per sort.
	collator := collate.New(language.Und)
	slices.SortStableFunc(data, func(a, b map[string]interface{}) int {
		for _, k := range keys {
			va, vb := a[k.column], b[k.column]
			if va == nil || vb == nil {
				if va == nil && vb == nil {
					continue
				}
				// nil sorts last, unless a descending dimension puts it first.
				nilFirst := k.desc && !k.measure
				if (va == nil) == nilFirst {
					return -1
				}
				return 1
			}
			c := compareCells(va, vb, k.numeric, collator)
			if k.desc {
				c = -c
			}
			if c != 0 {
				return c
			}
		}
		return 0
	})
}

// compareCells compares two non-nil cells. Numbers compare as numbers, whatever their Go type;
// with numericText, text that reads as a number (a NUMERIC) compares as one too. Other text
// compares with the collator, and as bytes when the collator finds two strings equal.
func compareCells(a, b interface{}, numericText bool, collator *collate.Collator) int {
	fa, okA := cellNumber(a, numericText)
	fb, okB := cellNumber(b, numericText)
	if okA && okB {
		switch {
		case fa < fb:
			return -1
		case fa > fb:
			return 1
		}
		return 0
	}
	if ba, ok := a.(bool); ok {
		if bb, ok := b.(bool); ok {
			switch {
			case ba == bb:
				return 0
			case !ba:
				return -1
			}
			return 1
		}
	}
	sa, sb := fmt.Sprint(a), fmt.Sprint(b)
	if c := collator.CompareString(sa, sb); c != 0 {
		return c
	}
	return strings.Compare(sa, sb)
}

// cellNumber reads a cell as a number.
func cellNumber(v interface{}, numericText bool) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case string:
		if numericText {
			if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

// emptyMeasureValue is what a measure of this type is over no row: 0 for a count or a sum,
// nil for a max, a min or an average, which have no value then. An unknown type gets 0.
func emptyMeasureValue(measureType string) interface{} {
	switch measureType {
	case "max", "min", "avg":
		return nil
	default:
		return 0
	}
}

// generateZeroValueRow creates a single row with zero values for all measures
func generateZeroValueRow(query Query) []map[string]interface{} {
	row := make(map[string]interface{})

	// Set all measures to zero
	for _, measure := range query.Measures {
		row[measure] = 0
	}

	// Add dimensions with empty/zero values
	for _, dimension := range query.Dimensions {
		row[dimension] = ""
	}

	return []map[string]interface{}{row}
}

// fillTimeSeriesGaps gives every period of the date range a row for each combination of
// dimension values seen in the data (or one row with empty dimensions when there is none). A
// row is keyed by its period and its dimension values, so a query with a dimension keeps every
// row of a period. A filled row holds each measure's empty value (emptyMeasureValue).
// Periods times combinations is bounded by maxGapFilledRows: past it, the data is returned
// unfilled, as a dimension with many values over a long range would otherwise make millions
// of rows.
func fillTimeSeriesGaps(data []map[string]interface{}, query Query, measureTypes map[string]string) ([]map[string]interface{}, error) {
	if len(query.TimeDimensions) == 0 {
		return data, nil
	}

	// A query has at most one time dimension (DefaultValidate).
	timeDim := query.TimeDimensions[0]

	// If no date range specified, return data as-is
	if timeDim.DateRange == nil {
		return data, nil
	}

	// Create time dimension column name
	timeDimColumn := fmt.Sprintf("%s_%s", timeDim.Dimension, timeDim.Granularity)

	// Determine time range strategy based on granularity and data
	var timeRange []string
	timezone := query.GetDefaultTimezone()
	startTime, endTime, sameDay, err := gapFillRange(timeDim, timezone)
	if err != nil {
		return data, err
	}

	// Counted before anything is allocated: a range of centuries by the hour would otherwise
	// build millions of keys before any bound could refuse it. DefaultValidate refuses it
	// first; this keeps the filler safe on its own.
	if countPeriods(startTime, endTime, timeDim.Granularity) > maxGapFilledRows {
		return data, invalid(ErrInvalidQuery, "dateRange of %q holds more than %d periods of a %s: shorten the range or use a coarser granularity",
			timeDim.Dimension, maxGapFilledRows, timeDim.Granularity)
	}

	// For hour granularity with same-day range and existing data, use data-driven approach
	if timeDim.Granularity == "hour" && sameDay && len(data) > 0 {
		timeRange = generateTimeRangeFromData(data, timeDimColumn, timeDim)
	} else {
		// For other cases, use the full date range
		timeRange = generateTimeRange(startTime, endTime, timeDim.Granularity)
	}

	if len(timeRange) == 0 {
		return data, nil
	}

	dimensionKey := func(row map[string]interface{}) string {
		parts := make([]string, len(query.Dimensions))
		for i, dimension := range query.Dimensions {
			parts[i] = fmt.Sprintf("%T:%v", row[dimension], row[dimension])
		}
		return strings.Join(parts, "\x00")
	}

	// The existing rows by period and dimension values, and the combinations of dimension
	// values in the order the data first shows them.
	existingData := make(map[string]map[string]interface{})
	var combinations []map[string]interface{}
	seen := make(map[string]bool)
	for _, row := range data {
		timeStr, ok := row[timeDimColumn].(string)
		if !ok {
			continue
		}
		key := dimensionKey(row)
		existingData[timeStr+"\x00"+key] = row
		if !seen[key] {
			seen[key] = true
			combination := make(map[string]interface{}, len(query.Dimensions))
			for _, dimension := range query.Dimensions {
				combination[dimension] = row[dimension]
			}
			combinations = append(combinations, combination)
		}
	}
	if len(combinations) == 0 {
		empty := make(map[string]interface{}, len(query.Dimensions))
		for _, dimension := range query.Dimensions {
			empty[dimension] = ""
		}
		combinations = append(combinations, empty)
	}

	if len(timeRange)*len(combinations) > maxGapFilledRows {
		return data, nil
	}

	result := make([]map[string]interface{}, 0, len(timeRange)*len(combinations))
	for _, timeStr := range timeRange {
		for _, combination := range combinations {
			if existingRow, exists := existingData[timeStr+"\x00"+dimensionKey(combination)]; exists {
				result = append(result, existingRow)
				continue
			}
			filled := make(map[string]interface{}, len(query.Measures)+len(combination)+1)
			filled[timeDimColumn] = timeStr
			for _, measure := range query.Measures {
				filled[measure] = emptyMeasureValue(measureTypes[measure])
			}
			for dimension, value := range combination {
				filled[dimension] = value
			}
			result = append(result, filled)
		}
	}

	return result, nil
}

// gapFillRange returns the bounds of a time dimension's range as the gap filler reads them
// (gapFillBound), and whether it is a single bare day. A bare end date names a whole day at
// every granularity, as in the SQL: it ends on the last millisecond of that day, so hourly
// periods run to its last hour, and a range that starts during that day does not end before
// it starts.
func gapFillRange(timeDim TimeDimension, timezone string) (start, end time.Time, sameDay bool, err error) {
	start, err = gapFillBound(timeDim.DateRange[0], timezone)
	if err != nil {
		return start, end, false, fmt.Errorf("failed to parse start date: %w", err)
	}
	end, err = gapFillBound(timeDim.DateRange[1], timezone)
	if err != nil {
		return start, end, false, fmt.Errorf("failed to parse end date: %w", err)
	}
	sameDay = isDateOnlyBound(timeDim.DateRange[0]) && isDateOnlyBound(timeDim.DateRange[1]) && start.Equal(end)
	if isDateOnlyBound(timeDim.DateRange[1]) && !sameDay {
		end = end.AddDate(0, 0, 1).Add(-time.Millisecond)
	}
	return start, end, sameDay, nil
}

// gapFillBound parses a dateRange bound like the SQL does (parseDateRangeBound) and returns
// its wall clock in the query's timezone, labeled UTC. The buckets come back that way: they are
// truncated in the query's timezone and serialized as UTC.
func gapFillBound(value string, timezone string) (time.Time, error) {
	parsed, err := parseDateRangeBound(value, timezone)
	if err != nil {
		return time.Time{}, err
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}
	local := parsed.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), time.UTC), nil
}

// maxGapFilledRows bounds the rows the gap filling may return, and so the periods of a range.
const maxGapFilledRows = MaxRows

// truncatePeriod returns the start of the period of t, in UTC. A week starts on Monday.
func truncatePeriod(t time.Time, granularity string) time.Time {
	t = t.UTC()
	switch granularity {
	case "hour":
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC)
	case "week":
		weekday := t.Weekday()
		if weekday == time.Sunday {
			weekday = 7
		}
		t = t.AddDate(0, 0, -int(weekday-time.Monday))
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	case "month":
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	case "year":
		return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	default: // day
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
}

// countPeriods returns how many periods generateTimeRange makes from start to end, computed
// from the bounds alone. Seconds since the epoch rather than time.Duration, which saturates
// past 292 years and would undercount.
func countPeriods(start, end time.Time, granularity string) int64 {
	start, end = truncatePeriod(start, granularity), truncatePeriod(end, granularity)
	if end.Before(start) {
		return 0
	}
	seconds := end.Unix() - start.Unix()
	switch granularity {
	case "hour":
		return seconds/3600 + 1
	case "day":
		return seconds/86400 + 1
	case "week":
		return seconds/(7*86400) + 1
	case "month":
		return int64(end.Year()-start.Year())*12 + int64(end.Month()-start.Month()) + 1
	case "year":
		return int64(end.Year()-start.Year()) + 1
	}
	return 0
}

// generateTimeRange generates a slice of time strings based on start, end, and granularity.
// Callers bound the count first (countPeriods).
func generateTimeRange(start, end time.Time, granularity string) []string {
	switch granularity {
	case "hour", "day", "week", "month", "year":
	default:
		return nil
	}
	current := truncatePeriod(start, granularity)
	end = truncatePeriod(end, granularity)
	times := make([]string, 0, countPeriods(current, end, granularity))
	for !current.After(end) {
		times = append(times, current.Format(time.RFC3339))
		switch granularity {
		case "hour":
			current = current.Add(time.Hour)
		case "day":
			current = current.AddDate(0, 0, 1)
		case "week":
			current = current.AddDate(0, 0, 7)
		case "month":
			current = current.AddDate(0, 1, 0)
		case "year":
			current = current.AddDate(1, 0, 0)
		}
	}
	return times
}

// generateTimeRangeFromData generates a time range based on existing data points
func generateTimeRangeFromData(data []map[string]interface{}, timeDimColumn string, timeDim TimeDimension) []string {
	if len(data) == 0 {
		return nil
	}

	// Extract all time values from existing data
	var existingTimes []time.Time
	for _, row := range data {
		if timeVal, exists := row[timeDimColumn]; exists {
			if timeStr, ok := timeVal.(string); ok {
				if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
					existingTimes = append(existingTimes, t)
				}
			}
		}
	}

	if len(existingTimes) == 0 {
		return nil
	}

	// Find min and max times
	minTime := existingTimes[0]
	maxTime := existingTimes[0]
	for _, t := range existingTimes {
		if t.Before(minTime) {
			minTime = t
		}
		if t.After(maxTime) {
			maxTime = t
		}
	}

	// Generate time range from min to max
	return generateTimeRange(minTime, maxTime, timeDim.Granularity)
}

// ToSQL is a convenience method on Query to build SQL using the default builder
func (q *Query) ToSQL(schema SchemaDefinition) (string, []interface{}, error) {
	builder := NewSQLBuilder()
	return builder.BuildSQL(*q, schema)
}
