package loader

import (
	"strings"
	"testing"
)

func TestLoadPDFChinese(t *testing.T) {
	doc, err := Load("testdata/sample_zh.pdf")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Format != "pdf" || doc.Title != "sample_zh" {
		t.Errorf("doc = %+v", doc)
	}
	if len(doc.Blocks) == 0 || doc.Blocks[0] != (Block{Level: 1, Text: "第 1 页"}) {
		t.Fatalf("Blocks = %+v", doc.Blocks)
	}
	var all strings.Builder
	for _, b := range doc.Blocks {
		all.WriteString(b.Text)
	}
	text := all.String()
	for _, want := range []string{"人事制度 > 休假 > 年假", "年休假5天", "年假申请需提前三个工作日提交"} {
		if !strings.Contains(text, want) {
			t.Errorf("PDF 文本缺少 %q，实际为 %q", want, text)
		}
	}
}
