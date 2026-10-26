package web

import (
	"encoding/json"
	"net/http"
	"testing"
)

func tenantBody(t *testing.T, slug, name, username, password string) []byte {
	t.Helper()
	b, err := json.Marshal(createTenantRequest{
		Slug: slug, Name: name, AdminUsername: username, AdminPassword: password,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTenantManagement(t *testing.T) {
	env := newTestServer(t)
	rec := env.callAs(t, "root", "POST", "/api/tenants", tenantBody(t, "beta", "Beta", "beta-admin", testPassword), "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tenant status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created createTenantResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Tenant.Slug != "beta" || created.User.Username != "beta-admin" || created.User.Role != "admin" {
		t.Fatalf("created = %+v", created)
	}
	if rec := env.callAs(t, "root", "POST", "/api/tenants", tenantBody(t, "beta", "Other", "other-admin", testPassword), "application/json"); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate slug status = %d", rec.Code)
	}
	if rec := env.callAs(t, "admin1", "POST", "/api/tenants", tenantBody(t, "gamma", "Gamma", "gamma-admin", testPassword), "application/json"); rec.Code != http.StatusForbidden {
		t.Fatalf("admin create tenant status = %d", rec.Code)
	}
	if rec := env.callAs(t, "root", "POST", "/api/tenants", tenantBody(t, "Bad Slug", "Bad", "bad-admin", testPassword), "application/json"); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad slug status = %d", rec.Code)
	}
	if rec := env.callAs(t, "root", "POST", "/api/tenants", tenantBody(t, "weak", "Weak", "weak-admin", "short"), "application/json"); rec.Code != http.StatusBadRequest {
		t.Fatalf("weak password status = %d", rec.Code)
	}

	rec = env.callAs(t, "admin1", "GET", "/api/tenants", nil, "")
	var own tenantsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &own); err != nil {
		t.Fatal(err)
	}
	if len(own.Tenants) != 1 || own.Tenants[0].Slug != "default" {
		t.Fatalf("admin tenants = %+v", own.Tenants)
	}
	rec = env.callAs(t, "root", "GET", "/api/tenants", nil, "")
	var all tenantsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &all); err != nil {
		t.Fatal(err)
	}
	if len(all.Tenants) != 3 {
		t.Fatalf("super tenants = %+v", all.Tenants)
	}

	if rec := login(t, env, "beta", "beta-admin", testPassword); rec.Code != http.StatusOK {
		t.Fatalf("beta login status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestResetTenantAdminPassword(t *testing.T) {
	env := newTestServer(t)
	if rec := env.callAs(t, "root", "POST", "/api/tenants", tenantBody(t, "beta", "Beta", "beta-admin", testPassword), "application/json"); rec.Code != http.StatusCreated {
		t.Fatalf("create tenant = %d", rec.Code)
	}
	rec := login(t, env, "beta", "beta-admin", testPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d", rec.Code)
	}
	oldToken := decodeLogin(t, rec).Token

	newPassword := "reset-password-123"
	body, _ := json.Marshal(resetTenantAdminRequest{Username: "beta-admin", Password: newPassword})
	rec = env.callAs(t, "root", "POST", "/api/tenants/beta/admin/reset", body, "application/json")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("reset status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec := call(t, env.srv.Handler(), "GET", "/api/session", nil, oldToken, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("重置后旧会话 status = %d", rec.Code)
	}
	if rec := login(t, env, "beta", "beta-admin", newPassword); rec.Code != http.StatusOK {
		t.Fatalf("新密码登录 status = %d", rec.Code)
	}
	if rec := login(t, env, "beta", "beta-admin", testPassword); rec.Code != http.StatusUnauthorized {
		t.Fatalf("旧密码登录 status = %d", rec.Code)
	}
	if rec := env.callAs(t, "root", "POST", "/api/tenants/missing/admin/reset", body, "application/json"); rec.Code != http.StatusNotFound {
		t.Fatalf("不存在的租户 status = %d", rec.Code)
	}
}
