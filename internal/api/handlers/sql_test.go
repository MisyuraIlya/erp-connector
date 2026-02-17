package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateReadOnlySQL(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
		code    string
	}{
		{
			name:  "select ok",
			query: "SELECT * FROM dbo.Stock WHERE ValueDate >= @dateFrom ORDER BY ValueDate ASC OFFSET @offset ROWS FETCH NEXT @pageSize ROWS ONLY",
		},
		{
			name:  "with cte ok",
			query: "WITH cte AS (SELECT 1 AS x) SELECT x FROM cte",
		},
		{
			name:  "string literal ok",
			query: "SELECT 'insert' AS word",
		},
		{
			name:    "empty query",
			query:   " ",
			wantErr: true,
			code:    "SQL_QUERY_REQUIRED",
		},
		{
			name:    "multi statement rejected",
			query:   "SELECT 1; SELECT 2",
			wantErr: true,
			code:    "SQL_MULTI_STATEMENT",
		},
		{
			name:    "comments rejected",
			query:   "SELECT 1 -- comment",
			wantErr: true,
			code:    "SQL_COMMENTS_NOT_ALLOWED",
		},
		{
			name:    "update rejected",
			query:   "UPDATE dbo.Stock SET ValueDate = GETDATE()",
			wantErr: true,
			code:    "SQL_NOT_READ_ONLY",
		},
		{
			name:    "exec rejected",
			query:   "EXEC sp_who",
			wantErr: true,
			code:    "SQL_NOT_READ_ONLY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReadOnlySQL(tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				var vErr sqlValidationError
				if !errors.As(err, &vErr) {
					t.Fatalf("expected sqlValidationError, got %T", err)
				}
				if vErr.code != tt.code {
					t.Fatalf("expected code %q, got %q", tt.code, vErr.code)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestDetectIntegerParams(t *testing.T) {
	query := `
SELECT TOP (@top) *
FROM dbo.Stock
ORDER BY ValueDate DESC
OFFSET @offset ROWS FETCH NEXT @pageSize ROWS ONLY
`

	hints := detectIntegerParams(query)
	for _, name := range []string{"top", "offset", "pagesize"} {
		if _, ok := hints[name]; !ok {
			t.Fatalf("expected integer hint for %q", name)
		}
	}
}

func TestBuildNamedArgs_CoercesHintedStringIntegers(t *testing.T) {
	params := map[string]any{
		"dateFrom": "2026-01-01",
		"offset":   "0",
		"pageSize": "100",
		"search":   "00123",
	}
	hints := detectIntegerParams("SELECT * FROM t ORDER BY id OFFSET @offset ROWS FETCH NEXT @pageSize ROWS ONLY")

	args := buildNamedArgs(params, hints)
	if len(args) != len(params) {
		t.Fatalf("expected %d args, got %d", len(params), len(args))
	}

	byName := make(map[string]any, len(args))
	for _, arg := range args {
		na, ok := arg.(sql.NamedArg)
		if !ok {
			t.Fatalf("expected sql.NamedArg, got %T", arg)
		}
		byName[na.Name] = na.Value
	}

	if got, ok := byName["offset"].(int64); !ok || got != 0 {
		t.Fatalf("expected offset int64(0), got %T(%v)", byName["offset"], byName["offset"])
	}
	if got, ok := byName["pageSize"].(int64); !ok || got != 100 {
		t.Fatalf("expected pageSize int64(100), got %T(%v)", byName["pageSize"], byName["pageSize"])
	}
	if got, ok := byName["search"].(string); !ok || got != "00123" {
		t.Fatalf("expected search string %q, got %T(%v)", "00123", byName["search"], byName["search"])
	}
}

func TestNormalizeParamValue(t *testing.T) {
	tests := []struct {
		name      string
		paramName string
		value     any
		hints     map[string]struct{}
		want      any
	}{
		{
			name:      "float integer",
			paramName: "x",
			value:     float64(42),
			want:      int64(42),
		},
		{
			name:      "float decimal",
			paramName: "x",
			value:     float64(3.14),
			want:      float64(3.14),
		},
		{
			name:      "json number integer",
			paramName: "x",
			value:     json.Number("12"),
			want:      int64(12),
		},
		{
			name:      "json number decimal",
			paramName: "x",
			value:     json.Number("2.5"),
			want:      float64(2.5),
		},
		{
			name:      "string integer with hint",
			paramName: "offset",
			value:     "10",
			hints: map[string]struct{}{
				"offset": {},
			},
			want: int64(10),
		},
		{
			name:      "string integer without hint",
			paramName: "search",
			value:     "10",
			want:      "10",
		},
		{
			name:      "string non-integer with hint",
			paramName: "offset",
			value:     "10a",
			hints: map[string]struct{}{
				"offset": {},
			},
			want: "10a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeParamValue(tt.paramName, tt.value, tt.hints)
			switch want := tt.want.(type) {
			case float64:
				gotF, ok := got.(float64)
				if !ok || gotF != want {
					t.Fatalf("expected float64(%v), got %T(%v)", want, got, got)
				}
			default:
				if got != want {
					t.Fatalf("expected %T(%v), got %T(%v)", want, want, got, got)
				}
			}
		})
	}
}
