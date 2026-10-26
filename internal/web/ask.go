package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"kb-rag/internal/query"
	"kb-rag/internal/store"
)

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	question, topK, ok := decodeAskRequest(w, r)
	if !ok {
		return
	}

	start := time.Now()
	res, err := s.query.WithTenant(p.TenantID).AskWithTopK(r.Context(), question, topK)
	citations := citationsFromHits(res.Hits)
	s.auditAsk(r, p, question, topK, citations, err == nil, false, time.Since(start))
	if err != nil {
		if len(citations) > 0 {
			writeJSON(w, http.StatusOK, askResponse{
				Generated: false,
				Warning:   err.Error(),
				Citations: citations,
			})
			return
		}
		if isAskDomainError(err) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		s.logger.Error("问答失败", "error", err)
		writeError(w, http.StatusBadGateway, "问答服务暂时不可用")
		return
	}
	writeJSON(w, http.StatusOK, askResponse{
		Answer:    res.Answer,
		Generated: true,
		Citations: citations,
	})
}

func (s *Server) handleAskStream(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	question, topK, ok := decodeAskRequest(w, r)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "当前连接不支持流式响应")
		return
	}

	start := time.Now()
	svc := s.query.WithTenant(p.TenantID)
	hits, err := svc.Retrieve(r.Context(), question, topK)
	citations := citationsFromHits(hits)
	if err != nil {
		s.auditAsk(r, p, question, topK, citations, false, true, time.Since(start))
		if isAskDomainError(err) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		s.logger.Error("检索失败", "error", err)
		writeError(w, http.StatusBadGateway, "问答服务暂时不可用")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if err := writeSSE(w, flusher, "citations", citations); err != nil {
		return
	}

	answer, genErr := svc.GenerateStream(r.Context(), question, hits, func(delta string) error {
		return writeSSE(w, flusher, "delta", map[string]string{"text": delta})
	})
	s.auditAsk(r, p, question, topK, citations, genErr == nil, true, time.Since(start))
	if genErr != nil {
		if r.Context().Err() == nil {
			_ = writeSSE(w, flusher, "error", map[string]string{"error": genErr.Error()})
		}
		return
	}
	_ = writeSSE(w, flusher, "done", map[string]any{"answer": answer, "generated": true})
}

func decodeAskRequest(w http.ResponseWriter, r *http.Request) (string, int, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req askRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return "", 0, false
	}
	question := strings.TrimSpace(req.Question)
	if question == "" {
		writeError(w, http.StatusBadRequest, "问题不能为空")
		return "", 0, false
	}
	topK := req.TopK
	if topK == 0 {
		topK = 8
	}
	if topK < 1 || topK > 20 {
		writeError(w, http.StatusBadRequest, "top_k 必须在 1 到 20 之间")
		return "", 0, false
	}
	return question, topK, true
}

func isAskDomainError(err error) bool {
	return errors.Is(err, query.ErrEmptyStore) || errors.Is(err, query.ErrNoResults) || errors.Is(err, query.ErrModelMismatch)
}

func (s *Server) auditAsk(r *http.Request, p store.Principal, question string, topK int, citations []citationDTO, generated, stream bool, elapsed time.Duration) {
	s.audit(r, p, p.TenantID, "ask", "", map[string]any{
		"question":    truncateRunes(question, 4000),
		"top_k":       topK,
		"citations":   len(citations),
		"generated":   generated,
		"duration_ms": elapsed.Milliseconds(),
		"stream":      stream,
	})
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func citationsFromHits(hits []store.Hit) []citationDTO {
	out := make([]citationDTO, 0, len(hits))
	for i, hit := range hits {
		out = append(out, citationDTO{
			N:       i + 1,
			Source:  filepath.Base(hit.Path),
			Heading: hit.Heading,
			Text:    hit.Text,
			Score:   hit.Score,
		})
	}
	return out
}

func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
