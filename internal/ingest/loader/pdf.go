package loader

import (
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

func loadPDF(path string) (doc Document, err error) {
	// 纯 Go PDF 库对畸形文件可能 panic；单个文件异常不得终止整个 ingest。
	defer func() {
		if r := recover(); r != nil {
			doc = Document{}
			err = fmt.Errorf("解析 PDF 发生异常: %v", r)
		}
	}()

	f, reader, err := pdf.Open(path)
	if err != nil {
		return Document{}, fmt.Errorf("打开 PDF: %w", err)
	}
	defer f.Close()

	var blocks []Block
	for i := 1; i <= reader.NumPage(); i++ {
		pageText, extractErr := reader.Page(i).GetPlainText(nil)
		if extractErr != nil {
			return Document{}, fmt.Errorf("提取 PDF 第 %d 页失败: %w", i, extractErr)
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
