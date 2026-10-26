package web

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestRoleMatrix(t *testing.T) {
	env := newTestServer(t)
	rec := uploadDoc(t, env, "editor1", "年假.md", []byte("# 休假\n\n员工满1年享5天年假。\n"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload = %d", rec.Code)
	}
	var up uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	docPath := fmtID("/api/documents/%d", up.Document.ID)
	reingestPath := docPath + "/reingest"

	cases := []struct {
		name   string
		who    string
		method string
		path   string
		want   int
	}{
		{"anonymous stats", "", "GET", "/api/stats", http.StatusUnauthorized},
		{"viewer stats", "viewer1", "GET", "/api/stats", http.StatusOK},
		{"viewer documents", "viewer1", "GET", "/api/documents", http.StatusOK},
		{"viewer upload", "viewer1", "POST", "/api/documents", http.StatusForbidden},
		{"viewer delete", "viewer1", "DELETE", docPath, http.StatusForbidden},
		{"viewer reingest", "viewer1", "POST", reingestPath, http.StatusForbidden},
		{"viewer users", "viewer1", "GET", "/api/users", http.StatusForbidden},
		{"viewer audit", "viewer1", "GET", "/api/audit", http.StatusForbidden},
		{"viewer tenants", "viewer1", "GET", "/api/tenants", http.StatusForbidden},
		{"editor users", "editor1", "GET", "/api/users", http.StatusForbidden},
		{"editor audit", "editor1", "GET", "/api/audit", http.StatusForbidden},
		{"editor tenants", "editor1", "GET", "/api/tenants", http.StatusForbidden},
		{"admin users", "admin1", "GET", "/api/users", http.StatusOK},
		{"admin audit", "admin1", "GET", "/api/audit", http.StatusOK},
		{"admin tenants", "admin1", "GET", "/api/tenants", http.StatusOK},
		{"admin create tenant", "admin1", "POST", "/api/tenants", http.StatusForbidden},
		{"super tenants", "root", "GET", "/api/tenants", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := env.callAs(t, tc.who, tc.method, tc.path, nil, "")
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}
