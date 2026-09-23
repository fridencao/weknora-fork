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

	// ---- 批二：service 层直读（消费点都有 ctx，无需桥接）----

	// SettingKeyHousekeepingEnabled 后台管家扫描总开关（默认 true）。
	SettingKeyHousekeepingEnabled = "housekeeping.enabled"
	SettingEnvHousekeepingEnabled = "WEKNORA_HOUSEKEEPING_ENABLED"

	// SettingKeyRetrievalMultiStoreTimeoutS 多向量库扇出检索的软超时（秒，默认 30）。
	SettingKeyRetrievalMultiStoreTimeoutS = "retrieval.multi_store_timeout_s"
	SettingEnvRetrievalMultiStoreTimeoutS = "MULTI_STORE_RETRIEVE_TIMEOUT_SEC"

	// SettingKeyTenantInvitationTTL 空间邀请链接有效期（Go duration 或秒数，默认 168h）。
	SettingKeyTenantInvitationTTL = "tenant.invitation_ttl"
	SettingEnvTenantInvitationTTL = "WEKNORA_INVITATION_TTL"

	// SettingKeyChatAttachmentTTLHours 会话临时附件的保留小时数（默认 24）。
	SettingKeyChatAttachmentTTLHours = "chat_attachment.ttl_hours"
	SettingEnvChatAttachmentTTLHours = "WEKNORA_CHAT_ATTACHMENT_TTL_HOURS"

	// SettingKeyChatAttachmentOCRMaxPages 单个扫描件送 VLM OCR 的页数上限（默认 8）。
	SettingKeyChatAttachmentOCRMaxPages = "chat_attachment.ocr_max_pages"
	SettingEnvChatAttachmentOCRMaxPages = "WEKNORA_CHAT_ATTACHMENT_OCR_MAX_PAGES"

	// SettingKeyChatAttachmentOCRConcurrency 扫描件多页 OCR 的并发度（默认 8）。
	SettingKeyChatAttachmentOCRConcurrency = "chat_attachment.ocr_concurrency"
	SettingEnvChatAttachmentOCRConcurrency = "WEKNORA_CHAT_ATTACHMENT_OCR_CONCURRENCY"

	// SettingKeyChatAttachmentWaitTimeoutS QA 轮次等待附件解析完成的秒数（默认 60）。
	SettingKeyChatAttachmentWaitTimeoutS = "chat_attachment.wait_timeout_s"
	SettingEnvChatAttachmentWaitTimeoutS = "WEKNORA_CHAT_ATTACHMENT_WAIT_TIMEOUT_SEC"

	// ---- 批二B：深包消费点，走包级 atomic 桥接 ----
	//
	// 这些消费点（models/vlm、models/embedding、utils、types、
	// storageallowlist）拿不到 ctx 也没有 settings 服务，因此由
	// systemSettingService 在 preload / dispatchSideEffects 推送到各包内的
	// atomic 覆盖位，读取顺序为 桥接值 > 环境变量 > 内置默认。

	// SettingKeyVLMHTTPTimeoutS VLM 请求的 HTTP 超时（秒，默认 180）。
	SettingKeyVLMHTTPTimeoutS = "vlm.http_timeout_s"
	SettingEnvVLMHTTPTimeoutS = "VLM_HTTP_TIMEOUT_SECONDS"

	// SettingKeyEmbeddingBatchSize 嵌入批处理的默认批大小（默认 5）。
	// 模型行自带的 GetBatchEmbedSize 仍优先于本键。
	SettingKeyEmbeddingBatchSize = "embedding.batch_size"
	SettingEnvEmbeddingBatchSize = "BATCH_EMBED_SIZE"

	// SettingKeyImageHostKeepURL 图片白名单：这些主机的图片不转存对象存储，
	// markdown 保留原始 URL（逗号分隔，如内网 MinerU 服务）。
	SettingKeyImageHostKeepURL = "image_host.keep_url"
	SettingEnvImageHostKeepURL = "IMAGE_HOST_KEEP_URL"

	// SettingKeyAuditRetentionDays 审计日志保留天数（0 禁用清理，默认 90）。
	SettingKeyAuditRetentionDays = "audit.retention_days"
	SettingEnvAuditRetentionDays = "WEKNORA_AUDIT_RETENTION_DAYS"

	// SettingKeyTaskPoolSize 异步任务协程池大小（默认 5，需重启生效）。
	SettingKeyTaskPoolSize = "task.pool_size"
	SettingEnvTaskPoolSize = "CONCURRENCY_POOL_SIZE"

	// SettingKeyLanguageDefault 默认语言区域（如 zh-CN / en-US，默认 zh-CN）。
	SettingKeyLanguageDefault = "language.default"
	SettingEnvLanguageDefault = "WEKNORA_LANGUAGE"

	// SettingKeyStorageAllowList 允许的存储后端白名单（留空表示全部允许）。
	SettingKeyStorageAllowList = "storage.allow_list"
	SettingEnvStorageAllowList = "STORAGE_ALLOW_LIST"

	// SettingDefaultReliabilityWeight 可靠度权重内置默认（ADR-005 下限）。
	SettingDefaultReliabilityWeight = 0.25
)
