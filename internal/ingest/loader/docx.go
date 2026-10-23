package loader

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

type xmlNode struct {
	Name     xml.Name
	Attrs    []xml.Attr
	Text     string
	Children []*xmlNode
}

func parseXMLTree(r io.Reader) (*xmlNode, error) {
	decoder := xml.NewDecoder(r)
	var root *xmlNode
	var stack []*xmlNode
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			node := &xmlNode{Name: t.Name, Attrs: t.Attr}
			if len(stack) == 0 {
				root = node
			} else {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, node)
			}
			stack = append(stack, node)
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(t)
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if root == nil {
		return nil, fmt.Errorf("XML 为空")
	}
	return root, nil
}

func firstChild(n *xmlNode, local string) *xmlNode {
	for _, child := range n.Children {
		if child.Name.Local == local {
			return child
		}
	}
	return nil
}

func childrenOf(n *xmlNode, local string) []*xmlNode {
	var out []*xmlNode
	for _, child := range n.Children {
		if child.Name.Local == local {
			out = append(out, child)
		}
	}
	return out
}

func attrVal(n *xmlNode, local string) string {
	for _, attr := range n.Attrs {
		if attr.Name.Local == local {
			return attr.Value
		}
	}
	return ""
}

var docxHeadingStyle = regexp.MustCompile(`(?i)^(?:heading|标题)[\s_-]?([1-6])$`)

func headingLevel(style string) int {
	style = strings.TrimSpace(style)
	if style == "" {
		return 0
	}
	if strings.EqualFold(style, "Title") || style == "标题" {
		return 1
	}
	if m := docxHeadingStyle.FindStringSubmatch(style); m != nil {
		level, _ := strconv.Atoi(m[1])
		return level
	}
	// 中文 Word 文档中，标题样式 ID 常直接是 1-6。
	if len(style) == 1 && style[0] >= '1' && style[0] <= '6' {
		return int(style[0] - '0')
	}
	return 0
}

func collectRuns(n *xmlNode, b *strings.Builder) {
	for _, child := range n.Children {
		switch child.Name.Local {
		case "t":
			b.WriteString(child.Text)
		case "tab":
			b.WriteString("\t")
		case "br", "cr":
			b.WriteString("\n")
		case "del", "instrText":
			continue
		default:
			collectRuns(child, b)
		}
	}
}

func docxParagraph(p *xmlNode) (Block, bool) {
	level := 0
	if pPr := firstChild(p, "pPr"); pPr != nil {
		if outline := firstChild(pPr, "outlineLvl"); outline != nil {
			if v, err := strconv.Atoi(attrVal(outline, "val")); err == nil && v >= 0 && v <= 8 {
				level = v + 1
			}
		}
		if style := firstChild(pPr, "pStyle"); style != nil {
			if l := headingLevel(attrVal(style, "val")); l > 0 {
				level = l
			}
		}
	}
	var b strings.Builder
	collectRuns(p, &b)
	text := strings.TrimSpace(b.String())
	if text == "" {
		return Block{}, false
	}
	return Block{Level: level, Text: text}, true
}

func docxTable(t *xmlNode) Block {
	var rows []string
	for _, tr := range childrenOf(t, "tr") {
		var cells []string
		for _, tc := range childrenOf(tr, "tc") {
			var parts []string
			for _, p := range childrenOf(tc, "p") {
				if block, ok := docxParagraph(p); ok {
					parts = append(parts, block.Text)
				}
			}
			cells = append(cells, strings.Join(parts, " "))
		}
		if row := strings.TrimSpace(strings.Join(cells, " | ")); row != "" {
			rows = append(rows, row)
		}
	}
	return Block{Text: strings.Join(rows, "\n")}
}

func walkBody(n *xmlNode, blocks *[]Block) {
	for _, child := range n.Children {
		switch child.Name.Local {
		case "p":
			if block, ok := docxParagraph(child); ok {
				*blocks = append(*blocks, block)
			}
		case "tbl":
			if block := docxTable(child); block.Text != "" {
				*blocks = append(*blocks, block)
			}
		case "sdt":
			if content := firstChild(child, "sdtContent"); content != nil {
				walkBody(content, blocks)
			}
		case "body":
			walkBody(child, blocks)
		}
	}
}

func loadDOCX(path string) (Document, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return Document{}, fmt.Errorf("打开 DOCX: %w", err)
	}
	defer zr.Close()

	var documentFile *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			documentFile = f
			break
		}
	}
	if documentFile == nil {
		return Document{}, fmt.Errorf("DOCX 缺少 word/document.xml")
	}
	rc, err := documentFile.Open()
	if err != nil {
		return Document{}, fmt.Errorf("读取 word/document.xml: %w", err)
	}
	defer rc.Close()
	root, err := parseXMLTree(rc)
	if err != nil {
		return Document{}, fmt.Errorf("解析 word/document.xml: %w", err)
	}
	body := firstChild(root, "body")
	if body == nil {
		return Document{}, fmt.Errorf("DOCX 缺少 body")
	}
	var blocks []Block
	walkBody(body, &blocks)
	return Document{Path: path, Title: firstTitle(blocks, path), Format: "docx", Blocks: blocks}, nil
}
