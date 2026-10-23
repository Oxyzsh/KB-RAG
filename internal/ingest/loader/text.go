package loader

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"golang.org/x/text/encoding/simplifiedchinese"
)

var blankLine = regexp.MustCompile(`\n[ \t]*\n`)

func decodeTextFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取文件: %w", err)
	}
	if utf8.Valid(raw) {
		return strings.TrimPrefix(string(raw), "\uFEFF"), nil
	}
	decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(raw)
	if err != nil {
		return "", fmt.Errorf("文本编码既不是 UTF-8 也不是 GB18030: %w", err)
	}
	return strings.TrimPrefix(string(decoded), "\uFEFF"), nil
}

func splitPlainText(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var out []string
	for _, part := range blankLine.Split(s, -1) {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func loadText(path string) (Document, error) {
	content, err := decodeTextFile(path)
	if err != nil {
		return Document{}, err
	}
	var blocks []Block
	for _, p := range splitPlainText(content) {
		blocks = append(blocks, Block{Text: p})
	}
	return Document{Path: path, Title: fallbackTitle(path), Format: "txt", Blocks: blocks}, nil
}

func loadMarkdown(path string) (Document, error) {
	content, err := decodeTextFile(path)
	if err != nil {
		return Document{}, err
	}
	src := []byte(content)
	root := goldmark.New().Parser().Parse(text.NewReader(src))

	var blocks []Block
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Heading:
			if title := strings.TrimSpace(string(node.Text(src))); title != "" {
				blocks = append(blocks, Block{Level: node.Level, Text: title})
			}
			return ast.WalkSkipChildren, nil
		case *ast.Paragraph, *ast.TextBlock:
			if body := strings.TrimSpace(string(n.Text(src))); body != "" {
				blocks = append(blocks, Block{Text: body})
			}
			return ast.WalkSkipChildren, nil
		case *ast.FencedCodeBlock:
			blocks = appendCodeBlock(blocks, node.Lines(), src)
			return ast.WalkSkipChildren, nil
		case *ast.CodeBlock:
			blocks = appendCodeBlock(blocks, node.Lines(), src)
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return Document{Path: path, Title: firstTitle(blocks, path), Format: "md", Blocks: blocks}, nil
}

func appendCodeBlock(blocks []Block, lines *text.Segments, src []byte) []Block {
	if lines == nil || lines.Len() == 0 {
		return blocks
	}
	var b strings.Builder
	for i := 0; i < lines.Len(); i++ {
		segment := lines.At(i)
		b.Write(segment.Value(src))
	}
	if code := strings.TrimSpace(b.String()); code != "" {
		blocks = append(blocks, Block{Text: code})
	}
	return blocks
}
