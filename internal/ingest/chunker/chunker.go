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
