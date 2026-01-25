package dto

type SQLRequest struct {
	Query  string         `json:"query"`
	Params map[string]any `json:"params,omitempty"`
}

type SQLMeta struct {
	RowCount   int   `json:"rowCount"`
	DurationMs int64 `json:"durationMs"`
}

type SQLResponse struct {
	API        string             `json:"api"`
	Status     string             `json:"status"`
	RowCount   int                `json:"rowCount"`
	Rows       []map[string]any   `json:"rows"`
	Recordsets [][]map[string]any `json:"recordsets"`
}
