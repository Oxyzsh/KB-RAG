package web

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestUserManagement(t *testing.T) {
	env := newTestServer(t)
	rec := env.callAs(t, "admin1", "POST", "/api/users", userBody(t, "viewer2", testPassword, "viewer"), "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created userResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.User.Username != "viewer2" || created.User.Role != "viewer" {
		t.Fatalf("created = %+v", created)
	}

	rec = env.callAs(t, "admin1", "GET", "/api/users", nil, "")
	var list userListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Users) != 5 {
		t.Fatalf("users = %+v", list.Users)
	}

	roleBody := []byte(`{"role":"editor"}`)
	rec = env.callAs(t, "admin1", "PATCH", fmtID("/api/users/%d", created.User.ID), roleBody, "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("update role status = %d body=%s", rec.Code, rec.Body.String())
	}
	var updated userResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.User.Role != "editor" {
		t.Fatalf("role = %q", updated.User.Role)
	}

	newPassword := "new-password-123"
	passBody, _ := json.Marshal(updateUserRequest{Password: &newPassword})
	rec = env.callAs(t, "admin1", "PATCH", fmtID("/api/users/%d", created.User.ID), passBody, "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("reset status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec := login(t, env, "default", "viewer2", newPassword); rec.Code != http.StatusOK {
		t.Fatalf("新密码登录 status = %d", rec.Code)
	}
	if rec := login(t, env, "default", "viewer2", testPassword); rec.Code != http.StatusUnauthorized {
		t.Fatalf("旧密码登录 status = %d", rec.Code)
	}

	rec = env.callAs(t, "admin1", "DELETE", fmtID("/api/users/%d", created.User.ID), nil, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = env.callAs(t, "admin1", "GET", "/api/users", nil, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Users) != 4 {
		t.Fatalf("删除后 users = %+v", list.Users)
	}
}

func TestUserValidationAndProtection(t *testing.T) {
	env := newTestServer(t)
	if rec := env.callAs(t, "viewer1", "POST", "/api/users", userBody(t, "x-user", testPassword, "viewer"), "application/json"); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer create status = %d", rec.Code)
	}
	if rec := env.callAs(t, "admin1", "POST", "/api/users", userBody(t, "short-pass", "short", "viewer"), "application/json"); rec.Code != http.StatusBadRequest {
		t.Fatalf("weak password status = %d", rec.Code)
	}
	if rec := env.callAs(t, "admin1", "POST", "/api/users", userBody(t, "bad-role", testPassword, "owner"), "application/json"); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad role status = %d", rec.Code)
	}
	if rec := env.callAs(t, "admin1", "POST", "/api/users", userBody(t, "viewer1", testPassword, "viewer"), "application/json"); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d", rec.Code)
	}

	selfRole := []byte(`{"role":"viewer"}`)
	if rec := env.callAs(t, "admin1", "PATCH", fmtID("/api/users/%d", env.users["admin1"].ID), selfRole, "application/json"); rec.Code != http.StatusBadRequest {
		t.Fatalf("self role status = %d", rec.Code)
	}
	if rec := env.callAs(t, "admin1", "DELETE", fmtID("/api/users/%d", env.users["admin1"].ID), nil, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("self delete status = %d", rec.Code)
	}
	if rec := env.callAs(t, "admin1", "DELETE", fmtID("/api/users/%d", env.users["root"].ID), nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("delete super status = %d", rec.Code)
	}
	if rec := env.callAs(t, "acme-admin", "PATCH", fmtID("/api/users/%d", env.users["admin1"].ID), selfRole, "application/json"); rec.Code != http.StatusNotFound {
		t.Fatalf("cross tenant update status = %d", rec.Code)
	}
	if rec := env.callAs(t, "acme-admin", "DELETE", fmtID("/api/users/%d", env.users["viewer1"].ID), nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("cross tenant delete status = %d", rec.Code)
	}
}
