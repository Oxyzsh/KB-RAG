package web

type embeddingDTO struct {
	Model     string `json:"model"`
	Dim       int    `json:"dim"`
	ChunkSize int    `json:"chunk_size"`
}

type statsDTO struct {
	Documents       int          `json:"documents"`
	DocumentsFailed int          `json:"documents_failed"`
	Chunks          int          `json:"chunks"`
	SizeBytes       int64        `json:"size_bytes"`
	Embedding       embeddingDTO `json:"embedding"`
}

type documentDTO struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Chunks    int    `json:"chunks"`
	Source    string `json:"source"`
	Error     string `json:"error"`
	UpdatedAt string `json:"updated_at"`
}

type listDocumentsResponse struct {
	Documents []documentDTO `json:"documents"`
}

type citationDTO struct {
	N       int     `json:"n"`
	Source  string  `json:"source"`
	Heading string  `json:"heading"`
	Text    string  `json:"text"`
	Score   float64 `json:"score"`
}

type askRequest struct {
	Question string `json:"question"`
	TopK     int    `json:"top_k"`
}

type askResponse struct {
	Answer    string        `json:"answer"`
	Generated bool          `json:"generated"`
	Warning   string        `json:"warning"`
	Citations []citationDTO `json:"citations"`
}

type uploadResponse struct {
	Document documentDTO `json:"document"`
	Outcome  string      `json:"outcome"`
}

type errorResponse struct {
	Error string `json:"error"`
}
