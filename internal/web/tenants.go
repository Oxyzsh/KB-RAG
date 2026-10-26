package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"kb-rag/internal/auth"
	"kb-rag/internal/store"
)

var tenantSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,31}$`)

func (s *Server) handleListTenants(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if !p.IsSuper {
		writeJSON(w, http.StatusOK, tenantsResponse{Tenants: []tenantDTO{{
			ID: p.TenantID, Slug: p.TenantSlug, Name: p.TenantName,
		}}})
		return
	}
	tenants, err := s.store.ListTenants(r.Context())
	if err != nil {
		s.logger.Error("读取租户列表失败", "error", err)
		writeError(w, http.StatusInternalServerError, "读取租户列表失败")
		return
	}
	out := make([]tenantDTO, 0, len(tenants))
	for _, t := range tenants {
		out = append(out, tenantDTO{ID: t.ID, Slug: t.Slug, Name: t.Name})
	}
	writeJSON(w, http.StatusOK, tenantsResponse{Tenants: out})
}

func (s *Server) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var req createTenantRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	slug := strings.TrimSpace(req.Slug)
	name := strings.TrimSpace(req.Name)
	username := strings.TrimSpace(req.AdminUsername)
	if !tenantSlugPattern.MatchString(slug) {
		writeError(w, http.StatusBadRequest, "租户标识需为 2-32 位小写字母、数字或连字符")
		return
	}
	if name == "" || len([]rune(name)) > 64 {
		writeError(w, http.StatusBadRequest, "租户名称不能为空且不超过 64 个字符")
		return
	}
	if !validateUsername(username) {
		writeError(w, http.StatusBadRequest, "管理员用户名需为 3-64 个字符且不含空白")
		return
	}
	hash, err := auth.HashPassword(req.AdminPassword)
	if err != nil {
		if errors.Is(err, auth.ErrWeakPassword) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.logger.Error("生成密码哈希失败", "error", err)
		writeError(w, http.StatusInternalServerError, "创建租户失败")
		return
	}
	tenant, user, err := s.store.CreateTenantWithAdmin(r.Context(), slug, name, username, hash)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrTenantSlugTaken):
			writeError(w, http.StatusConflict, "租户标识已存在")
		case errors.Is(err, store.ErrTenantNameExists):
			writeError(w, http.StatusConflict, "租户名称已存在")
		case errors.Is(err, store.ErrUsernameTaken):
			writeError(w, http.StatusConflict, "管理员用户名已存在")
		default:
			s.logger.Error("创建租户失败", "error", err)
			writeError(w, http.StatusInternalServerError, "创建租户失败")
		}
		return
	}
	s.audit(r, p, tenant.ID, "tenant.create", tenant.Slug, map[string]any{
		"name":           tenant.Name,
		"admin_username": username,
	})
	writeJSON(w, http.StatusCreated, createTenantResponse{
		Tenant: tenantDTO{ID: tenant.ID, Slug: tenant.Slug, Name: tenant.Name},
		User:   userDTOFromStore(user, tenant),
	})
}

func (s *Server) handleResetTenantAdmin(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	slug := strings.TrimSpace(r.PathValue("slug"))
	tenant, ok, err := s.store.TenantBySlug(r.Context(), slug)
	if err != nil {
		s.logger.Error("读取租户失败", "error", err, "slug", slug)
		writeError(w, http.StatusInternalServerError, "重置失败")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "租户不存在")
		return
	}
	var req resetTenantAdminRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		writeError(w, http.StatusBadRequest, "管理员用户名不能为空")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrWeakPassword) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.logger.Error("生成密码哈希失败", "error", err)
		writeError(w, http.StatusInternalServerError, "重置失败")
		return
	}
	if err := s.store.ResetTenantAdminPassword(r.Context(), tenant.ID, username, hash); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "该租户下没有此管理员")
			return
		}
		s.logger.Error("重置租户管理员密码失败", "error", err, "slug", slug)
		writeError(w, http.StatusInternalServerError, "重置失败")
		return
	}
	s.audit(r, p, tenant.ID, "tenant.admin.reset", username, map[string]any{"slug": slug})
	w.WriteHeader(http.StatusNoContent)
}
