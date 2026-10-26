package web

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestSuperSwitchTenant(t *testing.T) {
	env := newTestServer(t)
	if rec := uploadDoc(t, env, "editor1", "年假.md", []byte("# 休假\n\n员工满1年享5天年假。\n")); rec.Code != http.StatusCreated {
		t.Fatalf("default upload = %d", rec.Code)
	}
	if rec := uploadDoc(t, env, "acme-admin", "食堂.md", []byte("# 食堂\n\n食堂中午十二点开放。\n")); rec.Code != http.StatusCreated {
		t.Fatalf("acme upload = %d", rec.Code)
	}

	rec := env.callAsTenant(t, "root", "acme", "GET", "/api/session", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("session status = %d body=%s", rec.Code, rec.Body.String())
	}
	var session sessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.User.TenantSlug != "acme" || session.User.HomeTenantSlug != "default" || !session.User.CanSwitch {
		t.Fatalf("switched session = %+v", session.User)
	}

	var docs listDocumentsResponse
	rec = env.callAsTenant(t, "root", "acme", "GET", "/api/documents", nil, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &docs); err != nil {
		t.Fatal(err)
	}
	if len(docs.Documents) != 1 || docs.Documents[0].Name != "食堂.md" {
		t.Fatalf("acme documents = %+v", docs.Documents)
	}
	rec = env.callAs(t, "root", "GET", "/api/documents", nil, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &docs); err != nil {
		t.Fatal(err)
	}
	if len(docs.Documents) != 1 || docs.Documents[0].Name != "年假.md" {
		t.Fatalf("default documents = %+v", docs.Documents)
	}

	rec = env.callAsTenant(t, "root", "acme", "POST", "/api/ask", askBody(t, "年假怎么请"), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("ask status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp askResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Citations) != 1 || resp.Citations[0].Source != "食堂.md" {
		t.Fatalf("acme ask citations = %+v", resp.Citations)
	}

	var users userListResponse
	rec = env.callAsTenant(t, "root", "acme", "GET", "/api/users", nil, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil {
		t.Fatal(err)
	}
	if len(users.Users) != 1 || users.Users[0].Username != "acme-admin" {
		t.Fatalf("acme users = %+v", users.Users)
	}

	rec = env.callAsTenant(t, "root", "acme", "POST", "/api/users", userBody(t, "acme-viewer", testPassword, "viewer"), "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create acme user status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created userResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.User.TenantSlug != "acme" {
		t.Fatalf("created user tenant = %q", created.User.TenantSlug)
	}
	rec = env.callAs(t, "admin1", "GET", "/api/users", nil, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil {
		t.Fatal(err)
	}
	for _, u := range users.Users {
		if u.Username == "acme-viewer" {
			t.Fatal("acme 用户不应出现在 default 租户列表")
		}
	}

	rec = env.callAsTenant(t, "admin1", "acme", "GET", "/api/documents", nil, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("非超级管理员切换 status = %d", rec.Code)
	}
	rec = env.callAsTenant(t, "root", "nope", "GET", "/api/documents", nil, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("未知租户 status = %d", rec.Code)
	}

	rec = env.callAsTenant(t, "root", "acme", "GET", "/api/audit", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("acme audit status = %d", rec.Code)
	}
	var audit auditListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &audit); err != nil {
		t.Fatal(err)
	}
	if len(audit.Items) == 0 {
		t.Fatal("acme 审计为空")
	}
	for _, item := range audit.Items {
		if item.TenantSlug != "acme" {
			t.Fatalf("切换后审计混入其他租户: %+v", item)
		}
	}

	rec = env.callAsTenant(t, "root", "acme", "GET", "/api/tenants", nil, "")
	var tenants tenantsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &tenants); err != nil {
		t.Fatal(err)
	}
	if len(tenants.Tenants) != 2 {
		t.Fatalf("超级管理员应仍能看到全部租户: %+v", tenants.Tenants)
	}
}
