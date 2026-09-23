package chat

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeLengthChat struct {
	calls      int
	finishSeq  []string // 每次调用返回的 finish_reason
	seenBudget []int    // 每次调用看到的 MaxTokens
}

func (f *fakeLengthChat) Chat(_ context.Context, _ []Message, opts *ChatOptions) (*types.ChatResponse, error) {
	idx := f.calls
	if idx >= len(f.finishSeq) {
		idx = len(f.finishSeq) - 1
	}
	f.calls++
	budget := 0
	if opts != nil {
		budget = opts.MaxTokens
	}
	f.seenBudget = append(f.seenBudget, budget)
	return &types.ChatResponse{Content: "part", FinishReason: f.finishSeq[idx]}, nil
}

func (f *fakeLengthChat) ChatStream(_ context.Context, _ []Message, _ *ChatOptions) (<-chan types.StreamResponse, error) {
	return nil, nil
}
func (f *fakeLengthChat) GetModelName() string { return "fake" }
func (f *fakeLengthChat) GetModelID() string   { return "fake-id" }

// B2：首次截断 → 加倍预算重试一次 → 成功返回。
func TestLengthRetryRetriesOnceWithDoubledBudget(t *testing.T) {
	fake := &fakeLengthChat{finishSeq: []string{"length", "stop"}}
	c := &lengthRetryChat{next: fake}
	opts := &ChatOptions{MaxTokens: 1000}
	resp, err := c.Chat(context.Background(), nil, opts)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.FinishReason != "stop" {
		t.Fatalf("expected stop finish, got %q", resp.FinishReason)
	}
	if fake.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", fake.calls)
	}
	if len(fake.seenBudget) != 2 || fake.seenBudget[1] != 2000 {
		t.Fatalf("expected retry budget 2000, got %v", fake.seenBudget)
	}
}

// B2：重试后仍截断 → 显式报错而非半截文本。
func TestLengthRetryErrorsWhenStillTruncated(t *testing.T) {
	fake := &fakeLengthChat{finishSeq: []string{"length"}}
	c := &lengthRetryChat{next: fake}
	_, err := c.Chat(context.Background(), nil, &ChatOptions{MaxTokens: 1000})
	if err == nil {
		t.Fatal("expected error when retry still truncated")
	}
	if fake.calls != 2 {
		t.Fatalf("expected exactly 1 retry (2 calls), got %d", fake.calls)
	}
}

// B2：未配置 MaxTokens 时按 4096 起算加倍。
func TestLengthRetryDefaultBudget(t *testing.T) {
	fake := &fakeLengthChat{finishSeq: []string{"length", "stop"}}
	c := &lengthRetryChat{next: fake}
	if _, err := c.Chat(context.Background(), nil, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if fake.seenBudget[1] != minRetryBudget*2 {
		t.Fatalf("expected default retry budget %d, got %d", minRetryBudget*2, fake.seenBudget[1])
	}
}

// B2：正常结束不触发重试。
func TestLengthRetryNoOpOnStop(t *testing.T) {
	fake := &fakeLengthChat{finishSeq: []string{"stop"}}
	c := &lengthRetryChat{next: fake}
	if _, err := c.Chat(context.Background(), nil, &ChatOptions{MaxTokens: 10}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if fake.calls != 1 {
		t.Fatalf("expected single call, got %d", fake.calls)
	}
}

// B2 补丁：显式小预算（连接测试 MaxTokens=1）是探针——length 原样透传，不重试不报错
func TestLengthRetryPassesThroughProbeBudget(t *testing.T) {
	fake := &fakeLengthChat{finishSeq: []string{"length"}}
	c := &lengthRetryChat{next: fake}
	resp, err := c.Chat(context.Background(), nil, &ChatOptions{MaxTokens: 1})
	if err != nil {
		t.Fatalf("probe budget should not error: %v", err)
	}
	if resp == nil || resp.FinishReason != "length" {
		t.Fatalf("probe response should pass through, got %+v", resp)
	}
	if fake.calls != 1 {
		t.Fatalf("probe must not retry, got %d calls", fake.calls)
	}
}
