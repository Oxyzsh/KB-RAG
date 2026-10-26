package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestBootstrapAdmin(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "kb.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	admin, created, err := s.EnsureBootstrapAdmin(ctx, "root", "hash")
	if err != nil || !created {
		t.Fatalf("EnsureBootstrapAdmin = %+v, created=%v, err=%v", admin, created, err)
	}
	if admin.Role != "admin" || !admin.IsSuper || admin.TenantID == 0 {
		t.Fatalf("admin = %+v", admin)
	}
	tenant, ok, err := s.GetTenant(ctx, admin.TenantID)
	if err != nil || !ok || tenant.Slug != "default" {
		t.Fatalf("tenant = %+v, ok=%v, err=%v", tenant, ok, err)
	}
	if n, err := s.CountUsers(ctx); err != nil || n != 1 {
		t.Fatalf("CountUsers = %d, err=%v", n, err)
	}
	if _, created, err := s.EnsureBootstrapAdmin(ctx, "other", "hash2"); err != nil || created {
		t.Fatalf("第二次 bootstrap created=%v err=%v", created, err)
	}
}

func TestCreateTenantWithAdminAndUsers(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tenant, admin, err := s.CreateTenantWithAdmin(ctx, "acme", "Acme", "acme-admin", "hash-admin")
	if err != nil {
		t.Fatalf("CreateTenantWithAdmin: %v", err)
	}
	if tenant.Slug != "acme" || admin.Role != "admin" || admin.IsSuper {
		t.Fatalf("tenant=%+v admin=%+v", tenant, admin)
	}
	if _, _, err := s.CreateTenantWithAdmin(ctx, "acme", "Other", "x-admin", "hash"); !errors.Is(err, ErrTenantSlugTaken) {
		t.Fatalf("重复 slug 错误 = %v", err)
	}
	if _, _, err := s.CreateTenantWithAdmin(ctx, "other", "Acme", "y-admin", "hash"); !errors.Is(err, ErrTenantNameExists) {
		t.Fatalf("重复名称错误 = %v", err)
	}
	if _, err := s.CreateUser(ctx, tenant.ID, "editor1", "hash", "editor"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(ctx, tenant.ID, "editor1", "hash", "viewer"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("重复用户错误 = %v", err)
	}
	users, err := s.ListUsers(ctx, tenant.ID)
	if err != nil || len(users) != 2 {
		t.Fatalf("users = %+v, err=%v", users, err)
	}

	if err := s.UpdateUserRole(ctx, tenant.ID, admin.ID, "editor"); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("降级最后管理员错误 = %v", err)
	}
	if err := s.DeleteUser(ctx, tenant.ID, admin.ID, admin.ID); !errors.Is(err, ErrSelfDelete) {
		t.Fatalf("删除自己错误 = %v", err)
	}
	if err := s.DeleteUser(ctx, tenant.ID, admin.ID, 0); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("删除最后管理员错误 = %v", err)
	}
	second, err := s.CreateUser(ctx, tenant.ID, "admin2", "hash", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateUserRole(ctx, tenant.ID, second.ID, "viewer"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(ctx, tenant.ID, second.ID, admin.ID); err != nil {
		t.Fatal(err)
	}

	editor, ok, err := s.GetUserByUsername(ctx, tenant.ID, "editor1")
	if err != nil || !ok {
		t.Fatalf("editor = %+v, ok=%v, err=%v", editor, ok, err)
	}
	token := "session-token"
	if err := s.CreateSession(ctx, token, editor.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.PrincipalByTokenHash(ctx, token, time.Now()); err != nil || !ok {
		t.Fatalf("会话应存在: ok=%v err=%v", ok, err)
	}
	if err := s.ResetUserPassword(ctx, tenant.ID, editor.ID, "new-hash"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.PrincipalByTokenHash(ctx, token, time.Now()); err != nil || ok {
		t.Fatalf("重置密码后会话应失效: ok=%v err=%v", ok, err)
	}
	updated, _, _ := s.GetUser(ctx, tenant.ID, editor.ID)
	if updated.PasswordHash != "new-hash" {
		t.Fatalf("password hash = %q", updated.PasswordHash)
	}
}

func TestSessionLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tenant, admin, err := s.CreateTenantWithAdmin(ctx, "acme", "Acme", "acme-admin", "hash")
	if err != nil {
		t.Fatal(err)
	}

	good := "good-token"
	if err := s.CreateSession(ctx, good, admin.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	p, ok, err := s.PrincipalByTokenHash(ctx, good, time.Now())
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if p.Username != "acme-admin" || p.TenantSlug != "acme" || p.TenantID != tenant.ID || p.Role != "admin" {
		t.Fatalf("principal = %+v", p)
	}

	expired := "expired-token"
	if err := s.CreateSession(ctx, expired, admin.ID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.PrincipalByTokenHash(ctx, expired, time.Now()); err != nil || ok {
		t.Fatalf("过期会话 ok=%v err=%v", ok, err)
	}
	if err := s.DeleteExpiredSessions(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.PrincipalByTokenHash(ctx, expired, time.Now()); ok {
		t.Fatal("过期会话应被清理")
	}
	if err := s.DeleteSession(ctx, good); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.PrincipalByTokenHash(ctx, good, time.Now()); ok {
		t.Fatal("登出后会话应失效")
	}
}

func TestSuperUserProtected(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "kb.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	admin, _, err := s.EnsureBootstrapAdmin(ctx, "root", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateUserRole(ctx, admin.TenantID, admin.ID, "editor"); !errors.Is(err, ErrProtectedUser) {
		t.Fatalf("修改 super 角色错误 = %v", err)
	}
	if err := s.DeleteUser(ctx, admin.TenantID, admin.ID, 0); !errors.Is(err, ErrProtectedUser) {
		t.Fatalf("删除 super 错误 = %v", err)
	}
	if err := s.ResetUserPassword(ctx, admin.TenantID, admin.ID, "new"); !errors.Is(err, ErrProtectedUser) {
		t.Fatalf("重置 super 密码错误 = %v", err)
	}
}

func TestResetTenantAdminPassword(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	_, admin, err := s.CreateTenantWithAdmin(ctx, "acme", "Acme", "acme-admin", "old-hash")
	if err != nil {
		t.Fatal(err)
	}
	token := "acme-token"
	if err := s.CreateSession(ctx, token, admin.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetTenantAdminPassword(ctx, admin.TenantID, "acme-admin", "new-hash"); err != nil {
		t.Fatal(err)
	}
	u, _, _ := s.GetUser(ctx, admin.TenantID, admin.ID)
	if u.PasswordHash != "new-hash" {
		t.Fatalf("hash = %q", u.PasswordHash)
	}
	if _, ok, _ := s.PrincipalByTokenHash(ctx, token, time.Now()); ok {
		t.Fatal("重置后会话应失效")
	}
	if err := s.ResetTenantAdminPassword(ctx, admin.TenantID, "missing", "x"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("不存在管理员错误 = %v", err)
	}
}
