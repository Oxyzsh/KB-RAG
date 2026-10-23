package loader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSupported(t *testing.T) {
	cases := map[string]bool{
		"a.md": true, "a.MARKDOWN": true, "a.txt": true, "a.docx": true,
		"a.PDF": true, "a.doc": false, "a.png": false,
	}
	for name, want := range cases {
		if got := Supported(name); got != want {
			t.Errorf("Supported(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestLoadMarkdown(t *testing.T) {
	body := strings.Join([]string{
		"# 人事制度",
		"",
		"## 休假",
		"",
		"年假规定：员工累计工作满1年不满10年的，年休假5天。",
		"",
		"## 其他",
		"",
		"食堂中午开放。",
	}, "\n")
	path := writeFile(t, "制度.md", []byte(body))

	doc, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Format != "md" {
		t.Errorf("Format = %q", doc.Format)
	}
	if doc.Title != "人事制度" {
		t.Errorf("Title = %q", doc.Title)
	}
	want := []Block{
		{Level: 1, Text: "人事制度"},
		{Level: 2, Text: "休假"},
		{Text: "年假规定：员工累计工作满1年不满10年的，年休假5天。"},
		{Level: 2, Text: "其他"},
		{Text: "食堂中午开放。"},
	}
	if len(doc.Blocks) != len(want) {
		t.Fatalf("Blocks 数量 = %d, want %d: %+v", len(doc.Blocks), len(want), doc.Blocks)
	}
	for i := range want {
		if doc.Blocks[i] != want[i] {
			t.Errorf("Blocks[%d] = %+v, want %+v", i, doc.Blocks[i], want[i])
		}
	}
}

func TestLoadTXT(t *testing.T) {
	path := writeFile(t, "说明.txt", []byte("第一段内容。\r\n\r\n第二段内容。\r\n"))
	doc, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Format != "txt" || doc.Title != "说明" {
		t.Errorf("doc = %+v", doc)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("Blocks = %+v", doc.Blocks)
	}
	if doc.Blocks[0].Text != "第一段内容。" || doc.Blocks[1].Text != "第二段内容。" {
		t.Errorf("Blocks = %+v", doc.Blocks)
	}
}

func TestLoadGB18030(t *testing.T) {
	raw, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("年假规定：满一年五天。"))
	if err != nil {
		t.Fatal(err)
	}
	path := writeFile(t, "gbk.txt", raw)
	doc, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].Text != "年假规定：满一年五天。" {
		t.Errorf("Blocks = %+v", doc.Blocks)
	}
}

func TestLoadMarkdownBOM(t *testing.T) {
	path := writeFile(t, "bom.md", []byte("\uFEFF# 人事制度\n\n年假规定：满一年五天。\n"))
	doc, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Title != "人事制度" {
		t.Errorf("Title = %q", doc.Title)
	}
	if len(doc.Blocks) == 0 || doc.Blocks[0] != (Block{Level: 1, Text: "人事制度"}) {
		t.Errorf("Blocks = %+v", doc.Blocks)
	}
}

func TestUnsupportedFormat(t *testing.T) {
	path := writeFile(t, "a.doc", []byte("x"))
	if _, err := Load(path); err == nil {
		t.Fatal("期望不支持格式报错")
	}
}
