package chatpipeline

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

// stubKnowledgeRepo 只实现 KB→文档列举，其余接口方法嵌 nil（不触发）。
type stubKnowledgeRepo struct {
	interfaces.KnowledgeRepository
	docs  map[string][]*types.Knowledge
	errOn map[string]bool
}

func (s *stubKnowledgeRepo) ListKnowledgeByKnowledgeBaseID(
	_ context.Context, _ uint64, kbID string,
) ([]*types.Knowledge, error) {
	if s.errOn[kbID] {
		return nil, context.DeadlineExceeded
	}
	return s.docs[kbID], nil
}

func TestExpandAllowedDocIDs_MapsKBToDocIDs(t *testing.T) {
	t.Parallel()

	repo := &stubKnowledgeRepo{docs: map[string][]*types.Knowledge{
		"kb-1": {{ID: "doc-a"}, {ID: "doc-b"}},
		"kb-2": {{ID: "doc-c"}},
	}}

	allowed := expandAllowedDocIDs(context.Background(), repo, 1, []string{"kb-1", "kb-2"})

	require.Len(t, allowed, 3)
	require.Contains(t, allowed, "doc-a")
	require.Contains(t, allowed, "doc-b")
	require.Contains(t, allowed, "doc-c")
	// 关键回归断言：KB ID 本身不再是合法回跳键（M2 语义错位）。
	require.NotContains(t, allowed, "kb-1")
}

func TestExpandAllowedDocIDs_DegradesPerKB(t *testing.T) {
	t.Parallel()

	repo := &stubKnowledgeRepo{
		docs:  map[string][]*types.Knowledge{"kb-ok": {{ID: "doc-x"}}},
		errOn: map[string]bool{"kb-bad": true},
	}

	allowed := expandAllowedDocIDs(context.Background(), repo, 1, []string{"kb-bad", "kb-ok"})

	require.Equal(t, map[string]struct{}{"doc-x": {}}, allowed)
}

func TestExpandAllowedDocIDs_EmptyScope(t *testing.T) {
	t.Parallel()

	repo := &stubKnowledgeRepo{docs: map[string][]*types.Knowledge{}}
	require.Empty(t, expandAllowedDocIDs(context.Background(), repo, 1, nil))
}
