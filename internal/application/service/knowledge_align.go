package service

// M3 补遗（docs/07 WS1.3 收尾）：入库后自动溯源对齐。
//
// starkb 引擎解析时契约目录（doc_id）留在 starkb-api 侧，WeKnora 只拿到
// markdown 文本——目录名在 docreader→Go 之间丢失，因此 post-process 阶段
// 把 file_name + chunk 清单发给 starkb-api 的 /align/knowledge，由它反查
// 自己的 jobs 表拿契约目录并对齐，返回 per-chunk 的 sbk_* 元数据，本侧合并
// 进 chunk.Metadata 批量写回。对齐失败只记日志，绝不阻断入库主链路。
//
// 开关：STARKB_ALIGN_ON_INGEST=true 且 STARKB_API_URL 已配置。

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

const alignProvenanceTimeout = 120 * time.Second

type alignProvenanceRequest struct {
	KnowledgeID string                   `json:"knowledge_id"`
	FileName    string                   `json:"file_name"`
	Chunks      []alignProvenanceChunkIn `json:"chunks"`
}

type alignProvenanceChunkIn struct {
	ID      string `json:"chunk_id"`
	Content string `json:"content"`
	StartAt int    `json:"start_at"`
	EndAt   int    `json:"end_at"`
}

type alignProvenanceResult struct {
	ChunkID  string         `json:"chunk_id"`
	Metadata map[string]any `json:"metadata"`
}

type alignProvenanceResponse struct {
	ContractDir string                  `json:"contract_dir"`
	Total       int                     `json:"total"`
	Failed      int                     `json:"failed"`
	Results     []alignProvenanceResult `json:"results"`
}

// alignOnIngestEnabled 解析入库对齐开关（starkb.align_on_ingest 系统设置，
// DB > ENV > 默认）。STARKB_API_URL 仍属基础设施连接，留在环境变量。
func (s *KnowledgePostProcessService) alignOnIngestEnabled(ctx context.Context) bool {
	if os.Getenv("STARKB_API_URL") == "" || s.settings == nil {
		return false
	}
	return s.settings.GetBool(ctx,
		types.SettingKeyStarkbAlignOnIngest, types.SettingEnvStarkbAlignOnIngest, true)
}

// AlignProvenanceOnIngest 在 post-process 阶段对 starkb 引擎解析的文档执行
// 溯源对齐并把锚点合并写回 chunk metadata。knowledge.FileName 为空（无文件
// 语义的知识）或服务未配置时为 no-op。
func (s *KnowledgePostProcessService) AlignProvenanceOnIngest(ctx context.Context,
	chunkRepo interfaces.ChunkRepository,
	tenantID uint64, knowledge *types.Knowledge, chunks []*types.Chunk,
) {
	if !s.alignOnIngestEnabled(ctx) || knowledge == nil || knowledge.FileName == "" || len(chunks) == 0 {
		return
	}
	req := alignProvenanceRequest{
		KnowledgeID: knowledge.ID,
		FileName:    knowledge.FileName,
		Chunks:      make([]alignProvenanceChunkIn, 0, len(chunks)),
	}
	byID := make(map[string]*types.Chunk, len(chunks))
	for _, c := range chunks {
		if c == nil {
			continue
		}
		byID[c.ID] = c
		req.Chunks = append(req.Chunks, alignProvenanceChunkIn{
			ID: c.ID, Content: c.Content, StartAt: int(c.StartAt), EndAt: int(c.EndAt),
		})
	}
	if len(req.Chunks) == 0 {
		return
	}

	body, err := json.Marshal(req)
	if err != nil {
		logger.Warnf(ctx, "align on ingest: marshal request: %v", err)
		return
	}
	url := os.Getenv("STARKB_API_URL") + "/align/knowledge"
	client := &http.Client{Timeout: alignProvenanceTimeout}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		logger.Warnf(ctx, "align on ingest: 调用 starkb-api 失败（跳过）: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logger.Warnf(ctx, "align on ingest: starkb-api 返回 %d（跳过）", resp.StatusCode)
		return
	}
	var out alignProvenanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		logger.Warnf(ctx, "align on ingest: 解析响应失败: %v", err)
		return
	}

	updated := make([]*types.Chunk, 0, len(out.Results))
	for _, r := range out.Results {
		chunk := byID[r.ChunkID]
		if chunk == nil || len(r.Metadata) == 0 {
			continue
		}
		merged := map[string]any{}
		if len(chunk.Metadata) > 0 {
			_ = json.Unmarshal(chunk.Metadata, &merged)
		}
		for k, v := range r.Metadata {
			merged[k] = v
		}
		raw, err := json.Marshal(merged)
		if err != nil {
			continue
		}
		chunk.Metadata = types.JSON(raw)
		updated = append(updated, chunk)
	}
	if len(updated) > 0 {
		if err := chunkRepo.UpdateChunks(ctx, updated); err != nil {
			logger.Warnf(ctx, "align on ingest: 写回 chunk metadata 失败: %v", err)
			return
		}
	}
	logger.Infof(ctx, "align on ingest: knowledge %s 对齐 %d/%d chunk（失败 %d，契约 %s）",
		knowledge.ID, len(updated), out.Total, out.Failed, out.ContractDir)
}
