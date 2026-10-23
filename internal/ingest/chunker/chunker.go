package chunker

import (
	"strings"
	"unicode/utf8"

	"kb-rag/internal/ingest/loader"
)

const minChunkRunes = 10

type Chunk struct {
	Seq     int
	Heading string
	Text    string
}

type heading struct {
	level int
	text  string
}

func Split(doc loader.Document, size, overlap int) []Chunk {
	if size <= 0 || overlap < 0 || overlap >= size {
		return nil
	}
	var chunks []Chunk
	var stack []heading
	var buf []string
	bufLen := 0

	path := func() string {
		parts := make([]string, 0, len(stack))
		for _, h := range stack {
			parts = append(parts, h.text)
		}
		return strings.Join(parts, " > ")
	}
	flush := func() {
		text := strings.TrimSpace(strings.Join(buf, "\n"))
		buf = buf[:0]
		bufLen = 0
		if utf8.RuneCountInString(text) < minChunkRunes {
			return
		}
		chunks = append(chunks, Chunk{Seq: len(chunks), Heading: path(), Text: text})
	}

	for _, block := range doc.Blocks {
		text := strings.TrimSpace(block.Text)
		if text == "" {
			continue
		}
		if block.Level > 0 {
			flush()
			for len(stack) > 0 && stack[len(stack)-1].level >= block.Level {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, heading{level: block.Level, text: text})
			continue
		}

		if header, rows, ok := parseTable(text); ok {
			flush()
			for _, piece := range splitTable(header, rows, size) {
				if utf8.RuneCountInString(piece) >= minChunkRunes {
					chunks = append(chunks, Chunk{Seq: len(chunks), Heading: path(), Text: piece})
				}
			}
			continue
		}

		n := utf8.RuneCountInString(text)
		if n > size {
			flush()
			for _, piece := range splitLong(text, size, overlap) {
				chunks = append(chunks, Chunk{Seq: len(chunks), Heading: path(), Text: piece})
			}
			continue
		}
		if bufLen > 0 && bufLen+1+n > size {
			flush()
		}
		if bufLen > 0 {
			bufLen++
		}
		buf = append(buf, text)
		bufLen += n
	}
	flush()
	return chunks
}

// parseTable 识别 Markdown 表格块：表头行 + 分隔行 + 至少一行数据。
// 只接受“整块都是表格”的情况，避免误吞表格前后的普通段落。
func parseTable(text string) (header, rows []string, ok bool) {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	if len(lines) < 3 {
		return nil, nil, false
	}
	if strings.TrimSpace(lines[0]) == "" || !strings.Contains(lines[0], "|") {
		return nil, nil, false
	}
	if !isTableSeparator(lines[1]) {
		return nil, nil, false
	}

	for i := 2; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" || !strings.Contains(line, "|") {
			for _, rest := range lines[i:] {
				if strings.TrimSpace(rest) != "" {
					return nil, nil, false
				}
			}
			break
		}
		rows = append(rows, line)
	}
	if len(rows) == 0 {
		return nil, nil, false
	}
	return []string{lines[0], lines[1]}, rows, true
}

func isTableSeparator(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.Contains(line, "|") {
		return false
	}
	for _, r := range line {
		switch r {
		case '|', '-', ':', ' ', '\t', '\r':
		default:
			return false
		}
	}
	return true
}

// splitTable 按行拆分超大表格，并在每个片段重复表头。
// 单行即使超过 size 也不拆开，保证表格结构不被破坏。
func splitTable(header, rows []string, size int) []string {
	headerText := strings.Join(header, "\n")
	headerLen := utf8.RuneCountInString(headerText)
	var out []string
	var current []string
	currentLen := headerLen

	flush := func() {
		if len(current) == 0 {
			return
		}
		out = append(out, headerText+"\n"+strings.Join(current, "\n"))
		current = nil
		currentLen = headerLen
	}
	for _, row := range rows {
		rowLen := utf8.RuneCountInString(row) + 1
		if len(current) > 0 && currentLen+rowLen > size {
			flush()
		}
		current = append(current, row)
		currentLen += rowLen
	}
	flush()
	return out
}

func splitLong(text string, size, overlap int) []string {
	runes := []rune(text)
	var out []string
	for start := 0; start < len(runes); {
		end := start + size
		if end > len(runes) {
			end = len(runes)
		}
		if end < len(runes) {
			if cut := lastSentenceCut(runes, start, end); cut > 0 {
				end = cut
			}
		}
		piece := strings.TrimSpace(string(runes[start:end]))
		if utf8.RuneCountInString(piece) >= minChunkRunes {
			out = append(out, piece)
		}
		if end >= len(runes) {
			break
		}
		next := end - overlap
		if next <= start {
			next = end
		}
		start = next
	}
	return out
}

func lastSentenceCut(runes []rune, start, end int) int {
	minCut := start + (end-start)/2
	for i := end - 1; i >= minCut; i-- {
		switch runes[i] {
		case '。', '！', '？', '；', '\n':
			return i + 1
		}
	}
	return 0
}
