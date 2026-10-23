package loader

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

const sampleDocxXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>人事制度</w:t></w:r></w:p>
    <w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>休假</w:t></w:r></w:p>
    <w:p><w:pPr><w:outlineLvl w:val="2"/></w:pPr><w:r><w:t>年假条款</w:t></w:r></w:p>
    <w:p><w:r><w:t>正文第一段。</w:t></w:r><w:r><w:tab/><w:t>制表符</w:t></w:r></w:p>
    <w:tbl>
      <w:tr><w:tc><w:p><w:r><w:t>姓名</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>天数</w:t></w:r></w:p></w:tc></w:tr>
      <w:tr><w:tc><w:p><w:r><w:t>张三</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>5</w:t></w:r></w:p></w:tc></w:tr>
    </w:tbl>
  </w:body>
</w:document>`

func writeDOCX(t *testing.T, documentXML string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.docx")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, documentXML); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDOCX(t *testing.T) {
	doc, err := Load(writeDOCX(t, sampleDocxXML))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Format != "docx" || doc.Title != "人事制度" {
		t.Errorf("doc = %+v", doc)
	}
	if len(doc.Blocks) < 5 {
		t.Fatalf("Blocks = %+v", doc.Blocks)
	}
	if doc.Blocks[0] != (Block{Level: 1, Text: "人事制度"}) {
		t.Errorf("Blocks[0] = %+v", doc.Blocks[0])
	}
	if doc.Blocks[1] != (Block{Level: 2, Text: "休假"}) {
		t.Errorf("Blocks[1] = %+v", doc.Blocks[1])
	}
	if doc.Blocks[2] != (Block{Level: 3, Text: "年假条款"}) {
		t.Errorf("Blocks[2] = %+v", doc.Blocks[2])
	}
	if doc.Blocks[3].Text != "正文第一段。\t制表符" {
		t.Errorf("Blocks[3] = %q", doc.Blocks[3].Text)
	}
	if doc.Blocks[4].Text != "姓名 | 天数\n张三 | 5" {
		t.Errorf("表格 Blocks[4] = %q", doc.Blocks[4].Text)
	}
}

func TestLoadDOCXInvalidZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.docx")
	if err := os.WriteFile(path, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("期望损坏 DOCX 报错")
	}
}
