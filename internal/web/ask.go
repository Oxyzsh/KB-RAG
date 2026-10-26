package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"kb-rag/internal/query"
	"kb-rag/internal/store"
)

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req askRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	question := strings.TrimSpace(req.Question)
	if question == "" {
		writeError(w, http.StatusBadRequest, "问题不能为空")
		return
	}
	topK := req.TopK
	if topK == 0 {
		topK = 8
	}
	if topK < 1 || topK > 20 {
		writeError(w, http.StatusBadRequest, "top_k 必须在 1 到 20 之间")
		return
	}

	start := time.Now()
	res, err := s.query.WithTenant(p.TenantID).AskWithTopK(r.Context(), question, topK)
	citations := citationsFromHits(res.Hits)
	s.audit(r, p, p.TenantID, "ask", "", map[string]any{
		"question":    truncateRunes(question, 4000),
		"top_k":       topK,
		"citations":   len(citations),
		"generated":   err == nil,
		"duration_ms": time.Since(start).Milliseconds(),
	})
	if err != nil {
		if len(citations) > 0 {
			writeJSON(w, http.StatusOK, askResponse{
				Generated: false,
				Warning:   err.Error(),
				Citations: citations,
			})
			return
		}
		if errors.Is(err, query.ErrEmptyStore) || errors.Is(err, query.ErrNoResults) || errors.Is(err, query.ErrModelMismatch) {
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
