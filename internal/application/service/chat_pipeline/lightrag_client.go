package chatpipeline

// LightRAG 图谱服务客户端（M2，ADR-005/docs03）。
// 部署约定：一个 LightRAG 实例服务一个图谱空间（KB），BASE_URL 经环境变量注入；
// 多图谱空间由部署层（每 KB 一实例）路由，M3 再收敛为网关。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// LightragChunkKeyLen docs/02 §7 的溯源键定长设计：{doc36}-{chunk36} = 73 字符。
//
// 保留仅为兼容历史数据与单测构造——真实 LightRAG（1.5.8）从未产出该格式，
// 其实际 key 为 `{docID}-chunk-NNN`，见 ParseLightragChunkKey。
const LightragChunkKeyLen = 73

// lightragGraphChunkKeyRe LightRAG 真实产出的 chunk key：{docID}-chunk-{NNN}。
var lightragGraphChunkKeyRe = regexp.MustCompile(`^(.+)-chunk-(\d+)$`)

// ParseLightragChunkKey 把 LightRAG 证据 chunk key 解析为 (docID, graphChunkKey)。
//
// docID = WeKnora knowledge doc id（建图时以知识文档 ID 入队，KB 权限过滤依据）。
// graphChunkKey = LightRAG 侧 chunk 标识，**不是** WeKnora chunk id：两侧分块粒度
// 相差约 16 倍（图谱 chunk ≈5k 字符 / WeKnora 子 chunk ≈308 字符），按序号映射不可行，
// 回跳必须走正文契约锚点（见 graph_anchor.go）。
//
// 历史缺陷（A0，docs/08）：旧实现按 73 字符强校验 `{doc36}-{chunk36}`，真实数据
// 恒解析失败 → 证据 chunk 全部被丢弃 → 图谱通道静默召回 0 条。
func ParseLightragChunkKey(key string) (docID, graphChunkKey string, err error) {
	if m := lightragGraphChunkKeyRe.FindStringSubmatch(key); m != nil {
		return m[1], key, nil
	}
	// 兼容 docs/02 §7 定长设计（历史数据/单测构造）
	if len(key) == LightragChunkKeyLen && key[36] == '-' {
		return key[:36], key, nil
	}
	return "", "", fmt.Errorf("lightrag key 格式非法: %q", key)
}

// LightragClient LightRAG 服务查询客户端（OpenAI 兼容部署，鉴权用静态 API Key）。
type LightragClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewLightragClientFromEnv 从环境变量构造：LIGHT_RAG_BASE_URL / LIGHT_RAG_API_KEY。
func NewLightragClientFromEnv() *LightragClient {
	return &LightragClient{
		baseURL: strings.TrimRight(os.Getenv("LIGHT_RAG_BASE_URL"), "/"),
		apiKey:  os.Getenv("LIGHT_RAG_API_KEY"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// BaseURL 返回服务地址（空 = 未配置，调用方可据此降级跳过图谱通道）。
func (c *LightragClient) BaseURL() string { return c.baseURL }

// LightragQueryData /query/data 响应（c7e7a24 实测：外层包 data）。
type LightragQueryData struct {
	Data struct {
		Entities []struct {
			EntityName  string `json:"entity_name"`
			Description string `json:"description"`
			SourceID    string `json:"source_id"`
		} `json:"entities"`
		Relationships []struct {
			SourceID    string `json:"source_id"`
			TargetID    string `json:"target_id"`
			Description string `json:"description"`
		} `json:"relationships"`
		Chunks []struct {
			ChunkID  string `json:"chunk_id"`
			Content  string `json:"content"`
			FilePath string `json:"file_path"`
		} `json:"chunks"`
	} `json:"data"`
	Status string `json:"status"`
}

// QueryData 调用 LightRAG /query/data（mode=mix：KG + 向量证据召回）。
func (c *LightragClient) QueryData(ctx context.Context, query string, topK int) (*LightragQueryData, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("LIGHT_RAG_BASE_URL 未配置")
	}
	body := map[string]any{"query": query, "mode": "mix", "top_k": topK}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/query/data", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lightrag query/data HTTP %d", resp.StatusCode)
	}
	out := &LightragQueryData{}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, err
	}
	return out, nil
}
