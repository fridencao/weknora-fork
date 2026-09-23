package types

// StarKB 配置治理批一：迁入 system_settings 的键名与旧环境变量名。
//
// 集中定义的理由：注册表（internal/application/service）与消费方
// （chat_pipeline / handler / config）分属不同包，而 chat_pipeline
// 不能反向导入 service（会成环）。常量放在 types —— 两边都已依赖它。
//
// 每个键的语义、默认值与约束见 service.registry 中的 Description。
const (
	// SettingKeyFusionReliabilityWeight 可靠度因子权重（ADR-005：下限 0.25）。
	// 迁移前由 STARKB_RELIABILITY_WEIGHT 环境变量提供，且绕过了下限锁定。
	SettingKeyFusionReliabilityWeight = "fusion.reliability.weight"
	SettingEnvFusionReliabilityWeight = "STARKB_RELIABILITY_WEIGHT"

	// SettingKeyGraphChannelEnabled 图谱通道部署级默认开关。
	SettingKeyGraphChannelEnabled = "graph.channel.enabled"
	SettingEnvGraphChannelEnabled = "GRAPH_CHANNEL_ENABLED"

	// SettingKeyGraphChannelTopK 图谱通道返回条数上限。
	SettingKeyGraphChannelTopK = "graph.channel.top_k"
	SettingEnvGraphChannelTopK = "GRAPH_CHANNEL_TOP_K"

	// SettingKeyGraphChannelChunksPerHit 每个命中实体附带的证据 chunk 数。
	SettingKeyGraphChannelChunksPerHit = "graph.channel.chunks_per_hit"
	SettingEnvGraphChannelChunksPerHit = "GRAPH_CHANNEL_CHUNKS_PER_HIT"

	// SettingKeyGraphChannelTimeoutS 图谱通道软超时（秒，1–120）。
	SettingKeyGraphChannelTimeoutS = "graph.channel.timeout_s"
	SettingEnvGraphChannelTimeoutS = "GRAPH_CHANNEL_TIMEOUT_S"

	// SettingKeyStarkbClaimGate 答案级论断审计总开关。
	SettingKeyStarkbClaimGate = "starkb.claim_gate"
	SettingEnvStarkbClaimGate = "STARKB_CLAIM_GATE"

	// SettingKeyStarkbRequireProvenance 强绑定溯源门禁（无溯源则剥离）。
	SettingKeyStarkbRequireProvenance = "starkb.require_provenance"
	SettingEnvStarkbRequireProvenance = "STARKB_REQUIRE_PROVENANCE"

	// SettingKeyStarkbAlignOnIngest 入库后自动做 T1 溯源对齐。
	SettingKeyStarkbAlignOnIngest = "starkb.align_on_ingest"
	SettingEnvStarkbAlignOnIngest = "STARKB_ALIGN_ON_INGEST"

	// SettingKeyStarkbGraphOnIngest 入库后自动投喂图谱建图。
	SettingKeyStarkbGraphOnIngest = "starkb.graph_on_ingest"
	SettingEnvStarkbGraphOnIngest = "STARKB_GRAPH_ON_INGEST"

	// SettingKeyStarkbGraphCleanupOnDelete 删除知识时同步清理图谱数据。
	SettingKeyStarkbGraphCleanupOnDelete = "starkb.graph_cleanup_on_delete"
	SettingEnvStarkbGraphCleanupOnDelete = "STARKB_GRAPH_CLEANUP_ON_DELETE"

	// SettingKeyAgentLLMTimeout 单次 LLM 调用默认超时（Go duration 或秒数）。
	SettingKeyAgentLLMTimeout = "agent.llm_timeout"
	SettingEnvAgentLLMTimeout = "WEKNORA_AGENT_LLM_TIMEOUT"

	// SettingKeyAgentToolApprovalTimeout MCP 高风险工具人工审批等待时长。
	SettingKeyAgentToolApprovalTimeout = "agent.tool_approval_timeout"
	SettingEnvAgentToolApprovalTimeout = "WEKNORA_AGENT_TOOL_APPROVAL_TIMEOUT"

	// SettingKeyAgentToolApprovalFailOpen 审批不可用时是否放行（默认 fail-closed）。
	SettingKeyAgentToolApprovalFailOpen = "agent.tool_approval_fail_open"
	SettingEnvAgentToolApprovalFailOpen = "WEKNORA_AGENT_TOOL_APPROVAL_FAIL_OPEN"

	// SettingKeyDocreaderCallTimeout 调用 docreader 的单次超时。
	SettingKeyDocreaderCallTimeout = "docreader.call_timeout"
	SettingEnvDocreaderCallTimeout = "WEKNORA_DOCREADER_CALL_TIMEOUT"

	// SettingKeyDocumentProcessTimeout 单个文档入库到解析完成的总超时。
	SettingKeyDocumentProcessTimeout = "document.process_timeout"
	SettingEnvDocumentProcessTimeout = "WEKNORA_DOCUMENT_PROCESS_TIMEOUT"

	// SettingKeyTenantEnableRBAC 空间级 RBAC 鉴权开关。
	//
	// 与批一其余键的差别：它的消费方（中间件 / 路由）读的是
	// *config.Config 单例上的字段，而不是在调用点解析。LoadConfig 阶段 DB
	// 尚不可达，因此由 cmd/server/bootstrap.go 的启动钩子在「迁移完成、
	// 监听端口之前」同步写入 cfg。改动需重启进程方可生效。
	//
	// 内置默认 true：TenantConfig.IsRBACEnforced() 对 nil 指针返回 true，
	// applyAuthAndTenantDefaults 亦落 true（仅建议单机私有化部署时关掉）。
	SettingKeyTenantEnableRBAC = "tenant.enable_rbac"
	SettingEnvTenantEnableRBAC = "WEKNORA_TENANT_ENABLE_RBAC"

	// SettingKeyTenantEnableCrossTenantAccess 跨空间访问开关（默认 false）。
	// 生效路径同 SettingKeyTenantEnableRBAC。
	SettingKeyTenantEnableCrossTenantAccess = "tenant.enable_cross_tenant_access"
	SettingEnvTenantEnableCrossTenantAccess = "WEKNORA_TENANT_ENABLE_CROSS_TENANT_ACCESS"

	// SettingDefaultReliabilityWeight 可靠度权重内置默认（ADR-005 下限）。
	SettingDefaultReliabilityWeight = 0.25
)
