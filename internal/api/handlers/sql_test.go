package handlers

import (
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
