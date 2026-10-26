package web

import (
	"net/http"
	"strconv"
	"time"

	"kb-rag/internal/store"
)

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	q := r.URL.Query()
	page := atoiDefault(q.Get("page"), 1)
	pageSize := atoiDefault(q.Get("page_size"), 50)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	from := q.Get("from")
	to := q.Get("to")
	if from != "" {
		if _, err := time.Parse(time.RFC3339, from); err != nil {
			writeError(w, http.StatusBadRequest, "from 必须是 RFC3339 时间")
			return
		}
	}
	if to != "" {
		if _, err := time.Parse(time.RFC3339, to); err != nil {
			writeError(w, http.StatusBadRequest, "to 必须是 RFC3339 时间")
			return
		}
	}

	filter := store.AuditFilter{
		Action:   q.Get("action"),
		From:     from,
		To:       to,
		Page:     page,
		PageSize: pageSize,
	}
	if q.Get("scope") == "all" {
		if !p.IsSuper {
			writeError(w, http.StatusForbidden, "仅超级管理员可查看全局审计")
			return
		}
	} else {
		tenantID := p.TenantID
		filter.TenantID = &tenantID
	}

	entries, total, err := s.store.ListAudit(r.Context(), filter)
	if err != nil {
		s.logger.Error("读取审计日志失败", "error", err)
		writeError(w, http.StatusInternalServerError, "读取审计日志失败")
		return
	}
	items := make([]auditDTO, 0, len(entries))
	for _, e := range entries {
		items = append(items, auditDTO{
			ID:         e.ID,
			TenantID:   e.TenantID,
			TenantSlug: e.TenantSlug,
			UserID:     e.UserID,
			Username:   e.Username,
			Action:     e.Action,
			Target:     e.Target,
			Detail:     e.Detail,
			IP:         e.IP,
			CreatedAt:  e.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, auditListResponse{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

func atoiDefault(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}
