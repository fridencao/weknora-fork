package chatpipeline

// A0 修复（docs/08）：图谱证据回跳（LightRAG 证据 chunk → WeKnora 检索 chunk）。
//
// 问题实证（2026-09-22，data/processed/lightrag_a3）：
//  1. LightRAG 1.5.8 真实产出的 chunk key 是 `{docID}-chunk-NNN`（46 字符），
//     其中 docID = WeKnora knowledge doc id，NNN = 图谱侧该文档内序号；
//     而 docs/02 §7 设计的定长 `{doc36}-{chunk36}`（73 字符）从未被真实产出。
//     旧解析器按 73 字符强校验 → 真实数据恒失败 → chunkIDs 恒空 → 图谱通道
//     召回 0 条（三通道 RRF 静默退化为二通道）。
//  2. 两侧 chunk 粒度差约 16 倍（图谱 chunk ≈5k 字符 / WeKnora 子 chunk ≈308 字符），
//     同文档实测 10 vs 181，故**无法按序号映射**。
//
// 回跳口径：图谱 chunk 正文内嵌契约锚点 `<!--sbk:pXXX-bXXX-->`（建图素材取自契约层
// document.md），与 WeKnora chunk 的 `metadata.sbk_blocks[].block_id` 同源
// （contract/src/starkb_contract/alignment.py T1 对齐产物）。据此定位该图谱 chunk
// 覆盖的 WeKnora 子 chunk。
//
// 粒度选择：返回 chunk_type=text 的子 chunk——与向量/关键词通道的召回粒度一致
// （parent_text「仅用于上下文，不参与向量索引」），RRF 融合与父块扩展才能对齐。

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// sbkAnchorRe 匹配契约锚点标记。block_id 形如 p001-b004，此处放宽字符集以兼容
// 契约层后续可能的编号方案。
var sbkAnchorRe = regexp.MustCompile(`<!--sbk:([^>]+)-->`)

// GraphEvidenceRef 一个图谱证据 chunk 及其可回跳范围。
type GraphEvidenceRef struct {
	// Key LightRAG chunk key（{docID}-chunk-NNN），用于日志与去重。
	Key string
	// DocID WeKnora knowledge doc id（key 前段），KB 权限过滤依据。
	DocID string
	// BlockIDs 正文契约锚点（保序去重）。空 = 无法定位到具体位置。
	BlockIDs []string
}

// ParseGraphChunkAnchors 从图谱 chunk 正文解析契约锚点（保序去重）。
func ParseGraphChunkAnchors(content string) []string {
	if content == "" {
		return nil
	}
	matches := sbkAnchorRe.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, m := range matches {
		id := m[1]
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// StripGraphChunkAnchors 把契约锚点注释从正文剥掉（P1-6：图谱 chunk 原文要
// 作为「为什么抽到这条关系」的上下文展示给用户，`<!--sbk:…-->` 是机器定位
// 标记，对人只有噪音）。保序保留其余文本。
func StripGraphChunkAnchors(content string) string {
	if !strings.Contains(content, "<!--sbk:") {
		return content
	}
	return strings.TrimSpace(sbkAnchorRe.ReplaceAllString(content, ""))
}

// CollectGraphEvidence 把 /query/data 的证据 chunk 解析为可回跳引用。
// allowed 为 KB 范围内允许回跳的文档 ID 集合；范围外的证据直接丢弃。
func CollectGraphEvidence(data *LightragQueryData, allowed map[string]struct{}) []GraphEvidenceRef {
	if data == nil || len(data.Data.Chunks) == 0 || len(allowed) == 0 {
		return nil
	}
	refs := make([]GraphEvidenceRef, 0, len(data.Data.Chunks))
	seenKey := make(map[string]struct{}, len(data.Data.Chunks))
	for _, c := range data.Data.Chunks {
		docID, _, err := ParseLightragChunkKey(c.ChunkID)
		if err != nil {
			continue
		}
		if _, ok := allowed[docID]; !ok {
			continue
		}
		if _, dup := seenKey[c.ChunkID]; dup {
			continue
		}
		seenKey[c.ChunkID] = struct{}{}
		refs = append(refs, GraphEvidenceRef{
			Key:      c.ChunkID,
			DocID:    docID,
			BlockIDs: ParseGraphChunkAnchors(c.Content),
		})
	}
	return refs
}

// sbkBlockMeta WeKnora chunk metadata 中的契约对齐信息（contract T1 产物）。
type sbkBlockMeta struct {
	BlockID  string  `json:"block_id"`
	Coverage float64 `json:"coverage"`
	Role     string  `json:"role"`
}

type sbkChunkMeta struct {
	Blocks []sbkBlockMeta `json:"sbk_blocks"`
}

// ResolveGraphEvidence 把图谱证据回跳为 WeKnora 子 chunk（chunk_type=text）。
// 返回值二 sourceByChunk：子 chunk ID → 产出它的图谱 chunk key（P1-6 证据原文
// 上下文跟随展示用）；不需要的调用方用 `_` 忽略。
//
// 排序与配额：单个图谱 chunk 平均覆盖 ~11 个子 chunk，若全量返回会淹没向量/关键词
// 通道（图谱权重最低但条数最多，RRF 会失衡）。因此按「轮转」取用——先保证每个图谱
// 命中都贡献 1 条（顺序即 LightRAG 的相关性顺序，直接决定 RRF 名次），再取第 2 条，
// 以此类推；单命中至多 perHit 条，整体至多 total 条。
//
// 命中内部按「匹配块的 coverage 降序」排：coverage = 交集长/chunk 长，越高说明该
// block 越主导这个子 chunk，越适合作为该位置的证据代表。
//
// 无锚点的图谱 chunk（实测 173 条中 5 条，多为纯表格/图片区域）无法定位，跳过并计数。
func ResolveGraphEvidence(
	ctx context.Context,
	repo interfaces.ChunkRepository,
	tenantID uint64,
	refs []GraphEvidenceRef,
	perHit, total int,
) ([]*types.Chunk, map[string]string, error) {
	if len(refs) == 0 || repo == nil {
		return nil, nil, nil
	}
	if perHit <= 0 {
		perHit = DefaultGraphChunksPerHit
	}
	if total <= 0 {
		total = DefaultGraphChunkTotal
	}

	// 按文档分组：同一文档只查一次全量子 chunk，再在内存里做锚点求交。
	byDoc := make(map[string][]int, len(refs))
	docOrder := make([]string, 0, len(refs))
	for i, r := range refs {
		if _, ok := byDoc[r.DocID]; !ok {
			docOrder = append(docOrder, r.DocID)
		}
		byDoc[r.DocID] = append(byDoc[r.DocID], i)
	}

	// 每篇文档：block_id → 覆盖它的子 chunk（含 coverage，用于排序）
	docIndex := make(map[string]map[string][]scoredChunk, len(byDoc))
	for _, docID := range docOrder {
		chunks, err := repo.ListChunksByKnowledgeIDAndTypes(
			ctx, tenantID, docID, []types.ChunkType{types.ChunkTypeText})
		if err != nil {
			// 单篇失败仅跳过该篇（降级，不阻断通道）
			logger.Warnf(ctx, "graph 回跳: 列文档 %s 子 chunk 失败（跳过）: %v", docID, err)
			continue
		}
		idx := make(map[string][]scoredChunk, len(chunks))
		for _, c := range chunks {
			if c == nil || len(c.Metadata) == 0 {
				continue
			}
			var meta sbkChunkMeta
			if err := json.Unmarshal(c.Metadata, &meta); err != nil {
				continue
			}
			for _, b := range meta.Blocks {
				if b.BlockID == "" {
					continue
				}
				idx[b.BlockID] = append(idx[b.BlockID], scoredChunk{chunk: c, coverage: b.Coverage})
			}
		}
		docIndex[docID] = idx
	}

	// 逐命中解析候选（去重 + 排序），保持 refs 顺序即图谱相关性顺序。
	hits := make([][]*types.Chunk, 0, len(refs))
	noAnchor := 0
	for _, r := range refs {
		idx, ok := docIndex[r.DocID]
		if !ok || len(r.BlockIDs) == 0 {
			if len(r.BlockIDs) == 0 {
				noAnchor++
			}
			hits = append(hits, nil)
			continue
		}
		best := make(map[string]scoredChunk)
		for _, bid := range r.BlockIDs {
			for _, sc := range idx[bid] {
				cur, exists := best[sc.chunk.ID]
				if !exists || sc.coverage > cur.coverage {
					best[sc.chunk.ID] = sc
				}
			}
		}
		if len(best) == 0 {
			hits = append(hits, nil)
			continue
		}
		cands := make([]scoredChunk, 0, len(best))
		for _, sc := range best {
			cands = append(cands, sc)
		}
		sort.SliceStable(cands, func(i, j int) bool {
			if cands[i].coverage != cands[j].coverage {
				return cands[i].coverage > cands[j].coverage
			}
			return cands[i].chunk.ChunkIndex < cands[j].chunk.ChunkIndex
		})
		ordered := make([]*types.Chunk, 0, len(cands))
		for _, sc := range cands {
			ordered = append(ordered, sc.chunk)
		}
		hits = append(hits, ordered)
	}

	// 轮转取用，全局去重。sourceByChunk 记录每个子 chunk 由哪条图谱证据
	// （refs 下标对应的 Key）选出——P1-6 图谱原文上下文要跟随证据展示。
	result := make([]*types.Chunk, 0, total)
	sourceByChunk := make(map[string]string, total)
	seen := make(map[string]struct{}, total)
	for round := 0; round < perHit && len(result) < total; round++ {
		progressed := false
		for ri, cands := range hits {
			if len(result) >= total {
				break
			}
			if round >= len(cands) {
				continue
			}
			c := cands[round]
			if _, dup := seen[c.ID]; dup {
				continue
			}
			seen[c.ID] = struct{}{}
			result = append(result, c)
			sourceByChunk[c.ID] = refs[ri].Key
			progressed = true
		}
		if !progressed {
			break
		}
	}
	if noAnchor > 0 {
		logger.Infof(ctx, "graph 回跳: %d/%d 条图谱证据无契约锚点，无法定位（已跳过）",
			noAnchor, len(refs))
	}
	return result, sourceByChunk, nil
}

// scoredChunk 子 chunk 及其对某个契约块的覆盖度。
type scoredChunk struct {
	chunk    *types.Chunk
	coverage float64
}

// 图谱通道配额默认值：单命中 2 条、整体 20 条（与向量/关键词通道量级对齐）。
// 可用 GRAPH_CHANNEL_CHUNKS_PER_HIT 覆盖单命中配额。
const (
	DefaultGraphChunksPerHit = 2
	DefaultGraphChunkTotal   = 20
)
