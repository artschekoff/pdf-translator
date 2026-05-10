package queue

import "encoding/json"

const (
	QueueOCR    = "ocr"
	QueueAI     = "ai"
	QueueExport = "export"

	TaskOCRProcess     = "ocr:process"
	TaskAIBlock        = "ai:block"
	TaskAIDocument     = "ai:document"
	TaskExportDocument = "export:document"
)

type OcrPayload struct {
	DocumentID string `json:"document_id"`
	UserID     string `json:"user_id"`
}

type AIBlockPayload struct {
	DocumentID  string `json:"document_id"`
	BlockID     string `json:"block_id"`
	Instruction string `json:"instruction"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
}

type ExportPayload struct {
	DocumentID  string `json:"document_id"`
	Format      string `json:"format"`
	Theme       string `json:"theme"`
	CallbackURL string `json:"callback_url,omitempty"`
}

func Encode(v any) ([]byte, error) { return json.Marshal(v) }
func Decode(b []byte, v any) error  { return json.Unmarshal(b, v) }
