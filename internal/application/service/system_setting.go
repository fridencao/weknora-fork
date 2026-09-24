package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/Tencent/WeKnora/internal/agent/approval"
	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/infrastructure/docparser"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/models/limiter"
	"github.com/Tencent/WeKnora/internal/models/vlm"
	"github.com/Tencent/WeKnora/internal/sandbox"
	"github.com/Tencent/WeKnora/internal/storageallowlist"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/Tencent/WeKnora/internal/utils"
)

// pubsubChannelBase is the Redis channel base for system_settings change
// notifications. Mirrors the convention from approval/gate.go: optional
// suffix WEKNORA_REDIS_NAMESPACE so two deployments sharing one Redis
// instance don't cross-talk.
const pubsubChannelBase = "weknora:system_settings:changed"

// pubsubChannel resolves the effective channel name (with optional
// namespace suffix). Called both at publish time and inside the
// subscriber loop — keep it pure.
func pubsubChannel() string {
	if ns := strings.TrimSpace(os.Getenv("WEKNORA_REDIS_NAMESPACE")); ns != "" {
		return pubsubChannelBase + ":" + ns
	}
	return pubsubChannelBase
}

// changeMessage is the JSON payload published whenever a setting is
// updated. OriginID lets the publishing replica skip its own message
// (it already updated its local cache inline) — without it every
// publish would trigger a redundant DB roundtrip per replica.
type changeMessage struct {
	Key      string `json:"key"`
	OriginID string `json:"origin_id"`
}

// settingSpec is the in-code registry entry for a known system setting.
// The registry serves as the **only** authority on which keys are legal
// + what type they hold + what their ENV-fallback name is + what the
// built-in default is. Adding a new tunable is a matter of:
//  1. Adding an entry here.
//  2. (Optional) adding a SQL seed row in a new migration so the UI
//     shows the row even before any operator hits Update.
//  3. Replacing existing os.Getenv() reads with calls into the
//     service.
//
// Update rejects any key not in this registry — so the UI cannot inject
// arbitrary keys into the DB, even with an attacker-controlled body.
type settingSpec struct {
	// Type is one of "int" | "string" | "bool" | "string_list". Update
	// validates the payload's Go type against this; reads decode accordingly.
	Type string
	// EnvName is the legacy environment variable consulted when the DB
	// row is absent. Empty string means "no ENV fallback for this key"
	// (the caller passes the desired default explicitly via the GetXxx
	// def parameter — useful when the cfg already coerced it at startup).
	EnvName string
	// Default is the built-in fallback used when both DB and ENV miss.
	// Type must match the Type field (int → int64, string → string,
	// bool → bool, string_list → []string); the typed Get* methods cast
	// accordingly. Currently unused by the resolver (callers pass def
	// inline) but kept for future cfg-less callsites.
	Default any
	// Enum, when non-empty, restricts Update to values in this set.
	// Only meaningful for Type=="string". Other types ignore it.
	// Empty/nil means no restriction (free-form string).
	Enum []string
	// Category drives UI grouping. Stored on the row at first write;
	// the seed migration sets it explicitly so management UI can
	// render even before any Update.
	Category string
	// Description is shown in the UI under the key. Stored on the row
	// at first write (mirrors Category).
	Description string
	// RequiresRestart marks keys whose value is bound at process startup
	// (e.g. asynq worker pool size). The UI shows a restart badge; the
	// service persists the flag on first write.
	RequiresRestart bool
}

// registry pins the set of legal keys. Expanding it is a deliberate,
// reviewable operation — the implicit contract is "every key here is
// safely runtime-tunable (no startup caching that would not honour
// the new value, no in-memory state bound at init time we cannot
// re-derive)".
var registry = map[string]settingSpec{
	// NOTE: file.max_size_mb is intentionally NOT registered. Although
	// the Go upload handlers accept a runtime override via
	// systemSettingSvc.GetInt, the actual upload limit is gated end-to-end
	// by three independent layers:
	//   1. nginx client_max_body_size (templated at container startup
	//      from the MAX_FILE_SIZE_MB env var; envsubst writes the
	//      computed value into nginx.conf; nginx is never reloaded
	//      during the container's lifetime).
	//   2. docreader gRPC max_send/recv_message_length (read from the
	//      MAX_FILE_SIZE_MB env at python startup).
	//   3. The frontend client-side check (utils/index.ts) reads
	//      window.__RUNTIME_CONFIG__.MAX_FILE_SIZE_MB which is
	//      written into /usr/share/nginx/html/config.js by the
	//      docker-entrypoint at container start.
	// Surfacing a UI knob whose effect is silently capped by nginx /
	// docreader / the in-page bundle is worse than not having it.
	// Keep MAX_FILE_SIZE_MB as a deploy-time env var until all four
	// layers can be reconfigured in lockstep without restarts.
	"ssrf.whitelist": {
		Type:     "string_list",
		EnvName:  "SSRF_WHITELIST",
		Default:  []string{},
		Category: "security",
		Description: "SSRF 防护白名单。可填入 example.com / *.foo.com / 10.0.0.0/8 / 2001:db8::1。" +
			"修改后立即生效。SSRF_WHITELIST_EXTRA 环境变量仍由部署方维护，不在此处覆盖。",
	},
	"sandbox.docker_enabled": {
		Type:     "bool",
		EnvName:  sandbox.DockerBackendEnabledEnv,
		Default:  false,
		Category: "security",
		Description: "是否允许 Docker 沙箱后端。本机 docker.sock 等同宿主机 root，默认关闭。" +
			"仅系统管理员可打开；打开后立即生效，无需重启。私有化单机且已挂载 daemon socket，或配置了带 TLS 的远程 tcp:// 时再启用。",
	},
	"auth.registration_mode": {
		Type:     "string",
		EnvName:  "", // No env fallback — handler passes cfg.Auth.RegistrationMode as default
		Default:  "self_serve",
		Enum:     []string{"self_serve", "invite_only"},
		Category: "auth",
		Description: "自助注册模式。self_serve = 任何人可注册账号；invite_only = 关闭公网注册，" +
			"仅 Owner/Admin 可邀请。修改后立即生效，但谨慎对待 self_serve（公网会接受 spam）。",
	},
	"auth.default_tenant_mode": {
		Type:     "string",
		EnvName:  "WEKNORA_AUTH_DEFAULT_TENANT_MODE",
		Default:  "create_personal",
		Enum:     []string{"create_personal", "tenantless"},
		Category: "auth",
		Description: "公开注册成功后的默认空间策略。create_personal = 自动创建个人空间并设为 Owner；" +
			"tenantless = 仅创建用户，等待接受邀请或主动创建空间。修改后只影响新注册用户。",
	},
	"auth.complex_password_enabled": {
		Type:     "bool",
		EnvName:  "WEKNORA_AUTH_COMPLEX_PASSWORD_ENABLED",
		Default:  false,
		Category: "auth",
		Description: "是否启用复杂密码。开启后密码必须包含大小写字母、数字和特殊字符。" +
			"修改后立即生效，只影响新注册用户或新密码修改/重置操作。特殊字符包含：!@#$%^&*()_+-=[]{}|;:,.<>?",
	},
	// tenant.max_owned_per_user caps how many tenants a single non-superuser
	// can create (and Own) via self-service POST /tenants. Read on every
	// request — UI edits take effect immediately, no restart required. The
	// EnvName is the same WEKNORA_TENANT_MAX_OWNED_PER_USER that
	// applyAuthAndTenantDefaults parses at boot, so a deployment that
	// hasn't created a DB row keeps reading from env exactly as before.
	// 0 = use the in-code default (10); negative = disable the cap entirely.
	"tenant.max_owned_per_user": {
		Type:     "int",
		EnvName:  "WEKNORA_TENANT_MAX_OWNED_PER_USER",
		Default:  int64(10),
		Category: "tenant",
		Description: "每个非超管用户通过自助创建可拥有的最大空间数。每次创建空间时实时读取，" +
			"修改后立即生效。0 表示使用内置默认值 10；负数表示完全关闭限制（不建议在公开部署使用）。",
	},
	"tenant.self_service_creation_enabled": {
		Type:     "bool",
		EnvName:  "WEKNORA_TENANT_SELF_SERVICE_CREATION_ENABLED",
		Default:  true,
		Category: "tenant",
		Description: "是否允许非超管用户主动创建空间。关闭后，普通用户只能通过邀请加入已有空间；" +
			"跨空间超管仍可创建。修改后立即生效。",
	},
	// tenant.default_storage_quota_gb is the default storage quota (in GB)
	// applied to a newly-created tenant when the caller doesn't specify
	// one explicitly. Read at create time only — changing the value does
	// NOT retroactively resize already-existing tenants (they keep the
	// quota stored on their row at creation; superusers can edit
	// individual tenants via the existing tenant-update path).
	// 0 or negative = use the in-code default (10 GB).
	"tenant.default_storage_quota_gb": {
		Type:     "int",
		EnvName:  "WEKNORA_TENANT_DEFAULT_STORAGE_QUOTA_GB",
		Default:  int64(10),
		Category: "tenant",
		Description: "新建空间时默认分配的存储配额（GB），包含向量、原文、文本、索引等。" +
			"仅在创建时读取，修改后只对之后新建的空间生效，不会回写已存在的空间。" +
			"0 或负数表示使用内置默认值 10GB。",
	},
	// tenant.auto_create_api_key restores the legacy behaviour where creating
	// a tenant also minted a full-access API key and returned its plaintext
	// token in the create response. Newer versions stopped doing this (keys
	// are created explicitly via tenant_api_keys), which is a breaking change
	// for integrations that relied on the create response carrying a key.
	// Deployments that need the old behaviour set this to true (or the
	// WEKNORA_TENANT_AUTO_CREATE_API_KEY env var). Default false keeps the
	// current, safer no-implicit-key behaviour. Read at create time only.
	"tenant.auto_create_api_key": {
		Type:     "bool",
		EnvName:  "WEKNORA_TENANT_AUTO_CREATE_API_KEY",
		Default:  false,
		Category: "tenant",
		Description: "创建空间时是否自动生成一个全量权限（full_access）的 API Key，并在创建接口的响应中返回其明文 token。" +
			"用于兼容旧版本「创建空间即下发默认 API Key」的行为（属于破坏性变更的回退开关）。" +
			"每次创建空间时实时读取，修改后立即生效。默认 false（不自动创建，需通过 API Key 管理显式创建）。",
	},
	// tenant.auto_accept_invitation: invite = auto-join switch (default false).
	"tenant.auto_accept_invitation": {
		Type:     "bool",
		EnvName:  "WEKNORA_TENANT_AUTO_ACCEPT_INVITATION",
		Default:  false,
		Category: "tenant",
		Description: "全局开关：开启后，空间管理员通过邮箱邀请已注册用户加入空间时，" +
			"被邀请人将被立即自动加入（直接写入成员关系），无需在收件箱手动接受，也不再生成待接受的邀请记录。" +
			"关闭时保持原有「发出邀请 → 被邀请人收件箱确认」流程。每次邀请时实时读取，修改后立即生效。默认 false。",
	},
	"asynq.core_concurrency": {
		Type:            "int",
		EnvName:         "WEKNORA_ASYNQ_CORE_CONCURRENCY",
		Default:         int64(types.DefaultCoreWorkerConcurrency),
		Category:        "worker",
		RequiresRestart: true,
		Description:     "文档解析、手工重解析等核心任务的每实例保底并发。可额外使用共享弹性池；修改后需重启。",
	},
	"asynq.postprocess_concurrency": {
		Type:            "int",
		EnvName:         "WEKNORA_ASYNQ_POSTPROCESS_CONCURRENCY",
		Default:         int64(types.DefaultPostProcessWorkerConcurrency),
		Category:        "worker",
		RequiresRestart: true,
		Description:     "解析完成后的轻量编排与富化扇出专用并发，避免被长时间文档解析阻塞；修改后需重启。",
	},
	"asynq.enrichment_concurrency": {
		Type:            "int",
		EnvName:         "WEKNORA_ASYNQ_ENRICHMENT_CONCURRENCY",
		Default:         int64(types.DefaultEnrichmentWorkerConcurrency),
		Category:        "worker",
		RequiresRestart: true,
		Description:     "摘要、图片、图谱和问题生成的每实例保底并发。可额外使用共享弹性池；修改后需重启。",
	},
	"asynq.maintenance_concurrency": {
		Type:            "int",
		EnvName:         "WEKNORA_ASYNQ_MAINTENANCE_CONCURRENCY",
		Default:         int64(types.DefaultMaintenanceWorkerConcurrency),
		Category:        "worker",
		RequiresRestart: true,
		Description:     "数据源同步、批处理、移动和删除清理的每实例保底并发，与用户面流水线硬隔离；修改后需重启。",
	},
	"asynq.shared_concurrency": {
		Type:            "int",
		EnvName:         "WEKNORA_ASYNQ_SHARED_CONCURRENCY",
		Default:         int64(types.DefaultSharedWorkerConcurrency),
		Category:        "worker",
		RequiresRestart: true,
		Description:     "核心解析与内容富化共用的每实例弹性并发。空闲容量由有积压的一侧借用；修改后需重启。",
	},
	// asynq.wiki_concurrency is the size of the DEDICATED wiki worker pool,
	// separate from the upstream pools. Read once when the wiki asynq server
	// starts — changing it in the UI requires a process restart. Mirrors
	// WEKNORA_WIKI_ASYNQ_CONCURRENCY (default 8).
	"asynq.wiki_concurrency": {
		Type:            "int",
		EnvName:         "WEKNORA_WIKI_ASYNQ_CONCURRENCY",
		Default:         int64(types.DefaultWikiWorkerConcurrency),
		Category:        "worker",
		RequiresRestart: true,
		Description: "Wiki 生成专用池的 worker 并发数（与文档解析池相互隔离）。" +
			"Wiki 生成以合成大模型调用为主，独立并发预算可避免上传高峰期被解析任务饿死，" +
			"同时不会因 Wiki 洪峰拖慢用户面解析。修改后需重启服务进程方可生效。",
	},
	// model.max_concurrency is the DEFAULT per-model cap on concurrent
	// background (ingestion/enrichment) LLM/embedding/VLM calls, keyed by
	// model ID and shared across replicas. Read at every gated call via the
	// limiter governor; a runtime bridge (applyModelMaxConcurrency) pushes UI
	// edits into limiter.SetGlobalLimit so no restart is needed. Individual
	// models may override this via their own max_concurrency parameter.
	// Mirrors WEKNORA_MODEL_MAX_CONCURRENCY (default 32). 0/negative disables
	// the default cap.
	"model.max_concurrency": {
		Type:     "int",
		EnvName:  "WEKNORA_MODEL_MAX_CONCURRENCY",
		Default:  int64(32),
		Category: "worker",
		Description: "后台任务（文档入库/富化）对单个模型的默认并发上限，按模型 ID 全副本共享。" +
			"每次调用实时读取，修改后立即生效、无需重启。0 或负数表示关闭默认限制" +
			"（各模型仍会尊重自身在模型管理里配置的上限）。仅影响后台任务，不影响交互式对话。",
	},

	// ---------------------------------------------------------------------
	// StarKB 配置治理批一：业务可调参数从环境变量迁入系统设置。
	//
	// 边界：基础设施连接（DB/Redis/PG/对象存储）、密钥、路径、后端选型、
	// 引导与测试夹具**仍留环境变量**；本段只收纳「运维应当在界面上改」的
	// 权重、开关、策略与超时。EnvName 保留为兜底（DB > ENV > Default），
	// docker-compose 中对应变量已删除，故正常部署走 DB 或 Default。
	// ---------------------------------------------------------------------

	// fusion.reliability.weight 与 STARKB_RELIABILITY_WEIGHT 曾是同一语义的
	// 两个配置源，且 env 那条绕过了 ≥0.25 锁定（ADR-005 硬约束）。现统一到
	// 本键，下限由 validateRegistryEntry 强制。
	"fusion.reliability.weight": {
		Type:     "float",
		EnvName:  "STARKB_RELIABILITY_WEIGHT",
		Default:  0.25,
		Category: "retrieval",
		Description: "融合排序中「可靠度因子」的权重。下限 0.25（ADR-005 硬约束，" +
			"低于此值会被拒绝）。修改后立即生效，无需重启。",
	},

	// 图谱通道部署级默认。智能体可在其「检索策略」上做三态覆盖
	// （未设置 → 用本值），见 ADR-008 决策 2/3.1。
	"graph.channel.enabled": {
		Type:     "bool",
		EnvName:  "GRAPH_CHANNEL_ENABLED",
		Default:  true,
		Category: "retrieval",
		Description: "图谱通道的部署级默认开关。智能体未显式设置时使用本值；" +
			"智能体可各自覆盖。修改后立即生效，无需重启。",
	},
	"graph.channel.top_k": {
		Type:        "int",
		EnvName:     "GRAPH_CHANNEL_TOP_K",
		Default:     int64(20),
		Category:    "retrieval",
		Description: "图谱通道返回的实体/关系条数上限。修改后立即生效，无需重启。",
	},
	"graph.channel.chunks_per_hit": {
		Type:     "int",
		EnvName:  "GRAPH_CHANNEL_CHUNKS_PER_HIT",
		Default:  int64(2),
		Category: "retrieval",
		Description: "图谱通道每个命中实体附带裁剪的证据 chunk 数。越大证据越全、" +
			"但注入上下文越长。修改后立即生效，无需重启。",
	},
	"graph.channel.timeout_s": {
		Type:     "int",
		EnvName:  "GRAPH_CHANNEL_TIMEOUT_S",
		Default:  int64(8),
		Category: "retrieval",
		Description: "图谱通道软超时（秒），取值 1–120。检索主链路不应被图谱抖动拖死；" +
			"思考型 LLM 做关键词抽取时可放大。修改后立即生效，无需重启。",
	},

	// StarKB 功能开关。迁移前均为裸 os.Getenv，无法在界面上改。
	"starkb.claim_gate": {
		Type:     "bool",
		EnvName:  "STARKB_CLAIM_GATE",
		Default:  false,
		Category: "starkb",
		Description: "答案级论断审计（claim gate）总开关。开启后 AI 回答会随消息持久化" +
			"逐句论断与引用绑定，供前端角标与复核使用。修改后立即生效，无需重启。",
	},
	"starkb.require_provenance": {
		Type:     "bool",
		EnvName:  "STARKB_REQUIRE_PROVENANCE",
		Default:  false,
		Category: "starkb",
		Description: "强绑定溯源门禁。开启后无有效溯源的论断会被剥离，而不是仅告警。" +
			"建议在溯源覆盖率稳定后再开。修改后立即生效，无需重启。",
	},
	"starkb.align_on_ingest": {
		Type:     "bool",
		EnvName:  "STARKB_ALIGN_ON_INGEST",
		Default:  true,
		Category: "starkb",
		Description: "入库后自动做 T1 溯源对齐（把 sbk_* 写回 chunk metadata）。" +
			"需同时配置 starkb-api 地址；对齐失败仅告警、不阻断入库。修改后立即生效。",
	},
	"starkb.graph_on_ingest": {
		Type:     "bool",
		EnvName:  "STARKB_GRAPH_ON_INGEST",
		Default:  true,
		Category: "starkb",
		Description: "入库完成后自动把文档投喂图谱建图（走 starkb-api 回填队列，按配额限速）。" +
			"关闭后只能手工触发建图。修改后立即生效，无需重启。",
	},
	"starkb.graph_cleanup_on_delete": {
		Type:     "bool",
		EnvName:  "STARKB_GRAPH_CLEANUP_ON_DELETE",
		Default:  true,
		Category: "starkb",
		Description: "删除知识时同步清理其图谱数据（节点/关系/向量）。关闭时图谱会残留" +
			"孤儿数据，需人工巡检。修改后立即生效，无需重启。",
	},
	"starkb.graph_build_max_async": {
		Type:     "int",
		EnvName:  "STARKB_GRAPH_MAX_ASYNC",
		Default:  int64(2),
		Category: "starkb",
		Description: "图谱抽取的 LLM 并发数（1–8）。由 starkb-api 建图热读取；" +
			"受模型服务商账号并发上限约束，超限会触发 429 退避重试。" +
			"修改后下一轮建图生效。",
	},
	"starkb.graph_build_llm_interval_ms": {
		Type:     "int",
		EnvName:  "STARKB_GRAPH_LLM_INTERVAL_MS",
		Default:  int64(2000),
		Category: "starkb",
		Description: "图谱抽取相邻 LLM 调用的平滑间隔（毫秒，0–10000）。调小可提速，" +
			"但会增加触发服务商限流的概率。由 starkb-api 建图热读取；修改后下一轮建图生效。",
	},

	// Agent 超时与审批策略。
	"agent.llm_timeout": {
		Type:     "string",
		EnvName:  "WEKNORA_AGENT_LLM_TIMEOUT",
		Default:  "120s",
		Category: "agent",
		Description: "单次 LLM 调用的默认超时。支持 Go duration 写法（如 300s、5m）或" +
			"纯数字（按秒解释）。留空则用 agent 内置默认（120s）。修改后立即生效，无需重启。",
	},
	"agent.tool_approval_timeout": {
		Type:     "string",
		EnvName:  "WEKNORA_AGENT_TOOL_APPROVAL_TIMEOUT",
		Default:  "600s",
		Category: "agent",
		Description: "MCP 高风险工具等待人工审批的时长。支持 Go duration 写法或纯数字" +
			"（按秒解释）。超时后的行为由「审批超时放行」决定。修改后立即生效。",
	},
	"agent.tool_approval_fail_open": {
		Type:     "bool",
		EnvName:  "WEKNORA_AGENT_TOOL_APPROVAL_FAIL_OPEN",
		Default:  false,
		Category: "agent",
		Description: "审批超时/审批服务不可用时是否放行高风险工具调用。默认关闭（fail-closed，" +
			"拒绝执行）。仅在明确接受风险时才打开。修改后立即生效，无需重启。",
	},

	// 解析链路超时。
	"docreader.call_timeout": {
		Type:     "string",
		EnvName:  "WEKNORA_DOCREADER_CALL_TIMEOUT",
		Default:  "30m",
		Category: "docreader",
		Description: "调用 docreader 解析服务的单次超时。支持 Go duration 写法（如 30m、1h）。" +
			"大体积扫描件需放大。修改后立即生效，无需重启。",
	},
	"document.process_timeout": {
		Type:     "string",
		EnvName:  "WEKNORA_DOCUMENT_PROCESS_TIMEOUT",
		Default:  "2h",
		Category: "docreader",
		Description: "单个文档从入库到解析完成的总超时，超过则判定失败并触发降档重试。" +
			"支持 Go duration 写法。修改后立即生效，无需重启。",
	},

	// 租户安全门禁。与其余 RequiresRestart 键的区别：asynq.* 那批由消费方在
	// 构造时调用 GetInt 读取（resolveRaw 的 pre-warmup 路径保证 DB 可达），
	// 而这两项的消费方是中间件与路由，读的是 *config.Config 单例上的字段，
	// 无法在调用点解析。因此由 cmd/server/bootstrap.go 的启动钩子在
	// 「迁移完成、监听端口之前」同步写入 cfg —— 那个时点没有并发读者，写入
	// 是安全的；这也是它们必须 RequiresRestart 的原因（运行期改不会推送）。
	"tenant.enable_rbac": {
		Type:            "bool",
		EnvName:         "WEKNORA_TENANT_ENABLE_RBAC",
		Default:         true,
		Category:        "tenant",
		RequiresRestart: false,
		Description: "是否启用空间级角色强制鉴权。关闭时空间内的角色检查只记录不拦截" +
			"（跨空间访问始终拦截），仅建议单机私有化部署使用。默认 true。" +
			"该值在进程启动时绑定，修改后需重启服务方可生效。",
	},
	"tenant.enable_cross_tenant_access": {
		Type:            "bool",
		EnvName:         "WEKNORA_TENANT_ENABLE_CROSS_TENANT_ACCESS",
		Default:         false,
		Category:        "tenant",
		RequiresRestart: false,
		Description: "是否允许跨空间访问（需同时具备 CanAccessAllTenants 权限，" +
			"两者都为真才放行）。默认 false。该值在进程启动时绑定，修改后需重启" +
			"服务方可生效。",
	},

	// ---- 批二：service 层直读（消费点都有 ctx）----

	"housekeeping.enabled": {
		Type:     "bool",
		EnvName:  "WEKNORA_HOUSEKEEPING_ENABLED",
		Default:  true,
		Category: "maintenance",
		Description: "后台管家扫描的总开关。负责回收长时间卡在解析中/建图中的文档，" +
			"把它们翻回可重试状态。默认 true；关闭后卡住的文档需要人工重解析。" +
			"在管家启动时读取，修改后需重启进程生效。",
	},
	"retrieval.multi_store_timeout_s": {
		Type:     "int",
		EnvName:  "MULTI_STORE_RETRIEVE_TIMEOUT_SEC",
		Default:  int64(30),
		Category: "retrieval",
		Description: "跨多个向量库扇出检索时的软超时（秒）。单个库慢不应拖垮整轮问答：" +
			"超时后返回已就绪库的结果并记降级。调大提高召回完整度、增加尾延迟；" +
			"调小保护响应时间。默认 30。",
	},
	"tenant.invitation_ttl": {
		Type:     "string",
		EnvName:  "WEKNORA_INVITATION_TTL",
		Default:  "168h",
		Category: "tenant",
		Description: "空间邀请链接的有效期。支持 Go duration（如 168h、7d 请写 168h）" +
			"或纯秒数（如 604800）。过期后邀请不可用，需重新发出。默认 168h（7 天）。",
	},
	"chat_attachment.ttl_hours": {
		Type:     "int",
		EnvName:  "WEKNORA_CHAT_ATTACHMENT_TTL_HOURS",
		Default:  int64(24),
		Category: "chat",
		Description: "会话中上传的临时附件保留多少小时后自动清理。默认 24。" +
			"调大占用更多存储，但用户回看历史会话时附件仍在。",
	},
	"chat_attachment.ocr_max_pages": {
		Type:     "int",
		EnvName:  "WEKNORA_CHAT_ATTACHMENT_OCR_MAX_PAGES",
		Default:  int64(8),
		Category: "chat",
		Description: "扫描件/纯图片文档最多送多少页去做 VLM OCR，用于约束 OCR 延迟。" +
			"超出部分不参与识别。默认 8。",
	},
	"chat_attachment.ocr_concurrency": {
		Type:     "int",
		EnvName:  "WEKNORA_CHAT_ATTACHMENT_OCR_CONCURRENCY",
		Default:  int64(8),
		Category: "chat",
		Description: "多页扫描件同时送 VLM 做 OCR 的并发度。墙钟延迟随并发近似线性下降，" +
			"但会加大对 VLM 后端的压力。默认 8（与页数上限一致，一屏扫完）。",
	},
	"chat_attachment.wait_timeout_s": {
		Type:     "int",
		EnvName:  "WEKNORA_CHAT_ATTACHMENT_WAIT_TIMEOUT_SEC",
		Default:  int64(60),
		Category: "chat",
		Description: "发起提问时，最多等待仍在解析中的附件多少秒；超时后只用已完成的" +
			"附件继续回答（未完成的跳过，不报错）。默认 60。大文件或扫描件可调大。",
	},

	// ---- 批二B：深包消费点，经包级 atomic 桥接生效 ----

	"vlm.http_timeout_s": {
		Type:     "int",
		EnvName:  "VLM_HTTP_TIMEOUT_SECONDS",
		Default:  int64(180),
		Category: "model",
		Description: "调用视觉模型（VLM）时的 HTTP 超时（秒）。扫描件 OCR、图片描述" +
			"都走这条链路；大图或慢后端可调大。默认 180。",
	},
	"embedding.batch_size": {
		Type:     "int",
		EnvName:  "BATCH_EMBED_SIZE",
		Default:  int64(5),
		Category: "model",
		Description: "文本嵌入的默认批大小。模型行自带调优值时以模型行为准，本键是" +
			"兜底默认。调大提高吞吐、增加单次请求内存与超时风险。默认 5。",
	},
	"language.default": {
		Type:     "string",
		EnvName:  "WEKNORA_LANGUAGE",
		Default:  "",
		Category: "general",
		Description: "未显式指定语言时的默认区域，如 zh-CN / en-US / ja-JP。" +
			"留空则用内置默认 zh-CN。影响提示词语言与异步任务的默认语言。",
	},
	"storage.allow_list": {
		Type:     "string_list",
		EnvName:  "STORAGE_ALLOW_LIST",
		Default:  []string{},
		Category: "storage",
		Description: "允许使用的存储后端白名单（local/minio/cos/tos/s3/oss/ks3/obs）。" +
			"留空表示全部允许。用于把部署限制在合规的存储后端上。",
	},
	"image_host.keep_url": {
		Type:     "string_list",
		EnvName:  "IMAGE_HOST_KEEP_URL",
		Default:  []string{},
		Category: "document",
		Description: "受信图片主机白名单。列表内主机的图片仍会下载校验/OCR，" +
			"但不转存对象存储，markdown 保留原始 URL。典型用途是内网 MinerU " +
			"等解析服务。留空表示无白名单（全部图片转存）。",
	},
	"audit.retention_days": {
		Type:     "int",
		EnvName:  "WEKNORA_AUDIT_RETENTION_DAYS",
		Default:  int64(90),
		Category: "audit",
		Description: "审计日志保留天数，超期自动清理；0 表示禁用清理（自行归档的" +
			"合规场景）。默认 90。改动在下次巡检（每日一次）时生效，无需重启。",
	},
	"task.pool_size": {
		Type:            "int",
		EnvName:         "CONCURRENCY_POOL_SIZE",
		Default:         int64(5),
		Category:        "task",
		RequiresRestart: true,
		Description: "异步任务（文档解析等）的并发协程池大小。调大提高吞吐，" +
			"但占用更多 CPU/内存与下游配额。默认 5。该值在进程启动时绑定，" +
			"改动需重启服务进程方可生效。",
	},
}

// systemSettingService wires the repository, audit log, and (P2)
// the Redis client + an in-memory cache. Cache strategy is "preload
// at boot, invalidate via pubsub":
//
//   - On startup we async-load every row into `cache` (best-effort —
//     a DB hiccup just means a slower warmup, not a fatal error).
//   - GetXxx reads from cache (microsecond latency).
//   - Update writes DB → updates local cache → publishes a change
//     notification to Redis.
//   - Subscribers on every replica read the notification and re-fetch
//     the row from DB (NOT from the message payload — the message only
//     carries the key, never the value, so we never trust pubsub-as-
//     transport with config bytes).
//   - The publishing replica skips its own messages by matching
//     OriginID against its instanceID.
//
// When Redis is nil (lite mode / REDIS_ADDR unset), every code path
// degrades back to P1 behaviour: no cache invalidation, but local
// edits still take effect (since Update does write the local cache
// inline). This is the right behaviour for single-replica deployments.
type systemSettingService struct {
	// 租户门禁的 YAML 层缺省，由 bootstrap 在构造后注入（SetTenantGateDefaults）。
	tenantRBACDef  bool
	tenantCrossDef bool

	repo  interfaces.SystemSettingRepository
	audit interfaces.AuditLogService
	rdb   *redis.Client // may be nil in lite mode
	cfg   *config.Config

	// instanceID disambiguates this replica from its peers in the
	// pubsub stream. Generated once at construction; never changes.
	instanceID string

	// cache holds every known setting indexed by key. Populated by
	// loadCache (preload + after every pubsub message). All access
	// goes through `mu`. A nil entry means "we know there's no row
	// and the resolver should fall through to ENV/default".
	mu    sync.RWMutex
	cache map[string]*types.SystemSetting

	// loaded flips true once the initial preload finishes. Reads
	// before this point fall through to the DB so the very first
	// hot request after boot doesn't get a default-valued surprise.
	loaded atomic.Bool

	// subOnce guarantees SubscribeRedis can be called multiple times
	// without spawning duplicate goroutines (defensive — main only
	// calls it once).
	subOnce sync.Once
}

// NewSystemSettingService is the dig provider. audit may be nil
// (matches the tenantMemberService convention — tests that don't care
// about audit can pass nil and emitAudit no-ops). rdb may also be nil
// when REDIS_ADDR is unset — the service still uses its local cache,
// but skips cross-replica pubsub invalidation.
func NewSystemSettingService(
	repo interfaces.SystemSettingRepository,
	audit interfaces.AuditLogService,
	rdb *redis.Client,
	cfg *config.Config,
) interfaces.SystemSettingService {
	s := &systemSettingService{
		repo:       repo,
		audit:      audit,
		rdb:        rdb,
		cfg:        cfg,
		instanceID: uuid.NewString(),
		cache:      make(map[string]*types.SystemSetting),
	}
	// 租户门禁的 YAML 层缺省在此定格：此刻任何覆盖位都还没被推送，
	// IsRBACEnforced() 走 YAML/内置默认分支，正是我们要的 def。
	// 之后 dispatch/preload 的推送以 DB/ENV 层为准，def 只在两层都缺时兜底。
	if cfg != nil && cfg.Tenant != nil {
		s.tenantRBACDef = cfg.Tenant.IsRBACEnforced()
		s.tenantCrossDef = cfg.Tenant.EnableCrossTenantAccess
	}
	// Async preload — don't block container build / handler readiness
	// on a slow DB. The first few requests may miss cache and hit the
	// DB directly via the resolver fallback; that's a few ms each and
	// completes long before the cache is full.
	go s.preload(context.Background())
	return s
}

// preload populates the cache with every row from the system_settings
// table. Best-effort: a DB error here is logged and silently swallowed,
// because the resolver's DB-fallback path will still serve correct
// values (just slower). Logging the count gives operators a single line
// in the startup log they can grep for ("how many keys did P2 load?").
func (s *systemSettingService) preload(ctx context.Context) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		logger.Warnf(ctx, "[system_settings] preload failed, falling back to per-request DB reads: %v", err)
		return
	}
	s.mu.Lock()
	for _, row := range rows {
		s.cache[row.Key] = row
	}
	s.mu.Unlock()

	s.loaded.Store(true)
	s.mu.RLock()
	loadedCount := len(s.cache)
	s.mu.RUnlock()
	logger.Infof(ctx, "[system_settings] cache loaded %d keys (instance=%s)", loadedCount, s.instanceID[:8])

	// Side-effect bridges: any setting whose live value affects an
	// in-process subsystem needs to be pushed there after preload, so
	// the subsystem doesn't lag the cache by a full request cycle.
	// Add new bridges here as more env vars get migrated.
	s.applySSRFWhitelist(ctx)
	s.applyModelMaxConcurrency(ctx)
	s.applyDockerBackendEnabled(ctx)
	s.applyToolApprovalSettings(ctx)
	s.applyDeepPackageBridges(ctx)
	s.applyTenantGateBridges(ctx)
}

// applyDeepPackageBridges 把深包配置推送到各包内的 atomic 覆盖位。
//
// 这些消费点（models/vlm、models/embedding、utils、types、
// storageallowlist）既拿不到 ctx 也没有 settings 服务——VLM 超时在客户端
// 构造时读、批大小在嵌入循环里读、语言在提示词构造与任务载荷里读、
// 存储白名单在 provider 构造时读。所以只能反向推送，与 sandbox /
// approval 的桥接同一形状。
//
// 全量重推而非按键分发：一次推送就是几个 atomic store，为省这点开销
// 维护一份「哪个键影响哪个包」的映射不划算，也更容易漏。
func (s *systemSettingService) applyDeepPackageBridges(ctx context.Context) {
	vlmTimeoutS := s.GetInt(ctx,
		types.SettingKeyVLMHTTPTimeoutS, types.SettingEnvVLMHTTPTimeoutS, 180)
	vlm.SetVLMHTTPTimeout(time.Duration(vlmTimeoutS) * time.Second)

	batchEmbed := int(s.GetInt(ctx,
		types.SettingKeyEmbeddingBatchSize, types.SettingEnvEmbeddingBatchSize, 5))
	embedding.SetBatchEmbedSize(batchEmbed)

	language := s.GetString(ctx,
		types.SettingKeyLanguageDefault, types.SettingEnvLanguageDefault, "")
	types.SetDefaultLanguage(language)

	allowList := s.GetStringList(ctx,
		types.SettingKeyStorageAllowList, types.SettingEnvStorageAllowList, nil)
	storageallowlist.SetStorageAllowList(allowList)

	imageHosts := s.GetStringList(ctx,
		types.SettingKeyImageHostKeepURL, types.SettingEnvImageHostKeepURL, nil)
	docparser.SetImageHostKeepURLs(imageHosts)

	logger.Infof(ctx,
		"[system_settings] deep-package bridges applied "+
			"(vlm_timeout=%ds, batch_embed=%d, language=%q, storage_allow=%d, image_hosts=%d)",
		vlmTimeoutS, batchEmbed, language, len(allowList), len(imageHosts))
}

// applyToolApprovalSettings 把审批超时与 fail-open 策略推给 approval 包。
// Gate 是启动期单例，不推送的话改了设置也要重启才生效。
func (s *systemSettingService) applyToolApprovalSettings(ctx context.Context) {
	timeoutRaw := s.GetString(ctx,
		types.SettingKeyAgentToolApprovalTimeout, types.SettingEnvAgentToolApprovalTimeout, "600s")
	if secs, ok := parseDurationSeconds(timeoutRaw); ok {
		approval.SetToolApprovalTimeout(secs)
	}
	approval.SetToolApprovalFailOpen(s.GetBool(ctx,
		types.SettingKeyAgentToolApprovalFailOpen, types.SettingEnvAgentToolApprovalFailOpen, false))
	logger.Infof(ctx, "[system_settings] agent.tool_approval_* applied (timeout=%s)", timeoutRaw)
}

// encodeDefault produces the JSONB encoding for a spec's built-in
// default. Mirrors encodeForType but operates on already-typed Go
// values from registry so we never have to round-trip through `any`
// type assertions on the seed path. Returns an error when spec.Default
// is missing or its Go type doesn't match spec.Type — that's a code
// bug in the registry entry, surface it loudly rather than silently
// seeding the wrong shape.
func encodeDefault(spec settingSpec) (types.JSON, error) {
	switch spec.Type {
	case "int":
		var n int64
		switch v := spec.Default.(type) {
		case int:
			n = int64(v)
		case int64:
			n = v
		case float64:
			n = int64(v)
		default:
			return nil, fmt.Errorf("registry spec for int has wrong default type %T", spec.Default)
		}
		b, _ := json.Marshal(n)
		return types.JSON(b), nil
	case "float":
		var f float64
		switch v := spec.Default.(type) {
		case float64:
			f = v
		case float32:
			f = float64(v)
		case int:
			f = float64(v)
		case int64:
			f = float64(v)
		default:
			return nil, fmt.Errorf("registry spec for float has wrong default type %T", spec.Default)
		}
		b, _ := json.Marshal(f)
		return types.JSON(b), nil
	case "string":
		v, ok := spec.Default.(string)
		if !ok {
			return nil, fmt.Errorf("registry spec for string has wrong default type %T", spec.Default)
		}
		b, _ := json.Marshal(v)
		return types.JSON(b), nil
	case "bool":
		v, ok := spec.Default.(bool)
		if !ok {
			return nil, fmt.Errorf("registry spec for bool has wrong default type %T", spec.Default)
		}
		b, _ := json.Marshal(v)
		return types.JSON(b), nil
	case "string_list":
		switch v := spec.Default.(type) {
		case []string:
			if v == nil {
				v = []string{}
			}
			b, _ := json.Marshal(v)
			return types.JSON(b), nil
		case nil:
			return types.JSON(`[]`), nil
		default:
			return nil, fmt.Errorf("registry spec for string_list has wrong default type %T", spec.Default)
		}
	default:
		return nil, errors.New("unknown declared type: " + spec.Type)
	}
}

// reload re-fetches a single key from DB and updates the cache. Called
// from the pubsub subscriber loop after another replica publishes a
// change. A repo.Get(nil) result removes the entry — the row must have
// been deleted by an out-of-band tool (P1 has no Delete endpoint, but
// hand-edits still work).
func (s *systemSettingService) reload(ctx context.Context, key string) {
	row, err := s.repo.Get(ctx, key)
	if err != nil {
		logger.Warnf(ctx, "[system_settings] reload %q failed: %v", key, err)
		return
	}
	s.mu.Lock()
	if row == nil {
		delete(s.cache, key)
	} else {
		s.cache[key] = row
	}
	s.mu.Unlock()

	// Push any side-effect bridges for the changed key. Bridges are
	// idempotent — calling them on every reload (even when the change
	// is to a different key) is fine and lets us avoid plumbing a
	// per-key dispatch table.
	s.dispatchSideEffects(ctx, key)
}

// dispatchSideEffects fans out post-Update / post-reload work to
// subsystems whose state depends on a system_setting. Each bridge
// looks up its own keys and decides whether to act — this keeps the
// dispatcher trivial as we add more.
func (s *systemSettingService) dispatchSideEffects(ctx context.Context, changedKey string) {
	switch changedKey {
	case "ssrf.whitelist":
		s.applySSRFWhitelist(ctx)
	case "model.max_concurrency":
		s.applyModelMaxConcurrency(ctx)
	case sandbox.DockerBackendEnabledSettingKey:
		s.applyDockerBackendEnabled(ctx)
	case types.SettingKeyAgentToolApprovalTimeout, types.SettingKeyAgentToolApprovalFailOpen:
		s.applyToolApprovalSettings(ctx)
	case types.SettingKeyVLMHTTPTimeoutS,
		types.SettingKeyEmbeddingBatchSize,
		types.SettingKeyLanguageDefault,
		types.SettingKeyImageHostKeepURL,
		types.SettingKeyStorageAllowList:
		s.applyDeepPackageBridges(ctx)
	case types.SettingKeyTaskPoolSize:
		s.logRestartRequiredSetting(ctx, changedKey)
	case types.SettingKeyTenantEnableRBAC, types.SettingKeyTenantEnableCrossTenantAccess:
		// 两个安全门禁经 config 包的 atomic 覆盖位热更新（见
		// config/tenant_gate_bridge.go）。读取点改走 Effective 方法后不再
		// 与请求路径构成 data race；bootstrap 启动钩子仍在 listen 前同步
		// 应用一次，封住 preload 完成前的窗口。
		s.applyTenantGateBridges(ctx)
	}
}

// applyTenantGateBridges 把两个租户安全门禁推送到 config 包的覆盖位。
// 独立于 applyDeepPackageBridges：门禁推送失败必须显眼，不能混在一批
// 调优参数的日志里。
func (s *systemSettingService) applyTenantGateBridges(ctx context.Context) {
	rbac := s.GetBool(ctx,
		types.SettingKeyTenantEnableRBAC, types.SettingEnvTenantEnableRBAC, s.tenantRBACDef)
	cross := s.GetBool(ctx,
		types.SettingKeyTenantEnableCrossTenantAccess, types.SettingEnvTenantEnableCrossTenantAccess, s.tenantCrossDef)
	config.SetTenantRBACEnforcedOverride(rbac)
	config.SetTenantCrossTenantAccessOverride(cross)
	logger.Infof(ctx,
		"[system_settings] tenant gates applied (enable_rbac=%t enable_cross_tenant_access=%t)", rbac, cross)
}

// logRestartRequiredSetting 输出「已保存但需重启」的提示。消费方读的是
// 进程启动时绑定的值（config 单例字段或一次性构造的组件），运行期写入
// 不生效；留这条日志避免运维以为「保存了就已经生效」。
func (s *systemSettingService) logRestartRequiredSetting(ctx context.Context, key string) {
	logger.Infof(ctx,
		"[system_settings] %q 已保存，但该值在进程启动时绑定；"+
			"需重启服务进程方可生效（当前运行值保持不变）", key)
}

// applySSRFWhitelist resolves the active ssrf.whitelist via the 3-tier
// resolver and pushes the result (merged with SSRF_WHITELIST_EXTRA)
// to utils.SetSSRFWhitelistFromRaw. SSRF_WHITELIST_EXTRA stays env-only:
// it's typically set by docker-compose / k8s for sidecar service names
// and shouldn't be subject to UI accidents.
//
// Called at preload (initial sync), after Update (this replica's edit),
// and after reload (peer's edit via pubsub).
func (s *systemSettingService) applySSRFWhitelist(ctx context.Context) {
	list := s.GetStringList(ctx, "ssrf.whitelist", "SSRF_WHITELIST", []string{})
	primary := strings.Join(list, ",")
	extra := strings.TrimSpace(os.Getenv("SSRF_WHITELIST_EXTRA"))
	merged := primary
	if extra != "" {
		if merged == "" {
			merged = extra
		} else {
			merged = merged + "," + extra
		}
	}
	utils.SetSSRFWhitelistFromRaw(merged)
	logger.Infof(ctx, "[system_settings] SSRF whitelist applied (%d primary entries, extra=%v)",
		len(list), extra != "")
}

// applyModelMaxConcurrency resolves model.max_concurrency via the 3-tier
// resolver and pushes it into the model concurrency governor so UI edits take
// effect without a restart. Only the process-wide default limit is retuned;
// the installed limiter backend (redis/local) stays intact. The default (8)
// deliberately mirrors container.defaultModelMaxConcurrency so the value here
// matches what the container installs at boot.
//
// Called at preload (initial sync), after Update (this replica's edit), and
// after reload (peer's edit via pubsub).
func (s *systemSettingService) applyModelMaxConcurrency(ctx context.Context) {
	limit := int(s.GetInt(ctx, "model.max_concurrency", "WEKNORA_MODEL_MAX_CONCURRENCY", 32))
	limiter.SetGlobalLimit(limit)
	logger.Infof(ctx, "[system_settings] model.max_concurrency applied (limit=%d)", limit)
}

func (s *systemSettingService) applyDockerBackendEnabled(ctx context.Context) {
	enabled := s.GetBool(ctx, sandbox.DockerBackendEnabledSettingKey, sandbox.DockerBackendEnabledEnv, false)
	sandbox.SetDockerBackendEnabled(enabled)
	logger.Infof(ctx, "[system_settings] sandbox.docker_enabled applied (enabled=%v)", enabled)
}

// publishChange fans the change out to peers. Best-effort: a Redis
// outage logs a warning but does not fail the Update — the DB write
// already succeeded and our local cache is up-to-date. Other replicas
// will pick up the new value on their next preload (e.g. restart) or
// when their own resolver detects a stale cache via fallback.
func (s *systemSettingService) publishChange(ctx context.Context, key string) {
	if s.rdb == nil {
		return
	}
	payload, err := json.Marshal(changeMessage{Key: key, OriginID: s.instanceID})
	if err != nil {
		logger.Warnf(ctx, "[system_settings] marshal change for %q: %v", key, err)
		return
	}
	pubCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := s.rdb.Publish(pubCtx, pubsubChannel(), payload).Err(); err != nil {
		logger.Warnf(ctx, "[system_settings] publish %q: %v", key, err)
	}
}

// NewSystemSettingService is the dig provider. audit may be nil
// (matches the tenantMemberService convention — tests that don't care
// about audit can pass nil and emitAudit no-ops).
//
// Compatibility shim: the real ctor lives above (with rdb). Keeping this
// alternate signature would break dig (two providers for one type), so
// it is intentionally NOT exported separately. Tests that don't have a
// Redis client should pass nil — the service detects nil and degrades
// to the P1 "no cache, no pubsub" path.

// resolveRaw runs the 3-tier fallback ladder for an arbitrary key and
// returns either the raw DB value bytes (when present), or nil with
// the boolean fromDB=false to signal the caller to consult ENV / default.
//
// P2: cache-first. If the preload finished and the cache has an entry
// for this key, return it. Cache misses (key absent) are AUTHORITATIVE
// when loaded.IsTrue — preload populated every existing row, and any
// subsequent Update would have updated the cache inline. So a miss
// after preload means the row genuinely doesn't exist and we should
// skip the DB query entirely. Before preload finishes we still consult
// the DB to avoid a "cold-start serves defaults" surprise.
//
// Errors at the DB layer degrade to ENV/default with a warning log —
// upstream business code (file upload, etc.) gets a usable answer
// instead of a 500. This is the deliberate degradation policy spelled
// out in the interface comment.
func (s *systemSettingService) resolveRaw(ctx context.Context, key string) (raw types.JSON, fromDB bool) {
	spec, known := registry[key]
	if s.loaded.Load() {
		s.mu.RLock()
		row, ok := s.cache[key]
		s.mu.RUnlock()
		if ok && row != nil {
			if known && isBootstrapDefaultRow(row, spec) {
				return nil, false
			}
			return row.Value, true
		}
		// Cache populated and key not present → authoritative miss.
		return nil, false
	}
	// Pre-warmup path: hit the DB so a request that lands in the
	// startup window doesn't get the env/default surprise.
	row, err := s.repo.Get(ctx, key)
	if err != nil {
		logger.Warnf(ctx, "[system_settings] resolve %q failed, falling through to env/default: %v", key, err)
		return nil, false
	}
	if row == nil {
		return nil, false
	}
	if known && isBootstrapDefaultRow(row, spec) {
		return nil, false
	}
	return row.Value, true
}

// GetInt resolves an int64 setting. Priority: DB > ENV > def. Returns
// def on every error path so business code never has to handle the
// "the settings store is broken" case.
func (s *systemSettingService) GetInt(ctx context.Context, key string, envName string, def int64) int64 {
	if raw, ok := s.resolveRaw(ctx, key); ok {
		// Try canonical number form first.
		var n int64
		if err := json.Unmarshal(raw, &n); err == nil {
			return n
		}
		// Tolerate `"42"` so hand-edited rows still work.
		var quoted string
		if err := json.Unmarshal(raw, &quoted); err == nil {
			if v, err := strconv.ParseInt(quoted, 10, 64); err == nil {
				return v
			}
		}
		logger.Warnf(ctx, "[system_settings] %q: cannot parse %s as int, falling back", key, string(raw))
	}
	if envName != "" {
		if v := os.Getenv(envName); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				return n
			}
		}
	}
	return def
}

// GetFloat resolves a float setting. Same priority + degradation as
// GetInt; tolerates `"0.25"` (hand-edited row / legacy ENV) as well as
// the canonical JSON number form.
func (s *systemSettingService) GetFloat(ctx context.Context, key string, envName string, def float64) float64 {
	if raw, ok := s.resolveRaw(ctx, key); ok {
		var f float64
		if err := json.Unmarshal(raw, &f); err == nil {
			return f
		}
		// Tolerate `"0.25"` so hand-edited rows still work.
		var quoted string
		if err := json.Unmarshal(raw, &quoted); err == nil {
			if v, err := strconv.ParseFloat(strings.TrimSpace(quoted), 64); err == nil {
				return v
			}
		}
		logger.Warnf(ctx, "[system_settings] %q: cannot parse %s as float, falling back", key, string(raw))
	}
	if envName != "" {
		if v := os.Getenv(envName); v != "" {
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				return f
			}
		}
	}
	return def
}

// GetString resolves a string setting. Same priority + degradation as GetInt.
func (s *systemSettingService) GetString(ctx context.Context, key string, envName string, def string) string {
	if raw, ok := s.resolveRaw(ctx, key); ok {
		var v string
		if err := json.Unmarshal(raw, &v); err == nil {
			return v
		}
		logger.Warnf(ctx, "[system_settings] %q: cannot parse %s as string, falling back", key, string(raw))
	}
	if envName != "" {
		if v := os.Getenv(envName); v != "" {
			return v
		}
	}
	return def
}

// GetBool resolves a bool setting. Same priority + degradation as
// GetInt: DB > ENV > def.
//
// ENV parsing is deliberately looser than strconv.ParseBool: operators
// have historically written "yes"/"no"/"on"/"off" for these knobs (e.g.
// WEKNORA_HOUSEKEEPING_ENABLED=off), and strconv.ParseBool rejects all
// four. Before parseBoolLoose existed those values fell through to the
// default, which silently REVERSED the operator's intent — an "off" that
// turned the feature back on. Anything strconv accepts is still accepted.
func (s *systemSettingService) GetBool(ctx context.Context, key string, envName string, def bool) bool {
	if raw, ok := s.resolveRaw(ctx, key); ok {
		var v bool
		if err := json.Unmarshal(raw, &v); err == nil {
			return v
		}
		logger.Warnf(ctx, "[system_settings] %q: cannot parse %s as bool, falling back", key, string(raw))
	}
	if envName != "" {
		if v := os.Getenv(envName); v != "" {
			if b, ok := parseBoolLoose(v); ok {
				return b
			}
		}
	}
	return def
}

// parseBoolLoose accepts everything strconv.ParseBool does plus the
// word forms operators actually type: yes/no/on/off (case-insensitive,
// surrounding whitespace ignored). Reports false when the value is
// unrecognised so callers fall through to their default.
func parseBoolLoose(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "yes", "on":
		return true, true
	case "no", "off":
		return false, true
	}
	if b, err := strconv.ParseBool(strings.TrimSpace(raw)); err == nil {
		return b, true
	}
	return false, false
}

// GetStringList resolves a []string setting. Priority: DB > ENV > def.
//
// At the ENV level the value is parsed as a comma-separated string
// (matches the legacy SSRF_WHITELIST format and means operators don't
// have to learn a new convention to migrate). Whitespace around each
// entry is trimmed; empty entries are dropped. The returned slice is
// always non-nil so callers can iterate without a nil check.
//
// Same degradation policy as the other Get*: a DB-layer error logs a
// warning and falls through to ENV/default, so consumer paths
// (SSRF check, etc.) never have to handle "settings store broken".
func (s *systemSettingService) GetStringList(ctx context.Context, key string, envName string, def []string) []string {
	if raw, ok := s.resolveRaw(ctx, key); ok {
		var v []string
		if err := json.Unmarshal(raw, &v); err == nil {
			if v == nil {
				v = []string{}
			}
			return v
		}
		logger.Warnf(ctx, "[system_settings] %q: cannot parse %s as string_list, falling back", key, string(raw))
	}
	if envName != "" {
		if raw := os.Getenv(envName); raw != "" {
			out := make([]string, 0, 4)
			for _, entry := range strings.Split(raw, ",") {
				entry = strings.TrimSpace(entry)
				if entry != "" {
					out = append(out, entry)
				}
			}
			return out
		}
	}
	if def == nil {
		return []string{}
	}
	return def
}

// List returns all known settings for the management UI. Persisted rows
// are enriched with registry metadata. Registry keys without a saved DB
// override are returned as virtual rows using the effective fallback
// value (ENV/config/default), so merely migrating the schema never
// changes runtime behaviour.
func (s *systemSettingService) List(ctx context.Context) ([]*types.SystemSetting, error) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]*types.SystemSetting, len(rows))
	out := make([]*types.SystemSetting, 0, len(rows)+len(registry))
	for _, row := range rows {
		byKey[row.Key] = row
	}
	// asynq.concurrency was the old fixed-ratio aggregate. Keeping an old DB
	// row visible would suggest it still controls runtime capacity, so retire it
	// explicitly while preserving all genuinely unknown rows for diagnostics.
	delete(byKey, "asynq.concurrency")

	keys := make([]string, 0, len(registry))
	for key := range registry {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		spec := registry[key]
		if row := byKey[key]; row != nil {
			row.Enum = spec.Enum
			if isBootstrapDefaultRow(row, spec) {
				row.Value = s.fallbackJSONForSpec(key, spec)
			}
			out = append(out, row)
			delete(byKey, key)
			continue
		}
		out = append(out, s.virtualSetting(key, spec))
	}

	// Preserve out-of-band rows so operators can still see unexpected
	// data instead of having it disappear from the UI.
	extraKeys := make([]string, 0, len(byKey))
	for key := range byKey {
		extraKeys = append(extraKeys, key)
	}
	sort.Strings(extraKeys)
	for _, key := range extraKeys {
		out = append(out, byKey[key])
	}
	return out, nil
}

// Get returns one row by key. Used by the management UI's "load before
// edit" pattern. Returns (nil, nil) when missing (unknown-key handling
// is done at the handler layer for nicer 404 vs 200-with-default UX).
//
// Enriches the row with registry-side `Enum` for the same UI reason
// as List.
func (s *systemSettingService) Get(ctx context.Context, key string) (*types.SystemSetting, error) {
	spec, ok := registry[key]
	if !ok {
		return nil, fmt.Errorf("unknown setting key %q", key)
	}
	row, err := s.repo.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if row != nil {
		row.Enum = spec.Enum
		if isBootstrapDefaultRow(row, spec) {
			row.Value = s.fallbackJSONForSpec(key, spec)
		}
		return row, nil
	}
	return s.virtualSetting(key, spec), nil
}

func (s *systemSettingService) virtualSetting(key string, spec settingSpec) *types.SystemSetting {
	category := spec.Category
	if category == "" {
		category = "general"
	}
	return &types.SystemSetting{
		Key:             key,
		Value:           s.fallbackJSONForSpec(key, spec),
		ValueType:       spec.Type,
		Category:        category,
		Description:     spec.Description,
		IsSecret:        false,
		RequiresRestart: spec.RequiresRestart,
		LastModifiedBy:  "",
		Enum:            spec.Enum,
	}
}

func (s *systemSettingService) fallbackJSONForSpec(key string, spec settingSpec) types.JSON {
	if spec.EnvName != "" {
		if raw := strings.TrimSpace(os.Getenv(spec.EnvName)); raw != "" {
			switch spec.Type {
			case "int":
				if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
					if encoded, err := encodeForType(spec.Type, n); err == nil {
						return encoded
					}
				}
			case "float":
				if f, err := strconv.ParseFloat(raw, 64); err == nil {
					if encoded, err := encodeForType(spec.Type, f); err == nil {
						return encoded
					}
				}
			case "string":
				if encoded, err := encodeForType(spec.Type, raw); err == nil {
					return encoded
				}
			case "bool":
				if b, err := strconv.ParseBool(raw); err == nil {
					if encoded, err := encodeForType(spec.Type, b); err == nil {
						return encoded
					}
				}
			case "string_list":
				entries := make([]string, 0, 4)
				for _, entry := range strings.Split(raw, ",") {
					entry = strings.TrimSpace(entry)
					if entry != "" {
						entries = append(entries, entry)
					}
				}
				if encoded, err := encodeForType(spec.Type, entries); err == nil {
					return encoded
				}
			}
		}
	}
	if key == "auth.registration_mode" {
		mode := config.AuthRegistrationModeSelfServe
		if s.cfg != nil && s.cfg.Auth != nil {
			if configured := strings.TrimSpace(s.cfg.Auth.RegistrationMode); configured != "" {
				mode = configured
			}
		}
		if encoded, err := encodeForType(spec.Type, mode); err == nil {
			return encoded
		}
	}
	encoded, err := encodeDefault(spec)
	if err != nil {
		return types.JSON(`null`)
	}
	return encoded
}

// isBootstrapDefaultRow treats old migration/service seeded defaults as
// placeholders rather than operator-owned overrides. Those rows used an
// empty last_modified_by and the registry default value, so deployments
// that already ran the unsafe seed regain the intended ENV/config
// fallback behaviour until a SystemAdmin explicitly saves a value.
func isBootstrapDefaultRow(row *types.SystemSetting, spec settingSpec) bool {
	if row == nil || strings.TrimSpace(row.LastModifiedBy) != "" {
		return false
	}
	def, err := encodeDefault(spec)
	if err != nil {
		return false
	}
	return jsonEqual(row.Value, def)
}

func jsonEqual(a, b types.JSON) bool {
	var ca, cb bytes.Buffer
	if err := json.Compact(&ca, a); err != nil {
		return false
	}
	if err := json.Compact(&cb, b); err != nil {
		return false
	}
	return bytes.Equal(ca.Bytes(), cb.Bytes())
}

// Update validates and persists a new value. Steps:
//  1. Look up the registry spec — reject unknown keys with 400 semantics.
//  2. Coerce + validate the rawValue against spec.Type. Numeric inputs
//     from JSON unmarshalling arrive as float64; we accept both int64
//     and float64 for "int" and round-trip through strconv to surface
//     rejection of e.g. floats like 3.14 cleanly.
//  3. Build the SystemSetting row, write via repo.Upsert.
//  4. Emit an audit log carrying old + new values for forensics.
//
// Returns the persisted row (re-read from DB so updated_at /
// last_modified_by are fresh).
func (s *systemSettingService) Update(ctx context.Context, key string, rawValue any) (*types.SystemSetting, error) {
	spec, ok := registry[key]
	if !ok {
		return nil, fmt.Errorf("unknown setting key %q", key)
	}

	encoded, err := encodeForType(spec.Type, rawValue)
	if err != nil {
		return nil, fmt.Errorf("invalid value for %q (expected %s): %w", key, spec.Type, err)
	}

	// Enum check: only meaningful for "string". Compare the decoded
	// string against the registry-declared whitelist. Done after
	// encodeForType so we know the raw value passed type validation.
	//
	// Use a local name `str` rather than `s` to avoid shadowing the
	// outer `*systemSettingService` receiver — the previous version
	// relied on the shadow being unused inside the block, which was a
	// trap for future edits.
	if len(spec.Enum) > 0 && spec.Type == "string" {
		str, _ := rawValue.(string)
		allowed := false
		for _, opt := range spec.Enum {
			if str == opt {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("invalid value for %q: %q not in %v", key, str, spec.Enum)
		}
	}

	// Per-key structural validation. encodeForType already enforces the
	// value_type contract (int / string / bool / string_list); this hook
	// is for keys whose value carries an internal grammar that, if
	// silently malformed, would either fail to take effect at runtime
	// (e.g. SSRF whitelist parser drops bad CIDRs) or actively
	// mis-classify input. Reject with 400 so the UI can show a clear
	// inline error instead of "saved" + nothing happens.
	if err := validateRegistryEntry(key, rawValue); err != nil {
		return nil, fmt.Errorf("invalid value for %q: %w", key, err)
	}

	// Capture pre-image for the audit log — pulled fresh, not from
	// any cache, so concurrent admin edits race-fairly (last writer
	// wins, audit reflects what was actually replaced).
	prev, _ := s.repo.Get(ctx, key)
	var oldValue types.JSON
	var category, description string
	var isSecret, requiresRestart bool
	if prev != nil {
		oldValue = prev.Value
		category = prev.Category
		description = prev.Description
		isSecret = prev.IsSecret
		requiresRestart = prev.RequiresRestart
	} else {
		// First-write path: derive category/description from registry
		// so the row matches the seeded migration shape. Operators can
		// hand-edit description in the DB if they want richer copy.
		category = spec.Category
		if category == "" {
			category = "general"
		}
		description = spec.Description
		requiresRestart = spec.RequiresRestart
	}

	row := &types.SystemSetting{
		Key:             key,
		Value:           encoded,
		ValueType:       spec.Type,
		Category:        category,
		Description:     description,
		IsSecret:        isSecret,
		RequiresRestart: requiresRestart,
		LastModifiedBy:  auditActor(ctx),
	}
	if err := s.repo.Upsert(ctx, row); err != nil {
		return nil, fmt.Errorf("upsert system setting %q: %w", key, err)
	}

	// Re-read so caller sees DB-side defaults (id, updated_at) populated.
	persisted, err := s.repo.Get(ctx, key)
	if err != nil || persisted == nil {
		// Don't fail the operation just because the read-back hiccuped —
		// the upsert already succeeded. Return the optimistic value.
		persisted = row
	}
	// Mirror the registry-derived enrichment that List/Get apply.
	// Without this, the persisted row hands back an empty Enum field
	// to the API client, which causes the management UI to swap a
	// t-select for a plain text input on the post-save patch and
	// display the raw enum value (e.g. "self_serve") until the next
	// full reload. Other registry-only fields (Type via ValueType is
	// already on the row; Description/Category are persisted) don't
	// need the same fix-up because they're stored on the row itself.
	persisted.Enum = spec.Enum

	// Update local cache inline so this replica's next read sees the
	// new value without waiting for the pubsub roundtrip. Other replicas
	// pick it up via publishChange below.
	s.mu.Lock()
	s.cache[key] = persisted
	s.mu.Unlock()

	// Push to side-effect bridges (e.g. utils.SetSSRFWhitelistFromRaw).
	s.dispatchSideEffects(ctx, key)

	s.publishChange(ctx, key)
	s.emitChangeAudit(ctx, key, spec.Type, oldValue, encoded)
	return persisted, nil
}

// Reset deletes the DB override for `key` so the resolver falls back
// to ENV / built-in default. Idempotent — deleting a key that was
// never persisted is treated as success (no audit row written, since
// nothing actually changed) so retries from the UI can't pile up
// noise. Mirrors Update's cache+pubsub+audit+side-effect plumbing on
// the success path so other replicas drop the entry too.
//
// Unknown keys still 400 — we don't want a typo on the URL to silently
// pretend it cleared something.
func (s *systemSettingService) Reset(ctx context.Context, key string) error {
	spec, ok := registry[key]
	if !ok {
		return fmt.Errorf("unknown setting key %q", key)
	}

	// Capture pre-image for the audit log before the row vanishes.
	prev, _ := s.repo.Get(ctx, key)

	deleted, err := s.repo.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("delete system setting %q: %w", key, err)
	}

	// Drop from the local cache regardless of `deleted` so a stale
	// entry from before the row existed (shouldn't happen, but cheap
	// to defend) gets cleared too.
	s.mu.Lock()
	delete(s.cache, key)
	s.mu.Unlock()

	// Side-effect bridges and pubsub fire even for no-op resets so
	// peers that may have a stale cached row converge. publishChange
	// is best-effort; reload() on the peer side handles "row absent"
	// by deleting the cache entry, which is what we want here.
	s.dispatchSideEffects(ctx, key)
	s.publishChange(ctx, key)

	// Only emit an audit row on real deletions. The new_value field
	// is intentionally null to flag this as a reset (vs an Update
	// which always writes a concrete new_value).
	if deleted && prev != nil {
		s.emitChangeAudit(ctx, key, spec.Type, prev.Value, nil)
	}
	return nil
}

// SubscribeRedis starts a single goroutine that subscribes to the
// pubsub channel and refreshes the local cache when peers publish
// changes. Idempotent (subOnce). When Redis is nil (lite mode) returns
// nil immediately — single-replica deployments don't need pubsub
// because Update already writes the local cache inline.
//
// The subscriber loop runs until ctx is cancelled (server shutdown).
// On Redis disconnection we reconnect with exponential backoff up to
// 30s, mirroring the approval/gate.go convention so operators see the
// same recovery behaviour across pubsub-using subsystems.
func (s *systemSettingService) SubscribeRedis(ctx context.Context) error {
	if s.rdb == nil {
		logger.Infof(ctx, "[system_settings] Redis not configured, skipping pubsub (single-replica mode)")
		return nil
	}
	s.subOnce.Do(func() {
		go s.runSubscribeLoop(ctx)
	})
	return nil
}

// runSubscribeLoop is the long-running goroutine spawned by
// SubscribeRedis. Reconnects on transient errors; exits on ctx.Done().
func (s *systemSettingService) runSubscribeLoop(ctx context.Context) {
	channel := pubsubChannel()
	logger.Infof(ctx, "[system_settings] subscribed to %s (instance=%s)", channel, s.instanceID[:8])

	const maxBackoff = 30 * time.Second
	backoff := time.Second
	for {
		// ctx may already be cancelled (server shutting down before
		// pubsub became active).
		if ctx.Err() != nil {
			return
		}
		sub := s.rdb.Subscribe(ctx, channel)
		// Verify the subscription is active so a publish-and-disconnect
		// race doesn't silently drop the first message.
		if _, err := sub.Receive(ctx); err != nil {
			logger.Warnf(ctx, "[system_settings] subscribe %s: %v (retry in %s)", channel, err, backoff)
			_ = sub.Close()
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			continue
		}
		backoff = time.Second // reset after a healthy connection
		ch := sub.Channel()
		s.consumeMessages(ctx, ch)
		_ = sub.Close()
		// consumeMessages returns either because ctx is done or the
		// subscription was torn down; loop back and try again.
	}
}

// consumeMessages drains the pubsub channel, dispatching to reload()
// for every key the peer says changed. Returns when the channel
// closes (Redis disconnect) or ctx is done.
func (s *systemSettingService) consumeMessages(ctx context.Context, ch <-chan *redis.Message) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var m changeMessage
			if err := json.Unmarshal([]byte(msg.Payload), &m); err != nil {
				logger.Warnf(ctx, "[system_settings] bad pubsub payload: %v", err)
				continue
			}
			// Skip our own publish — the local cache is already fresh
			// (Update wrote it inline). Without this every Update would
			// trigger a redundant DB roundtrip on the publishing replica.
			if m.OriginID == s.instanceID {
				continue
			}
			s.reload(ctx, m.Key)
		}
	}
}

// emitChangeAudit writes one audit row per successful Update. Best-
// effort — a nil audit service or a write failure does not bubble up.
// This mirrors tenantMemberService.emitAudit's failure semantics: the
// business op (config update) succeeds even if audit is broken.
func (s *systemSettingService) emitChangeAudit(
	ctx context.Context, key, valueType string, oldValue, newValue types.JSON,
) {
	if s.audit == nil {
		return
	}
	details, _ := json.Marshal(map[string]any{
		"key":        key,
		"value_type": valueType,
		"old_value":  json.RawMessage(oldValue),
		"new_value":  json.RawMessage(newValue),
	})
	_ = s.audit.Log(ctx, &types.AuditLog{
		// tenant_id=0 marks the row as system-scope (the audit_logs
		// table itself is tenant-scoped; 0 is the convention for
		// platform-wide events).
		TenantID:    0,
		ActorUserID: auditActor(ctx),
		ActorRole:   "system_admin",
		Action:      types.AuditActionSystemSettingChanged,
		TargetType:  "system_setting",
		TargetID:    key,
		Outcome:     types.AuditOutcomeSuccess,
		Details:     types.JSON(details),
	})
}

// encodeForType validates rawValue against the declared type and
// returns the canonical JSON encoding for the DB. Rejects type
// mismatches (e.g. passing "abc" for an int field) with a clear error
// the handler can surface to the UI.
func encodeForType(declared string, rawValue any) (types.JSON, error) {
	switch declared {
	case "int":
		var n int64
		switch v := rawValue.(type) {
		case int:
			n = int64(v)
		case int32:
			n = int64(v)
		case int64:
			n = v
		case float64:
			// JSON unmarshalling delivers numbers as float64; reject
			// non-integer floats (e.g. 3.14) cleanly rather than
			// silently truncating.
			if v != float64(int64(v)) {
				return nil, fmt.Errorf("expected integer, got %v", v)
			}
			n = int64(v)
		case string:
			parsed, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("expected integer, got %q", v)
			}
			n = parsed
		default:
			return nil, fmt.Errorf("expected integer, got %T", rawValue)
		}
		b, _ := json.Marshal(n)
		return types.JSON(b), nil
	case "float":
		// 浮点设置项（如 fusion.reliability.weight）。JSON 解码把数字一律
		// 送成 float64，所以这里主要防的是「字符串数字」与 bool 之类的
		// 误传；字符串形式保留给从旧 ENV 值粘贴的场景。
		var f float64
		switch v := rawValue.(type) {
		case float64:
			f = v
		case float32:
			f = float64(v)
		case int:
			f = float64(v)
		case int32:
			f = float64(v)
		case int64:
			f = float64(v)
		case string:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				return nil, fmt.Errorf("expected number, got %q", v)
			}
			f = parsed
		default:
			return nil, fmt.Errorf("expected number, got %T", rawValue)
		}
		b, _ := json.Marshal(f)
		return types.JSON(b), nil
	case "string":
		v, ok := rawValue.(string)
		if !ok {
			return nil, fmt.Errorf("expected string, got %T", rawValue)
		}
		b, _ := json.Marshal(v)
		return types.JSON(b), nil
	case "bool":
		v, ok := rawValue.(bool)
		if !ok {
			return nil, fmt.Errorf("expected bool, got %T", rawValue)
		}
		b, _ := json.Marshal(v)
		return types.JSON(b), nil
	case "string_list":
		// Accept either a JSON array of strings (the canonical UI shape
		// — t-tag-input emits string[]) or a single comma-separated
		// string (operator pasting from a legacy ENV value). Reject
		// arrays containing non-strings to avoid silently coercing
		// `[1, 2]` into `["1", "2"]` — that hides typos.
		var entries []string
		switch v := rawValue.(type) {
		case []any:
			entries = make([]string, 0, len(v))
			for i, item := range v {
				s, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("expected string at index %d, got %T", i, item)
				}
				s = strings.TrimSpace(s)
				if s != "" {
					entries = append(entries, s)
				}
			}
		case []string:
			entries = make([]string, 0, len(v))
			for _, s := range v {
				s = strings.TrimSpace(s)
				if s != "" {
					entries = append(entries, s)
				}
			}
		case string:
			for _, s := range strings.Split(v, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					entries = append(entries, s)
				}
			}
			if entries == nil {
				entries = []string{}
			}
		default:
			return nil, fmt.Errorf("expected string array, got %T", rawValue)
		}
		b, _ := json.Marshal(entries)
		return types.JSON(b), nil
	default:
		return nil, errors.New("unknown declared type: " + declared)
	}
}

// validateRegistryEntry runs key-specific structural validation that
// goes beyond the type contract enforced by encodeForType. It is called
// by Update right after the type/enum checks; nil error means the value
// is acceptable for persistence.
//
// Adding new validators:
//   - keep them strict (reject silently-broken input rather than fixing
//     it server-side — the user should see what they typed),
//   - keep them deterministic (no DNS lookups, no env reads),
//   - keep error messages user-facing (the handler surfaces them as the
//     400 body verbatim).
func validateRegistryEntry(key string, rawValue any) error {
	switch key {
	case "asynq.core_concurrency", "asynq.postprocess_concurrency",
		"asynq.enrichment_concurrency", "asynq.maintenance_concurrency",
		"asynq.shared_concurrency", "asynq.wiki_concurrency":
		n, err := coerceToPositiveInt64(rawValue)
		if err != nil {
			return err
		}
		if n < 1 {
			return errors.New("concurrency must be at least 1")
		}
	case "ssrf.whitelist":
		// Coerce into the same shape encodeForType produced. We don't
		// look at the encoded JSON because that's already canonicalised
		// — easier to validate the raw input the user typed.
		entries, err := coerceToStringSlice(rawValue)
		if err != nil {
			return err
		}
		return utils.ValidateSSRFWhitelistEntries(entries)
	case "fusion.reliability.weight":
		// ADR-005 硬约束：可靠度权重不得低于 0.25。迁移前这条约束只写在
		// 配置中心（Python），而 Go 侧读的是 STARKB_RELIABILITY_WEIGHT
		// 环境变量，可以绕过；现在唯一入口是本键，约束才真正生效。
		f, err := coerceToFloat64(rawValue)
		if err != nil {
			return err
		}
		if f < 0.25 {
			return errors.New("reliability weight must be at least 0.25 (ADR-005)")
		}
		if f > 1 {
			return errors.New("reliability weight must not exceed 1")
		}
	case "graph.channel.top_k", "graph.channel.chunks_per_hit":
		n, err := coerceToPositiveInt64(rawValue)
		if err != nil {
			return err
		}
		if n < 1 {
			return errors.New("must be at least 1")
		}
	case "graph.channel.timeout_s":
		n, err := coerceToPositiveInt64(rawValue)
		if err != nil {
			return err
		}
		// 上限沿用 graphRecallTimeout 的历史约束（>120 视为误配）。
		if n < 1 || n > 120 {
			return errors.New("timeout must be between 1 and 120 seconds")
		}
	case types.SettingKeyStarkbGraphBuildMaxAsync:
		n, err := coerceToPositiveInt64(rawValue)
		if err != nil {
			return err
		}
		// >8 极易触发服务商并发限流（429 风暴），视为误配。
		if n < 1 || n > 8 {
			return errors.New("max_async must be between 1 and 8")
		}
	case types.SettingKeyStarkbGraphBuildLLMIntervalMS:
		n, err := coerceToPositiveInt64(rawValue)
		if err != nil {
			return err
		}
		// 0 = 无间隔（不推荐）；上限防误配成秒级。
		if n < 0 || n > 10000 {
			return errors.New("llm_interval_ms must be between 0 and 10000")
		}
	case "agent.llm_timeout", "agent.tool_approval_timeout",
		"docreader.call_timeout", "document.process_timeout":
		s, ok := rawValue.(string)
		if !ok {
			return fmt.Errorf("expected string, got %T", rawValue)
		}
		return validateDurationValue(s)
	}
	return nil
}

// coerceToFloat64 accepts the same input shapes as encodeForType's
// "float" branch (JSON delivers numbers as float64; strings are kept so
// operators can paste a legacy ENV value).
func coerceToFloat64(rawValue any) (float64, error) {
	switch v := rawValue.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, fmt.Errorf("expected number, got %q", v)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("expected number, got %T", rawValue)
	}
}

// validateDurationValue accepts Go duration syntax ("30m", "2h") or a
// bare positive number interpreted as seconds — matching the legacy ENV
// parsing in applyKnowledgeBaseEnvOverrides / applyAgentEnvOverrides, so
// a value pasted from the old environment variable keeps working.
func validateDurationValue(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return errors.New("duration must not be empty")
	}
	if d, err := time.ParseDuration(raw); err == nil {
		if d <= 0 {
			return errors.New("duration must be positive")
		}
		return nil
	}
	if sec, err := strconv.Atoi(raw); err == nil && sec > 0 {
		return nil
	}
	return fmt.Errorf("invalid duration %q: use Go duration (e.g. 30m, 2h) or a positive number of seconds", raw)
}

// parseDurationSeconds 把 Go duration（"30m"）或裸秒数（"600"）解析为整秒。
// 与 validateDurationValue 同一口径，供需要把时长推给别处的桥接使用。
func parseDurationSeconds(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return int(d / time.Second), true
	}
	if sec, err := strconv.Atoi(raw); err == nil && sec > 0 {
		return sec, true
	}
	return 0, false
}

// coerceToPositiveInt64 accepts int / int64 / float64 from JSON decoding.
func coerceToPositiveInt64(rawValue any) (int64, error) {
	switch v := rawValue.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		if v != float64(int64(v)) {
			return 0, errors.New("expected integer value")
		}
		return int64(v), nil
	default:
		return 0, fmt.Errorf("expected integer, got %T", rawValue)
	}
}

// coerceToStringSlice mirrors the input shapes accepted by
// encodeForType for "string_list": []any of strings, []string, or a
// comma-separated string. Returns the trimmed, empty-stripped result.
//
// Kept private because the only caller is validateRegistryEntry; the
// main encode path has its own (slightly different) coercion that
// preserves rejection of non-string elements at a specific index for
// clearer error messages.
func coerceToStringSlice(rawValue any) ([]string, error) {
	switch v := rawValue.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string at index %d, got %T", i, item)
			}
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	case string:
		out := make([]string, 0, 4)
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected string array, got %T", rawValue)
	}
}
