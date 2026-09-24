package handler

// M6-1 WS1.5 · 覆盖率分母的 D3 豁免口径单测。
//
// 豁免判定必须与入队侧（knowledge_graph_ingest.go 的 FileName == "" 跳过分支）
// 用同一把尺子：尺子不一致时，粘贴类文档会被算成「未覆盖」，覆盖率永远到不了
// 100%，用户会以为建图坏了。

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

func TestCountGraphEligibleDocsExemptPasteDocs(t *testing.T) {
	docs := []*types.Knowledge{
		{ID: "doc-1", FileName: "年报.pdf"},
		{ID: "doc-2", FileName: "访谈.docx"},
		{ID: "paste-1", FileName: ""}, // 粘贴文本：无契约目录，从不进图谱
		{ID: "paste-2", FileName: ""}, // 同上
		nil,                           // 防御：列表里混入 nil 不崩
		{ID: "", FileName: "x.pdf"},   // 无 id 同样不可能进队列
	}
	total, exempt := countGraphEligibleDocs(docs)
	require.Equal(t, 6, total)
	require.Equal(t, 4, exempt, "粘贴类 2 + nil 1 + 无 id 1，都应豁免")
}

func TestCountGraphEligibleDocsEmptyList(t *testing.T) {
	total, exempt := countGraphEligibleDocs(nil)
	require.Equal(t, 0, total)
	require.Equal(t, 0, exempt)
}
