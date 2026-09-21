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
	"strings"
	"time"
)

// LightragChunkKeyLen 溯源键定长规则（docs/02 §7）：{doc36}-{chunk36} = 73 字符。
const LightragChunkKeyLen = 73

// ParseLightragChunkKey 将 source_id/chunk key 无损解析回 (docID, chunkID)。
func ParseLightragChunkKey(key string) (docID, chunkID string, err error) {
	if len(key) != LightragChunkKeyLen || key[36] != '-' {
		return "", "", fmt.Errorf("lightrag key 长度/格式非法: %q", key)
	}
	return key[:36], key[37:73], nil
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
