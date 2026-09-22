package chat

// B2（docs/09 WS3.2）：finish_reason=length 防御。
//
// WeKnora 此前不检查 finish_reason——输出被 max_tokens 截断时上层拿到半截文本
// 直接失败或入库（摘要空输出、实体抽取 JSON 半截解析失败的两类故障同源）。
// 本装饰器在非流式 Chat 检测到 length 时加倍 max_tokens 预算重试一次；重试后
// 仍截断则显式报错（而非静默返回半截文本）。摘要/抽取/问题生成等所有非流式
// 调用方自动覆盖；流式路径的截断语义由消费方处理，不在此拦截。
//
// 装饰位置：langfuse 之后、concurrency 之前——重试的两次 provider 往返共用
// 同一个并发槽位。

import (
	"context"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
)

// minRetryBudget 截断重试时的最小预算基数（MaxTokens 未显式配置时按此起算）。
const minRetryBudget = 4096

type lengthRetryChat struct {
	next Chat
}

func (c *lengthRetryChat) Chat(ctx context.Context, messages []Message,
	opts *ChatOptions,
) (*types.ChatResponse, error) {
	resp, err := c.next.Chat(ctx, messages, opts)
	if err != nil || resp == nil || resp.FinishReason != "length" {
		return resp, err
	}

	budget := minRetryBudget
	if opts != nil && opts.MaxTokens > 0 {
		budget = opts.MaxTokens
	}
	retryOpts := &ChatOptions{}
	if opts != nil {
		*retryOpts = *opts
	}
	retryOpts.MaxTokens = budget * 2
	retried, retryErr := c.next.Chat(ctx, messages, retryOpts)
	if retryErr != nil {
		return resp, err // 重试失败：返回首次结果与首次错误，语义不变
	}
	if retried != nil && retried.FinishReason == "length" {
		return nil, fmt.Errorf("LLM 输出被 max_tokens 截断（finish_reason=length，预算 %d 加倍重试后仍截断；model=%s）",
			budget, c.next.GetModelName())
	}
	return retried, nil
}

func (c *lengthRetryChat) ChatStream(ctx context.Context, messages []Message,
	opts *ChatOptions,
) (<-chan types.StreamResponse, error) {
	return c.next.ChatStream(ctx, messages, opts)
}

func (c *lengthRetryChat) GetModelName() string { return c.next.GetModelName() }

func (c *lengthRetryChat) GetModelID() string { return c.next.GetModelID() }

// wrapChatLengthRetry 检测截断并重试；包装失败时透传原错误。
func wrapChatLengthRetry(c Chat, err error) (Chat, error) {
	if err != nil {
		return nil, err
	}
	return &lengthRetryChat{next: c}, nil
}
