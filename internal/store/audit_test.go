package store

import (
	"context"
	"testing"
)

func TestAuditInsertAndList(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tenantID := testTenant(t, s)
	userID := int64(42)

	entries := []AuditEntry{
		{TenantID: &tenantID, UserID: &userID, Username: "admin", Action: "login.success", Detail: "{}", IP: "127.0.0.1"},
		{Username: "ghost", Action: "login.failure", Detail: `{"reason":"invalid_credentials"}`, IP: "127.0.0.1"},
		{TenantID: &tenantID, UserID: &userID, Username: "admin", Action: "ask", Detail: `{"question":"q"}`, IP: "127.0.0.1"},
	}
	for _, e := range entries {
		if err := s.InsertAudit(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	items, total, err := s.ListAudit(ctx, AuditFilter{TenantID: &tenantID})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("tenant audit total=%d items=%d", total, len(items))
	}
	for _, e := range items {
		if e.TenantID == nil || *e.TenantID != tenantID || e.TenantSlug != "default" {
			t.Fatalf("entry = %+v", e)
		}
	}

	items, total, err = s.ListAudit(ctx, AuditFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(items) != 3 {
		t.Fatalf("all audit total=%d items=%d", total, len(items))
	}
	nullTenants := 0
	for _, e := range items {
		if e.TenantID == nil {
			nullTenants++
		}
	}
	if nullTenants != 1 {
		t.Fatalf("未知用户登录失败应为唯一 NULL tenant 记录: %+v", items)
	}

	items, total, err = s.ListAudit(ctx, AuditFilter{Action: "login.failure"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || items[0].Username != "ghost" {
		t.Fatalf("action filter total=%d items=%+v", total, items)
	}

	items, total, err = s.ListAudit(ctx, AuditFilter{Page: 2, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(items) != 1 {
		t.Fatalf("page total=%d items=%d", total, len(items))
	}
}
