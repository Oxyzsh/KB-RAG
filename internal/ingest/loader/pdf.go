package loader

import (
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

func loadPDF(path string) (Document, error) {
	f, reader, err := pdf.Open(path)
	if err != nil {
		return Document{}, fmt.Errorf("打开 PDF: %w", err)
	}
	defer f.Close()

	var blocks []Block
	for i := 1; i <= reader.NumPage(); i++ {
		pageText, err := reader.Page(i).GetPlainText(nil)
		if err != nil {
			return Document{}, fmt.Errorf("提取 PDF 第 %d 页失败: %w", i, err)
		}
		pageText = strings.TrimSpace(strings.ReplaceAll(pageText, "\r\n", "\n"))
		if pageText == "" {
			continue
		}
		blocks = append(blocks, Block{Level: 1, Text: fmt.Sprintf("第 %d 页", i)})
		for _, paragraph := range splitPlainText(pageText) {
			blocks = append(blocks, Block{Text: paragraph})
		}
	}
	if len(blocks) == 0 {
		return Document{}, fmt.Errorf("PDF 未提取到文本（可能是扫描件，第一期不支持 OCR）")
	}
	// PDF 没有可靠标题，按设计使用文件名，页码只作为切分结构标记。
	return Document{Path: path, Title: fallbackTitle(path), Format: "pdf", Blocks: blocks}, nil
}
