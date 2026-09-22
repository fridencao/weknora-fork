package config

import "github.com/Tencent/WeKnora/internal/types"

// RetrievalDefaults 把部署层的对话配置折算成检索参数集，作为**唯一缺省来源**
// （ADR-008 决策 2）。
//
// 背景：改造前检索参数有四个来源——部署 YAML、租户「检索设置」(RetrievalConfig)、
// 智能体「检索策略」、以及 types 里的内置默认。其中对话路径只读部署 YAML、
// 检索 API 路径只读租户配置，于是同一个请求里「要不要用图谱」与「rerank 阈值」
// 可能取自不同的源。租户层退休后，本函数是唯一的缺省入口，智能体再在其上做三态覆盖。
//
// 留 0 / nil 的字段刻意**不在此处补默认值**——由 types.RetrievalConfig 的
// GetEffective* 访问器回落，避免"默认值"在两个地方各写一份而再次漂移。
func (c *ConversationConfig) RetrievalDefaults() *types.RetrievalConfig {
	if c == nil {
		return &types.RetrievalConfig{}
	}
	return &types.RetrievalConfig{
		EmbeddingTopK:       c.EmbeddingTopK,
		VectorThreshold:     c.VectorThreshold,
		KeywordThreshold:    c.KeywordThreshold,
		RerankTopK:          c.RerankTopK,
		RerankThreshold:     c.RerankThreshold,
		RRFK:                c.RRFK,
		RRFVectorWeight:     c.RRFVectorWeight,
		RRFKeywordWeight:    c.RRFKeywordWeight,
		RRFGraphWeight:      c.RRFGraphWeight,
		GraphChannelEnabled: c.GraphChannelEnabled,
	}
}

// RetrievalDefaults 是调用方的便捷入口，nil 安全——服务层不必自己判空
// cfg / cfg.Conversation。
func (c *Config) RetrievalDefaults() *types.RetrievalConfig {
	if c == nil {
		return &types.RetrievalConfig{}
	}
	return c.Conversation.RetrievalDefaults()
}
