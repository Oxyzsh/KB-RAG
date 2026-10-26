package web

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAsk(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	rec := uploadDoc(t, srv, token, "年假.md", []byte("# 休假\n\n员工累计工作满1年不满10年的，年休假5天。\n"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d", rec.Code)
	}

	body := []byte(`{"question":"年假怎么请","top_k":8}`)
	rec = call(t, srv.Handler(), "POST", "/api/ask", body, token, "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("ask status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp askResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Generated || resp.Answer == "" || len(resp.Citations) == 0 {
		t.Fatalf("ask = %+v", resp)
	}
	if resp.Citations[0].N != 1 || resp.Citations[0].Source != "年假.md" {
		t.Fatalf("citation = %+v", resp.Citations[0])
	}
}

func TestAskGenerationFailureReturnsCitations(t *testing.T) {
	srv, _, llmFake, token := newTestServer(t)
	rec := uploadDoc(t, srv, token, "年假.md", []byte("# 休假\n\n员工累计工作满1年可享5天年假。\n"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d", rec.Code)
	}
	llmFake.fail = true

	body := []byte(`{"question":"年假怎么请"}`)
	rec = call(t, srv.Handler(), "POST", "/api/ask", body, token, "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("ask status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp askResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Generated || resp.Warning == "" || len(resp.Citations) == 0 {
		t.Fatalf("ask = %+v", resp)
	}
}

func TestAskEmptyKnowledgeBase(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	body := []byte(`{"question":"年假怎么请"}`)
	rec := call(t, srv.Handler(), "POST", "/api/ask", body, token, "application/json")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAskValidation(t *testing.T) {
	srv, _, _, token := newTestServer(t)
	cases := [][]byte{
		[]byte(`{"question":"  "}`),
		[]byte(`{"question":"q","top_k":21}`),
		[]byte(`not-json`),
	}
	for _, body := range cases {
		rec := call(t, srv.Handler(), "POST", "/api/ask", body, token, "application/json")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body=%s status=%d", body, rec.Code)
		}
	}
}
