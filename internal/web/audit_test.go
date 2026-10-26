package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAuditAPI(t *testing.T) {
	env := newTestServer(t)
	// 未知租户登录失败：tenant_id 为 NULL。
	login(t, env, "missing", "ghost", "whatever-password")
	if rec := uploadDoc(t, env, "editor1", "年假.md", []byte("# 休假\n\n员工累计工作满1年可享5天年假。\n")); rec.Code != http.StatusCreated {
		t.Fatalf("upload = %d", rec.Code)
	}
	if rec := env.callAs(t, "viewer1", "POST", "/api/ask", askBody(t, "年假怎么请"), "application/json"); rec.Code != http.StatusOK {
		t.Fatalf("ask = %d", rec.Code)
	}

	if rec := env.callAs(t, "viewer1", "GET", "/api/audit", nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer audit status = %d", rec.Code)
	}
	if rec := env.callAs(t, "admin1", "GET", "/api/audit?scope=all", nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("admin global audit status = %d", rec.Code)
	}

	rec := env.callAs(t, "admin1", "GET", "/api/audit?page_size=200", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin audit status = %d body=%s", rec.Code, rec.Body.String())
	}
	var list auditListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	actions := map[string]int{}
	for _, item := range list.Items {
		actions[item.Action]++
		if strings.Contains(item.Detail, testPassword) {
			t.Fatalf("审计包含密码: %+v", item)
		}
	}
	if actions["document.upload"] != 1 || actions["ask"] != 1 {
		t.Fatalf("租户审计 actions = %+v, items=%+v", actions, list.Items)
	}
	if actions["login.failure"] != 0 {
		t.Fatalf("租户管理员不应看到 NULL tenant 的登录失败: %+v", actions)
	}

	rec = env.callAs(t, "root", "GET", "/api/audit?scope=all&page_size=200", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("super global audit status = %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	nullFailure := 0
	var askItem *auditDTO
	for i := range list.Items {
		item := list.Items[i]
		if item.Action == "login.failure" && item.TenantID == nil {
			nullFailure++
		}
		if item.Action == "ask" {
			askItem = &list.Items[i]
		}
	}
	if nullFailure != 1 {
		t.Fatalf("super 全局审计应包含 NULL tenant 登录失败: %+v", list.Items)
	}
	if askItem == nil {
		t.Fatal("缺少 ask 审计")
	}
	if !strings.Contains(askItem.Detail, "年假怎么请") {
		t.Fatalf("ask detail 缺少问题原文: %s", askItem.Detail)
	}
	if strings.Contains(askItem.Detail, "员工满1年享5天年假") {
		t.Fatalf("ask detail 不应包含答案: %s", askItem.Detail)
	}

	rec = env.callAs(t, "root", "GET", "/api/audit?scope=all&action=ask&page_size=1", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("filter status = %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].Action != "ask" {
		t.Fatalf("filter items = %+v", list.Items)
	}
	if list.Total < 1 || list.PageSize != 1 {
		t.Fatalf("filter meta = total %d page_size %d", list.Total, list.PageSize)
	}
}
