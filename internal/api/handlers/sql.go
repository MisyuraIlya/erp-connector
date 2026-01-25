package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"erp-connector/internal/api/dto"
	"erp-connector/internal/api/utils"
)

const (
	sqlMaxBodyBytes = 1 << 20
	sqlMaxRows      = 10000
	sqlTimeout      = 8 * time.Second
)

var (
	errSQLNotReadOnly = errors.New("sql not read-only")
	errSQLInvalid     = errors.New("sql invalid")
	errSQLRowLimit    = errors.New("row limit exceeded")
	errSQLUnsupported = errors.New("sql contains unsupported tokens")
)

type sqlValidationError struct {
	code string
	msg  string
	err  error
}

func (e sqlValidationError) Error() string { return e.msg }

func NewSQLHandler(dbConn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if dbConn == nil {
			utils.WriteError(w, http.StatusServiceUnavailable, "Database connection unavailable", "DB_UNAVAILABLE", nil)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, sqlMaxBodyBytes)
		defer r.Body.Close()

		var req dto.SQLRequest
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&req); err != nil {
			utils.WriteError(w, http.StatusBadRequest, "Invalid JSON body", "INVALID_JSON", nil)
			return
		}
		if err := ensureEOF(dec); err != nil {
			utils.WriteError(w, http.StatusBadRequest, "Invalid JSON body", "INVALID_JSON", nil)
			return
		}

		if err := validateReadOnlySQL(req.Query); err != nil {
			var vErr sqlValidationError
			if errors.As(err, &vErr) {
				utils.WriteError(w, http.StatusBadRequest, vErr.msg, vErr.code, nil)
				return
			}
			utils.WriteError(w, http.StatusBadRequest, "Query rejected", "SQL_NOT_READ_ONLY", nil)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), sqlTimeout)
		defer cancel()

		args := buildNamedArgs(req.Params)

		rows, err := dbConn.QueryContext(ctx, req.Query, args...)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				utils.WriteError(w, http.StatusGatewayTimeout, "Query timeout", "SQL_TIMEOUT", nil)
				return
			}
			utils.WriteError(w, http.StatusInternalServerError, "Query execution failed", "DB_ERROR", nil)
			return
		}
		defer rows.Close()

		recordsets, err := collectRecordsets(rows, sqlMaxRows)
		if err != nil {
			if errors.Is(err, errSQLRowLimit) {
				utils.WriteError(w, http.StatusRequestEntityTooLarge, "Row limit exceeded", "SQL_ROW_LIMIT", nil)
				return
			}
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				utils.WriteError(w, http.StatusGatewayTimeout, "Query timeout", "SQL_TIMEOUT", nil)
				return
			}
			utils.WriteError(w, http.StatusInternalServerError, "Query execution failed", "DB_ERROR", nil)
			return
		}

		rowsOut := make([]map[string]any, 0)
		if len(recordsets) > 0 {
			rowsOut = recordsets[0]
		}

		resp := dto.SQLResponse{
			API:        r.URL.Path,
			Status:     "success",
			RowCount:   len(rowsOut),
			Rows:       rowsOut,
			Recordsets: ensureRecordsets(recordsets),
		}
		utils.WriteJSON(w, http.StatusOK, resp)
	}
}

func ensureEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return errors.New("extra data")
}

func buildNamedArgs(params map[string]any) []any {
	if len(params) == 0 {
		return nil
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	args := make([]any, 0, len(keys))
	for _, key := range keys {
		name := strings.TrimPrefix(key, "@")
		args = append(args, sql.Named(name, normalizeParamValue(params[key])))
	}
	return args
}

func normalizeParamValue(v any) any {
	switch t := v.(type) {
	case float64:
		if math.Trunc(t) == t {
			return int64(t)
		}
		return t
	default:
		return v
	}
}

func validateReadOnlySQL(query string) error {
	q := strings.TrimSpace(query)
	if q == "" {
		return sqlValidationError{code: "SQL_QUERY_REQUIRED", msg: "Query is required", err: errSQLInvalid}
	}

	if strings.Contains(q, ";") {
		return sqlValidationError{code: "SQL_MULTI_STATEMENT", msg: "Multiple statements are not allowed", err: errSQLInvalid}
	}

	stripped := stripStringLiterals(q)
	lower := strings.ToLower(stripped)

	if strings.Contains(lower, "--") || strings.Contains(lower, "/*") || strings.Contains(lower, "*/") {
		return sqlValidationError{code: "SQL_COMMENTS_NOT_ALLOWED", msg: "SQL comments are not allowed", err: errSQLUnsupported}
	}

	if !startsWithSelectOrWith(lower) {
		return sqlValidationError{code: "SQL_NOT_READ_ONLY", msg: "Only SELECT queries are allowed", err: errSQLNotReadOnly}
	}

	for _, re := range disallowedKeywordRegex {
		if re.MatchString(lower) {
			return sqlValidationError{code: "SQL_NOT_READ_ONLY", msg: "Only SELECT queries are allowed", err: errSQLNotReadOnly}
		}
	}

	return nil
}

func startsWithSelectOrWith(lower string) bool {
	trimmed := strings.TrimSpace(lower)
	return strings.HasPrefix(trimmed, "select ") || strings.HasPrefix(trimmed, "select\n") ||
		strings.HasPrefix(trimmed, "with ") || strings.HasPrefix(trimmed, "with\n") ||
		strings.HasPrefix(trimmed, "select\t") || strings.HasPrefix(trimmed, "with\t") ||
		trimmed == "select" || trimmed == "with"
}

var disallowedKeywordRegex = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\binsert\b`),
	regexp.MustCompile(`(?i)\bupdate\b`),
	regexp.MustCompile(`(?i)\bdelete\b`),
	regexp.MustCompile(`(?i)\bmerge\b`),
	regexp.MustCompile(`(?i)\btruncate\b`),
	regexp.MustCompile(`(?i)\bdrop\b`),
	regexp.MustCompile(`(?i)\balter\b`),
	regexp.MustCompile(`(?i)\bcreate\b`),
	regexp.MustCompile(`(?i)\bexec\b`),
	regexp.MustCompile(`(?i)\bexecute\b`),
	regexp.MustCompile(`(?i)\bgrant\b`),
	regexp.MustCompile(`(?i)\brevoke\b`),
}

func stripStringLiterals(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inString := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			if ch == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					i++
					continue
				}
				inString = false
			}
			continue
		}
		if ch == '\'' {
			inString = true
			continue
		}
		b.WriteByte(ch)
	}

	return b.String()
}

func collectRecordsets(rows *sql.Rows, maxRows int) ([][]map[string]any, error) {
	recordsets := make([][]map[string]any, 0, 1)
	total := 0

	for {
		cols, err := rows.Columns()
		if err != nil {
			return nil, err
		}

		set := make([]map[string]any, 0)
		for rows.Next() {
			if maxRows > 0 && total >= maxRows {
				return nil, errSQLRowLimit
			}

			values := make([]any, len(cols))
			scanArgs := make([]any, len(cols))
			for i := range values {
				scanArgs[i] = &values[i]
			}

			if err := rows.Scan(scanArgs...); err != nil {
				return nil, err
			}

			row := make(map[string]any, len(cols))
			for i, col := range cols {
				v := values[i]
				if b, ok := v.([]byte); ok {
					row[col] = string(b)
					continue
				}
				row[col] = v
			}
			set = append(set, row)
			total++
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}

		recordsets = append(recordsets, set)
		if !rows.NextResultSet() {
			break
		}
	}

	return recordsets, nil
}

func ensureRecordsets(recordsets [][]map[string]any) [][]map[string]any {
	if recordsets == nil {
		return make([][]map[string]any, 0)
	}
	return recordsets
}
