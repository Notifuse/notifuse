#!/bin/bash

# Coverage Report Generator for /internal and /pkg directories
# Generates per-file test coverage reports and lists files below threshold

set -e

# Default threshold
THRESHOLD=${1:-80}
COVERAGE_FILE="coverage-internal-pkg.out"
REPORT_FILE="coverage-report.txt"

echo "📊 Generating Test Coverage Report for /internal and /pkg"
echo "========================================================="
echo ""
echo "Threshold: ${THRESHOLD}%"
echo ""

# Change to the script's directory to ensure we're in the right location
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

# Run tests with coverage for internal and pkg directories
echo "🧪 Running tests with coverage..."
go test -race -coverprofile="$COVERAGE_FILE" -covermode=atomic ./internal/... ./pkg/... -v > /dev/null 2>&1 || {
    echo "⚠️  Some tests failed, but continuing with coverage analysis..."
}

if [ ! -f "$COVERAGE_FILE" ]; then
    echo "❌ Error: Coverage file not generated. Make sure tests can run."
    exit 1
fi

# Extract per-file coverage using go tool cover
echo "📈 Analyzing coverage data..."
COVERAGE_DATA=$(go tool cover -func="$COVERAGE_FILE")

# Filter for internal and pkg directories, exclude mocks, and write to temp file
TEMP_FILE=$(mktemp)
echo "$COVERAGE_DATA" | grep -E "(internal|pkg)/" | grep -v "total:" | grep -v "/mocks/" > "$TEMP_FILE" || true

# Check if we have any coverage data
if [ ! -s "$TEMP_FILE" ]; then
    echo "❌ Error: No coverage data found for /internal or /pkg directories"
    rm -f "$TEMP_FILE"
    exit 1
fi

# Create a Python script to process the data (more reliable than complex awk)
PYTHON_SCRIPT=$(mktemp)
cat > "$PYTHON_SCRIPT" << 'PYTHON_EOF'
import sys
import re
from collections import defaultdict

threshold = float(sys.argv[1])
file_data = defaultdict(lambda: {'sum': 0.0, 'count': 0, 'methods': []})

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    
    # Skip mocks
    if '/mocks/' in line:
        continue
    
    # Find percentage (last field ending with %)
    match = re.search(r'(\d+\.?\d*)%', line)
    if not match:
        continue
    
    percentage = float(match.group(1))
    
    # Extract file path and line number (format: file.go:line:)
    file_match = re.match(r'([^:]+):(\d+):', line)
    if not file_match:
        continue
    
    file_path = file_match.group(1)
    line_num = file_match.group(2)
    
    # Extract method name - it's between the line number and percentage
    # Format: file.go:line:\t\t\tMethodName\t\t\tpercentage%
    # Split by tabs and find the non-empty field that's not the percentage
    parts = line.split('\t')
    method_name = ""
    for part in parts:
        part = part.strip()
        if part and not re.match(r'^\d+\.?\d*%?$', part) and ':' not in part:
            # This should be the method name
            method_name = part
            break
    
    # If we didn't find it by tabs, try regex
    if not method_name:
        method_match = re.search(r':\d+:\s+([A-Za-z_][A-Za-z0-9_]*)\s', line)
        if method_match:
            method_name = method_match.group(1)
    
    # Extract relative path (internal/... or pkg/...)
    rel_match = re.search(r'(internal|pkg)/[^:]+', file_path)
    if rel_match:
        rel_path = rel_match.group(0)
        file_data[rel_path]['sum'] += percentage
        file_data[rel_path]['count'] += 1
        # Store method-level data
        if method_name:
            file_data[rel_path]['methods'].append({
                'name': method_name,
                'coverage': percentage,
                'line': line_num
            })

# Calculate averages and output
results = []
method_results = []  # For methods below threshold

for file_path, data in file_data.items():
    if data['count'] > 0:
        avg = data['sum'] / data['count']
        results.append((avg, file_path, data['methods']))
        # Collect methods below threshold
        for method in data['methods']:
            if method['coverage'] < threshold:
                method_results.append({
                    'file': file_path,
                    'method': method['name'],
                    'coverage': method['coverage'],
                    'line': method['line']
                })

# Sort by coverage (lowest first)
results.sort(key=lambda x: x[0])
method_results.sort(key=lambda x: (x['file'], x['coverage']))

# Output file-level data: coverage_percentage\tfile_path
for avg, file_path, methods in results:
    print(f"FILE:{avg:.1f}\t{file_path}")

# Output method-level data: METHOD:coverage\tfile_path\tmethod_name\tline
for method in method_results:
    print(f"METHOD:{method['coverage']:.1f}\t{method['file']}\t{method['method']}\t{method['line']}")
PYTHON_EOF

# Process coverage data with Python
PROCESSED_TEMP=$(mktemp)
python3 "$PYTHON_SCRIPT" "$THRESHOLD" < "$TEMP_FILE" > "$PROCESSED_TEMP"

# Separate file and method data
PER_FILE_TEMP=$(mktemp)
METHODS_TEMP=$(mktemp)
grep "^FILE:" "$PROCESSED_TEMP" | sed 's/^FILE://' > "$PER_FILE_TEMP"
grep "^METHOD:" "$PROCESSED_TEMP" | sed 's/^METHOD://' > "$METHODS_TEMP"

# Count total files
TOTAL_FILES=$(wc -l < "$PER_FILE_TEMP" | tr -d ' ')

# Extract files below threshold
BELOW_THRESHOLD_FILE=$(mktemp)
awk -v threshold="$THRESHOLD" '{if ($1 + 0 < threshold) printf "%.1f%%\t%s\n", $1, $2}' "$PER_FILE_TEMP" > "$BELOW_THRESHOLD_FILE"

# Count files below threshold
BELOW_THRESHOLD_COUNT=$(wc -l < "$BELOW_THRESHOLD_FILE" | tr -d ' ')

# Count methods below threshold
METHODS_BELOW_COUNT=$(wc -l < "$METHODS_TEMP" | tr -d ' ')

# Calculate average coverage
AVG_COVERAGE=$(awk '{sum+=$1; count++} END {if(count>0) printf "%.1f", sum/count; else printf "0"}' "$PER_FILE_TEMP")

# Generate report
{
    echo "Test Coverage Report for /internal and /pkg"
    echo "Generated: $(date)"
    echo "========================================================="
    echo ""
    echo "Summary Statistics:"
    echo "  Total files analyzed: $TOTAL_FILES (mocks excluded)"
    echo "  Files below ${THRESHOLD}%: $BELOW_THRESHOLD_COUNT"
    echo "  Methods below ${THRESHOLD}%: $METHODS_BELOW_COUNT"
    echo "  Average coverage: ${AVG_COVERAGE}%"
    echo ""
    
    if [ "$BELOW_THRESHOLD_COUNT" -gt 0 ]; then
        echo "========================================================="
        echo "Files Below ${THRESHOLD}% Coverage:"
        echo "========================================================="
        echo ""
        cat "$BELOW_THRESHOLD_FILE"
        echo ""
    else
        echo "✅ All files meet the ${THRESHOLD}% coverage threshold!"
        echo ""
    fi
    
    if [ "$METHODS_BELOW_COUNT" -gt 0 ]; then
        echo "========================================================="
        echo "Methods Below ${THRESHOLD}% Coverage:"
        echo "========================================================="
        echo ""
        echo "Format: Coverage% | File | Method | Line"
        echo ""
        awk -F'\t' '{
            coverage = $1
            file = $2
            method = $3
            line = $4
            printf "%-8s %-55s %-35s (line %s)\n", coverage"%", file, method, line
        }' "$METHODS_TEMP"
        echo ""
    fi
    
    echo "========================================================="
    echo "Complete Coverage Report (sorted by coverage, lowest first):"
    echo "========================================================="
    echo ""
    
    # Generate full sorted list
    awk '{printf "%.1f%%\t%s\n", $1, $2}' "$PER_FILE_TEMP"
    
} | tee "$REPORT_FILE"

# Cleanup
rm -f "$TEMP_FILE" "$PER_FILE_TEMP" "$BELOW_THRESHOLD_FILE" "$PYTHON_SCRIPT" "$PROCESSED_TEMP" "$METHODS_TEMP"

echo ""
echo "✅ Coverage report saved to: $REPORT_FILE"
echo "📄 Coverage profile saved to: $COVERAGE_FILE"
echo ""
echo "💡 Tip: Use 'go tool cover -html=$COVERAGE_FILE' to view detailed HTML coverage"
