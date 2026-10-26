package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"kb-rag/internal/auth"
	"kb-rag/internal/store"
)

type principalCtxKey struct{}

func withPrincipal(ctx context.Context, p store.Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

func principalFrom(ctx context.Context) (store.Principal, bool) {
	p, ok := ctx.Value(principalCtxKey{}).(store.Principal)
	return p, ok
}

var dummyPasswordHash = sync.OnceValue(func() string {
	hash, _ := auth.HashPassword("kb-rag-timing-dummy")
	return hash
})

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	max      int
	window   time.Duration
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{attempts: make(map[string][]time.Time), max: max, window: window}
}

func (l *loginLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-l.window)
	list := l.attempts[key]
	kept := list[:0]
	for _, t := range list {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.attempts, key)
		return true
	}
	l.attempts[key] = kept
	return len(kept) < l.max
}

func (l *loginLimiter) fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.attempts[key] = append(l.attempts[key], now)
}

func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

func (s *Server) withSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, prefix) {
			writeError(w, http.StatusUnauthorized, "未认证")
			return
		}
		token := strings.TrimSpace(header[len(prefix):])
		if token == "" {
			writeError(w, http.StatusUnauthorized, "未认证")
			return
		}
		p, ok, err := s.store.PrincipalByTokenHash(r.Context(), auth.HashToken(token), time.Now())
		if err != nil {
			s.logger.Error("读取会话失败", "error", err)
			writeError(w, http.StatusInternalServerError, "读取会话失败")
			return
		}
		if !ok {
			writeError(w, http.StatusUnauthorized, "未认证")
			return
		}
		if slug := strings.TrimSpace(r.Header.Get("X-Tenant")); slug != "" {
			if !p.IsSuper {
				writeError(w, http.StatusForbidden, "无权切换租户")
				return
			}
			if slug != p.TenantSlug {
				tenant, ok, err := s.store.TenantBySlug(r.Context(), slug)
				if err != nil {
					s.logger.Error("切换租户失败", "error", err, "slug", slug)
					writeError(w, http.StatusInternalServerError, "切换租户失败")
					return
				}
				if !ok {
					writeError(w, http.StatusBadRequest, "租户不存在")
					return
				}
				p.TenantID = tenant.ID
				p.TenantSlug = tenant.Slug
				p.TenantName = tenant.Name
			}
		}
		next.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), p)))
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req loginRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	tenantSlug := strings.TrimSpace(req.Tenant)
	if tenantSlug == "" {
		tenantSlug = "default"
	}
	username := strings.TrimSpace(req.Username)
	if username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "租户、用户名和密码不能为空")
		return
	}

	now := time.Now()
	key := clientIP(r) + "|" + tenantSlug + "|" + username
	if !s.limiter.allow(key, now) {
		writeError(w, http.StatusTooManyRequests, "尝试次数过多，请稍后再试")
		return
	}

	tenant, tenantOK, err := s.store.TenantBySlug(r.Context(), tenantSlug)
	if err != nil {
		s.logger.Error("读取租户失败", "error", err)
		writeError(w, http.StatusInternalServerError, "登录失败")
		return
	}
	var user store.User
	userOK := false
	if tenantOK {
		user, userOK, err = s.store.GetUserByUsername(r.Context(), tenant.ID, username)
		if err != nil {
			s.logger.Error("读取用户失败", "error", err)
			writeError(w, http.StatusInternalServerError, "登录失败")
			return
		}
	}

	encoded := dummyPasswordHash()
	if userOK {
		encoded = user.PasswordHash
	}
	s.hashSem <- struct{}{}
	valid, verifyErr := auth.VerifyPassword(encoded, req.Password)
	<-s.hashSem
	if verifyErr != nil && userOK {
		s.logger.Error("密码哈希损坏", "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "登录失败")
		return
	}
	if !userOK || !valid {
		s.limiter.fail(key, now)
		s.auditLoginFailure(r, tenant, tenantOK, username)
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	s.limiter.reset(key)

	token, err := auth.NewSessionToken()
	if err != nil {
		s.logger.Error("生成会话令牌失败", "error", err)
		writeError(w, http.StatusInternalServerError, "登录失败")
		return
	}
	expires := now.Add(s.sessionTTL)
	if err := s.store.CreateSession(r.Context(), auth.HashToken(token), user.ID, expires); err != nil {
		s.logger.Error("创建会话失败", "error", err)
		writeError(w, http.StatusInternalServerError, "登录失败")
		return
	}

	p := store.Principal{
		UserID:         user.ID,
		Username:       user.Username,
		Role:           user.Role,
		IsSuper:        user.IsSuper,
		TenantID:       tenant.ID,
		TenantSlug:     tenant.Slug,
		TenantName:     tenant.Name,
		HomeTenantID:   tenant.ID,
		HomeTenantSlug: tenant.Slug,
		HomeTenantName: tenant.Name,
		TokenHash:      auth.HashToken(token),
	}
	s.audit(r, p, p.TenantID, "login.success", username, nil)
	writeJSON(w, http.StatusOK, loginResponse{
		Token:     token,
		ExpiresAt: expires.UTC().Format(time.RFC3339),
		User:      userDTOFromPrincipal(p, user.CreatedAt, user.UpdatedAt),
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if err := s.store.DeleteSession(r.Context(), p.TokenHash); err != nil {
		s.logger.Error("删除会话失败", "error", err)
		writeError(w, http.StatusInternalServerError, "退出失败")
		return
	}
	s.audit(r, p, p.TenantID, "logout", p.Username, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	writeJSON(w, http.StatusOK, sessionResponse{User: userDTOFromPrincipal(p, "", "")})
}

func userDTOFromPrincipal(p store.Principal, createdAt, updatedAt string) userDTO {
	return userDTO{
		ID:             p.UserID,
		Username:       p.Username,
		Role:           p.Role,
		IsSuper:        p.IsSuper,
		TenantID:       p.TenantID,
		TenantSlug:     p.TenantSlug,
		TenantName:     p.TenantName,
		HomeTenantID:   p.HomeTenantID,
		HomeTenantSlug: p.HomeTenantSlug,
		HomeTenantName: p.HomeTenantName,
		CanSwitch:      p.IsSuper,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}
}

func userDTOFromStore(u store.User, tenant store.Tenant) userDTO {
	return userDTO{
		ID:             u.ID,
		Username:       u.Username,
		Role:           u.Role,
		IsSuper:        u.IsSuper,
		TenantID:       u.TenantID,
		TenantSlug:     tenant.Slug,
		TenantName:     tenant.Name,
		HomeTenantID:   u.TenantID,
		HomeTenantSlug: tenant.Slug,
		HomeTenantName: tenant.Name,
		CanSwitch:      u.IsSuper,
		CreatedAt:      u.CreatedAt,
		UpdatedAt:      u.UpdatedAt,
	}
}

func (s *Server) audit(r *http.Request, p store.Principal, tenantID int64, action, target string, detail any) {
	e := store.AuditEntry{
		TenantID: &tenantID,
		UserID:   &p.UserID,
		Username: p.Username,
		Action:   action,
		Target:   target,
		Detail:   "{}",
		IP:       clientIP(r),
	}
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			e.Detail = string(b)
		}
	}
	s.writeAudit(e)
}

func (s *Server) auditLoginFailure(r *http.Request, tenant store.Tenant, tenantOK bool, username string) {
	e := store.AuditEntry{
		Username: username,
		Action:   "login.failure",
		Detail:   `{"reason":"invalid_credentials"}`,
		IP:       clientIP(r),
	}
	if tenantOK {
		e.TenantID = &tenant.ID
	}
	s.writeAudit(e)
}

func (s *Server) writeAudit(e store.AuditEntry) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.store.InsertAudit(ctx, e); err != nil {
		s.logger.Warn("写审计日志失败", "action", e.Action, "error", err)
	}
}

func validateUsername(username string) bool {
	n := len([]rune(username))
	if n < 3 || n > 64 {
		return false
	}
	for _, r := range username {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r < 32 {
			return false
		}
	}
	return true
}
