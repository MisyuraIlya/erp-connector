package dto

// TODO
type SendOrderRequest struct {
}

type SendOrderMeta struct {
	DurationMs int64 `json:"durationMs"`
}

type SendOrderResponse struct {
	Status       string        `json:"status"`
	WrittenFiles []string      `json:"writtenFiles,omitempty"`
	Meta         SendOrderMeta `json:"meta,omitempty"`
}
