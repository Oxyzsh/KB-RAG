package chunker

import (
	"strings"
	"testing"

	"kb-rag/internal/ingest/loader"
)

func doc(blocks ...loader.Block) loader.Document {
	return loader.Document{Title: "test", Blocks: blocks}
}

func TestHeadingBoundaryAndPath(t *testing.T) {
	chunks := Split(doc(
		loader.Block{Level: 1, Text: "人事制度"},
		loader.Block{Level: 2, Text: "休假"},
		loader.Block{Text: "年假规定：员工累计工作满1年不满10年的，年休假5天。"},
		loader.Block{Level: 2, Text: "食堂"},
		loader.Block{Text: "食堂开放时间为中午十二点至下午一点。"},
	), 500, 80)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %+v", chunks)
	}
	if chunks[0].Heading != "人事制度 > 休假" {
		t.Errorf("Heading[0] = %q", chunks[0].Heading)
	}
	if chunks[1].Heading != "人事制度 > 食堂" {
		t.Errorf("Heading[1] = %q", chunks[1].Heading)
	}
	if strings.Contains(chunks[0].Text, "食堂") || strings.Contains(chunks[1].Text, "年假") {
		t.Errorf("片段跨越标题边界: %+v", chunks)
	}
	if chunks[0].Seq != 0 || chunks[1].Seq != 1 {
		t.Errorf("Seq 不连续: %+v", chunks)
	}
}

func TestPackingParagraphs(t *testing.T) {
	chunks := Split(doc(
		loader.Block{Text: strings.Repeat("甲", 60) + "。"},
		loader.Block{Text: strings.Repeat("乙", 60) + "。"},
	), 500, 80)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %+v", chunks)
	}
}

func TestLongParagraphSizeAndOverlap(t *testing.T) {
	text := strings.Repeat("甲乙丙丁戊己庚辛壬癸。", 80)
	chunks := Split(doc(loader.Block{Text: text}), 100, 20)
	if len(chunks) < 2 {
		t.Fatalf("期望多个片段，实际 %+v", chunks)
	}
	for _, c := range chunks {
		if n := len([]rune(c.Text)); n > 100 {
			t.Errorf("片段长度 %d 超过上限: %q", n, c.Text)
		}
	}
	for i := 0; i+1 < len(chunks); i++ {
		prev := []rune(chunks[i].Text)
		next := []rune(chunks[i+1].Text)
		if len(prev) < 20 || len(next) < 20 {
			continue
		}
		if string(prev[len(prev)-20:]) != string(next[:20]) {
			t.Errorf("片段 %d 与 %d 的重叠不是 20 字符:\n%q\n%q", i, i+1, chunks[i].Text, chunks[i+1].Text)
		}
	}
}

func TestDropFragments(t *testing.T) {
	chunks := Split(doc(loader.Block{Text: "短"}), 500, 80)
	if len(chunks) != 0 {
		t.Fatalf("碎片未被丢弃: %+v", chunks)
	}
}

func TestInvalidParams(t *testing.T) {
	if chunks := Split(doc(loader.Block{Text: "足够长的内容内容内容。"}), 100, 100); len(chunks) != 0 {
		t.Fatalf("非法参数应返回空: %+v", chunks)
	}
}
