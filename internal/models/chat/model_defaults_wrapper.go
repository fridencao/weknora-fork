package chat

// 模型行思考默认值装饰器。
//
// 模型编辑界面可以在模型行上配置思考强度默认（reasoning_effort，含 "off"
// 关闭）与思考预算默认（thinking_budget_tokens）。本装饰器把它们作为
// 「调用方未表态时的默认」合入每次请求的 Options：
//   - 调用方显式设置了 reasoning_effort / thinking（agent、会话、管线）→
//     原样保留，模型行配置不覆盖；
//   - 调用方未表态且模型行配置了强度/预算 → 填入默认值，协议层照常按
//     厂商阶梯编码（厂商不支持的档位由 ThinkingLevels.Clamp 收窄，不支持
//     off 的常思考模型会跳过发送，不产生拒绝）。
//
// 装饰位置：链路最内层（协议客户端之上、langfuse 之前）——纯字段合并，
// 无网络往返，不参与耗时统计。

import (
	"context"

	"github.com/Tencent/WeKnora/internal/models/api"
	"github.com/Tencent/WeKnora/internal/types"
)

type modelDefaultsChat struct {
	next Chat
	// effort 为空表示模型行未配置强度默认。
	effort api.ReasoningEffort
	// budget 为 0 表示模型行未配置预算默认。
	budget int
}

// applyDefaults 原地补全调用方未表态的思考偏好。
func (c *modelDefaultsChat) applyDefaults(opts *ChatOptions) {
	if opts == nil {
		return
	}
	if _, requested := opts.Reasoning(); !requested && c.effort != "" {
		opts.ReasoningEffort = c.effort
	}
	if opts.ThinkingBudgetTokens == 0 && c.budget > 0 {
		opts.ThinkingBudgetTokens = c.budget
	}
}

func (c *modelDefaultsChat) Chat(ctx context.Context, messages []Message,
	opts *ChatOptions,
) (*types.ChatResponse, error) {
	c.applyDefaults(opts)
	return c.next.Chat(ctx, messages, opts)
}

func (c *modelDefaultsChat) ChatStream(ctx context.Context, messages []Message,
	opts *ChatOptions,
) (<-chan types.StreamResponse, error) {
	c.applyDefaults(opts)
	return c.next.ChatStream(ctx, messages, opts)
}

func (c *modelDefaultsChat) GetModelName() string { return c.next.GetModelName() }

func (c *modelDefaultsChat) GetModelID() string { return c.next.GetModelID() }

// wrapChatModelDefaults 在模型行配置了任一思考默认时启用装饰器。
func wrapChatModelDefaults(c Chat, config *ChatConfig, err error) (Chat, error) {
	if err != nil {
		return nil, err
	}
	if config == nil || (config.ReasoningEffort == "" && config.ThinkingBudgetTokens <= 0) {
		return c, nil
	}
	return &modelDefaultsChat{
		next:   c,
		effort: config.ReasoningEffort,
		budget: config.ThinkingBudgetTokens,
	}, nil
}
