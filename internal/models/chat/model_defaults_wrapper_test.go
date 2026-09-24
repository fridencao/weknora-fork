package chat

// 模型行思考默认值装饰器的行为锁定：
//   - 调用方未表态 → 模型行 reasoning_effort / thinking_budget_tokens 生效；
//   - 调用方显式设置（含 "off" 与任意强度）→ 模型行配置不覆盖；
//   - 模型行未配置 → 装饰器不启用，opts 原样透传（含 nil opts）。

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/api"
	"github.com/Tencent/WeKnora/internal/types"
)

// captureChat 记录装饰器传下来的 opts。不嵌入 Chat 接口——实现全接口，
// 避免「嵌入字段 Chat 与方法 Chat 同名」的非法声明。
type captureChat struct{ lastOpts *ChatOptions }

func (c *captureChat) Chat(ctx context.Context, messages []Message,
	opts *ChatOptions,
) (*types.ChatResponse, error) {
	c.lastOpts = opts
	return &types.ChatResponse{}, nil
}

func (c *captureChat) ChatStream(ctx context.Context, messages []Message,
	opts *ChatOptions,
) (<-chan types.StreamResponse, error) {
	c.lastOpts = opts
	return nil, nil
}

func (c *captureChat) GetModelName() string { return "capture" }
func (c *captureChat) GetModelID() string   { return "capture-id" }

func TestWrapChatModelDefaultsAppliesWhenCallerSilent(t *testing.T) {
	capture := &captureChat{}
	wrapped, err := wrapChatModelDefaults(capture, &ChatConfig{
		ReasoningEffort:      api.ReasoningOff,
		ThinkingBudgetTokens: 1024,
	}, nil)
	if err != nil {
		t.Fatalf("wrap failed: %v", err)
	}
	opts := &ChatOptions{Temperature: 0.2}
	if _, err := wrapped.Chat(context.Background(), nil, opts); err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	if opts.ReasoningEffort != api.ReasoningOff {
		t.Fatalf("reasoning_effort = %q, want %q", opts.ReasoningEffort, api.ReasoningOff)
	}
	if opts.ThinkingBudgetTokens != 1024 {
		t.Fatalf("thinking_budget_tokens = %d, want 1024", opts.ThinkingBudgetTokens)
	}
}

func TestWrapChatModelDefaultsRespectsCallerPreference(t *testing.T) {
	capture := &captureChat{}
	wrapped, _ := wrapChatModelDefaults(capture, &ChatConfig{
		ReasoningEffort:      api.ReasoningOff,
		ThinkingBudgetTokens: 1024,
	}, nil)
	opts := &ChatOptions{ReasoningEffort: api.ReasoningHigh, ThinkingBudgetTokens: 64}
	if _, err := wrapped.Chat(context.Background(), nil, opts); err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	if opts.ReasoningEffort != api.ReasoningHigh {
		t.Fatalf("reasoning_effort = %q, want caller value %q", opts.ReasoningEffort, api.ReasoningHigh)
	}
	if opts.ThinkingBudgetTokens != 64 {
		t.Fatalf("thinking_budget_tokens = %d, want caller value 64", opts.ThinkingBudgetTokens)
	}
}

func TestWrapChatModelDefaultsPassthroughWhenUnconfigured(t *testing.T) {
	capture := &captureChat{}
	wrapped, _ := wrapChatModelDefaults(capture, &ChatConfig{}, nil)
	if wrapped != Chat(capture) {
		t.Fatalf("unconfigured config must return the inner Chat unchanged")
	}
}

func TestWrapChatModelDefaultsNilOpts(t *testing.T) {
	capture := &captureChat{}
	wrapped, _ := wrapChatModelDefaults(capture, &ChatConfig{ReasoningEffort: api.ReasoningLow}, nil)
	if _, err := wrapped.Chat(context.Background(), nil, nil); err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	if capture.lastOpts != nil {
		t.Fatalf("nil opts must stay nil (protocol applies its own defaults)")
	}
}
