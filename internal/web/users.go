package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"kb-rag/internal/auth"
	"kb-rag/internal/store"
)

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	users, err := s.store.ListUsers(r.Context(), p.TenantID)
	if err != nil {
		s.logger.Error("读取用户列表失败", "error", err)
		writeError(w, http.StatusInternalServerError, "读取用户列表失败")
		return
	}
	tenant := store.Tenant{ID: p.TenantID, Slug: p.TenantSlug, Name: p.TenantName}
	out := make([]userDTO, 0, len(users))
	for _, u := range users {
		out = append(out, userDTOFromStore(u, tenant))
	}
	writeJSON(w, http.StatusOK, userListResponse{Users: out})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var req createUserRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	username := strings.TrimSpace(req.Username)
	if !validateUsername(username) {
		writeError(w, http.StatusBadRequest, "用户名需为 3-64 个字符且不含空白")
		return
	}
	if !store.ValidRole(req.Role) {
		writeError(w, http.StatusBadRequest, "角色必须是 admin、editor 或 viewer")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrWeakPassword) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.logger.Error("生成密码哈希失败", "error", err)
		writeError(w, http.StatusInternalServerError, "创建用户失败")
		return
	}
	u, err := s.store.CreateUser(r.Context(), p.TenantID, username, hash, req.Role)
	if err != nil {
		if errors.Is(err, store.ErrUsernameTaken) {
			writeError(w, http.StatusConflict, "用户名已存在")
			return
		}
		s.logger.Error("创建用户失败", "error", err)
		writeError(w, http.StatusInternalServerError, "创建用户失败")
		return
	}
	s.audit(r, p, p.TenantID, "user.create", username, map[string]any{"role": req.Role})
	tenant := store.Tenant{ID: p.TenantID, Slug: p.TenantSlug, Name: p.TenantName}
	writeJSON(w, http.StatusCreated, userResponse{User: userDTOFromStore(u, tenant)})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "用户 ID 非法")
		return
	}
	var req updateUserRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if req.Role == nil && req.Password == nil {
		writeError(w, http.StatusBadRequest, "没有需要修改的字段")
		return
	}
	if req.Role != nil && !store.ValidRole(*req.Role) {
		writeError(w, http.StatusBadRequest, "角色必须是 admin、editor 或 viewer")
		return
	}
	if req.Role != nil && id == p.UserID && *req.Role != p.Role {
		writeError(w, http.StatusBadRequest, "不能修改自己的角色")
		return
	}
	var newHash string
	if req.Password != nil {
		newHash, err = auth.HashPassword(*req.Password)
		if err != nil {
			if errors.Is(err, auth.ErrWeakPassword) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			s.logger.Error("生成密码哈希失败", "error", err)
			writeError(w, http.StatusInternalServerError, "修改用户失败")
			return
		}
	}

	if req.Role != nil {
		if err := s.store.UpdateUserRole(r.Context(), p.TenantID, id, *req.Role); err != nil {
			if writeUserStoreError(w, err) {
				return
			}
			s.logger.Error("修改用户角色失败", "error", err, "id", id)
			writeError(w, http.StatusInternalServerError, "修改用户失败")
			return
		}
	}
	if req.Password != nil {
		if err := s.store.ResetUserPassword(r.Context(), p.TenantID, id, newHash); err != nil {
			if writeUserStoreError(w, err) {
				return
			}
			s.logger.Error("重置密码失败", "error", err, "id", id)
			writeError(w, http.StatusInternalServerError, "修改用户失败")
			return
		}
	}

	u, ok, err := s.store.GetUser(r.Context(), p.TenantID, id)
	if err != nil || !ok {
		s.logger.Error("读取用户失败", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "修改用户失败")
		return
	}
	detail := map[string]any{}
	if req.Role != nil {
		detail["role"] = *req.Role
	}
	if req.Password != nil {
		detail["password_reset"] = true
	}
	s.audit(r, p, p.TenantID, "user.update", u.Username, detail)
	tenant := store.Tenant{ID: p.TenantID, Slug: p.TenantSlug, Name: p.TenantName}
	writeJSON(w, http.StatusOK, userResponse{User: userDTOFromStore(u, tenant)})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "用户 ID 非法")
		return
	}
	u, ok, err := s.store.GetUser(r.Context(), p.TenantID, id)
	if err != nil {
		s.logger.Error("读取用户失败", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "删除用户失败")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "用户不存在")
		return
	}
	if err := s.store.DeleteUser(r.Context(), p.TenantID, id, p.UserID); err != nil {
		if writeUserStoreError(w, err) {
			return
		}
		s.logger.Error("删除用户失败", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "删除用户失败")
		return
	}
	s.audit(r, p, p.TenantID, "user.delete", u.Username, nil)
	w.WriteHeader(http.StatusNoContent)
}

func writeUserStoreError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, store.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "用户不存在")
	case errors.Is(err, store.ErrLastAdmin):
		writeError(w, http.StatusConflict, "不能删除或降级租户最后一个管理员")
	case errors.Is(err, store.ErrProtectedUser):
		writeError(w, http.StatusForbidden, "不能修改或删除超级管理员")
	case errors.Is(err, store.ErrSelfDelete):
		writeError(w, http.StatusBadRequest, "不能删除当前登录用户")
	default:
		return false
	}
	return true
}
