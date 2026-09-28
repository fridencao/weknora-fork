package service

// processRun / loadRegulationTextFromDB 的内部行为测试（2026-09-28 抽取口径收紧）：
//   1. 文档白名单过滤：只抽 FitProper/FPT 相关文档的 chunk（OJK_DOC_FILTER 默认值）
//   2. R2 强制：LLM 返回的空 pasal 条目不落库
//   3. pasal 前缀归一：LLM 细化引用（"Pasal 3 ayat (1)" vs 切片 ref "Pasal 3"）不误报

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func ojkInternalTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&OJKRun{}, &OJKChecklistItem{}))
	require.NoError(t, db.Exec(`CREATE TABLE knowledges (
		id TEXT PRIMARY KEY, title TEXT, knowledge_base_id TEXT,
		tenant_id INTEGER, deleted_at DATETIME)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE chunks (
		id TEXT PRIMARY KEY, knowledge_base_id TEXT, knowledge_id TEXT,
		tenant_id INTEGER, content TEXT, start_at INTEGER, end_at INTEGER,
		metadata TEXT DEFAULT '{}', deleted_at DATETIME)`).Error)
	return db
}

func TestOJKLoadRegulationText_DocFilter(t *testing.T) {
	db := ojkInternalTestDB(t)

	// 两个文档：FitProper（白名单内）与 TI（白名单外）
	rows := []struct {
		sql string
	}{
		{`INSERT INTO knowledges (id, title, knowledge_base_id, tenant_id) VALUES
			('k-fit', 'POJK_27_2016_FitProper.pdf', 'kb-1', 10011),
			('k-ti',  'POJK_11_2022_TI.pdf',        'kb-1', 10011)`},
		{`INSERT INTO chunks (id, knowledge_base_id, knowledge_id, tenant_id, content, start_at, end_at, deleted_at) VALUES
			('c1', 'kb-1', 'k-fit', 10011, '## Pasal 3' || char(10) || 'Kandidat wajib memiliki integritas yang tinggi tanpa pernah dihukum.', 0, 100, NULL),
			('c2', 'kb-1', 'k-ti',  10011, '## Pasal 99' || char(10) || 'Bank wajib menerapkan tata kelola TI yang baik dan terdokumentasi.', 200, 300, NULL)`},
	}
	for _, r := range rows {
		require.NoError(t, db.Exec(r.sql).Error)
	}

	svc := NewOJKService(db)
	text, err := svc.loadRegulationTextFromDB(context.Background(), 10011, "kb-1")
	require.NoError(t, err)
	assert.Contains(t, text, "integritas", "whitelisted doc must be kept")
	assert.NotContains(t, text, "tata kelola TI", "filtered-out doc must be dropped")

	// 关闭过滤（OJK_DOC_FILTER=-）后 TI 文档回归
	t.Setenv("OJK_DOC_FILTER", "-")
	text2, err := svc.loadRegulationTextFromDB(context.Background(), 10011, "kb-1")
	require.NoError(t, err)
	assert.Contains(t, text2, "tata kelola TI")
}

func TestOJKProcessRun_R2DropAndPrefixMatch(t *testing.T) {
	db := ojkInternalTestDB(t)
	svc := NewOJKService(db)

	run := OJKRun{RunID: "r-proc", TenantID: 10011, KBID: "kb-1", Status: "pending"}
	require.NoError(t, db.Create(&run).Error)

	// 法规文本：FitProper 文档的 Pasal 3（切片 ref = "Pasal 3"）
	require.NoError(t, db.Exec(`INSERT INTO knowledges (id, title, knowledge_base_id, tenant_id) VALUES
		('k-fit', 'POJK_27_2016_FitProper.pdf', 'kb-1', 10011)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO chunks (id, knowledge_base_id, knowledge_id, tenant_id, content, start_at, end_at, deleted_at) VALUES
		('c1', 'kb-1', 'k-fit', 10011,
		'## Pasal 3' || char(10) || 'Kandidat wajib melengkapi formulir aplikasi dan pernyataan kebenaran data yang ditandatangani di atas materai.', 0, 200, NULL)`).Error)

	// mock LLM：三条——前缀匹配的合法条 / 空 pasal（R2 丢弃）/ 不存在的 Pasal 99（误报验证）
	llmItems := []map[string]interface{}{
		{
			"pasal": "Pasal 3 ayat (1)", "pasal_text": "Kandidat wajib melengkapi formulir aplikasi",
			"requirement": "Kandidat wajib melengkapi formulir aplikasi", "area": "Integrity",
			"severity": "critical", "check_method": "document_presence",
			"evidence_type": "Formulir aplikasi", "applicable_roles": []interface{}{"Direktur"},
			"requirement_id": "R1",
		},
		{
			"pasal": "", "pasal_text": "no anchor", "requirement": "item without pasal anchor",
			"area": "Integrity", "severity": "info", "check_method": "document_presence",
			"evidence_type": "x", "applicable_roles": []interface{}{"Direktur"},
			"requirement_id": "R2",
		},
		{
			"pasal": "Pasal 99", "pasal_text": "hallucinated anchor", "requirement": "item with bad pasal",
			"area": "Integrity", "severity": "info", "check_method": "document_presence",
			"evidence_type": "x", "applicable_roles": []interface{}{"Direktur"},
			"requirement_id": "R3",
		},
	}
	raw, _ := json.Marshal(llmItems)
	svc.InvokeLLMFn = func(ctx context.Context, cfg LLMConfig, system, user string) (string, error) {
		return string(raw), nil
	}
	svc.LoadModelConfigFn = func(ctx context.Context, tenantID uint64) (LLMConfig, error) {
		return LLMConfig{}, nil
	}

	svc.processRun("r-proc", 10011, "kb-1", "OJK Test KB")

	var after OJKRun
	require.NoError(t, db.Where("run_id = ?", "r-proc").First(&after).Error)
	assert.Equal(t, "done", after.Status)
	assert.Equal(t, 2, after.TotalItems, "empty-pasal item must be dropped: 3 - 1")
	assert.Equal(t, 1, after.FlaggedItems, "only Pasal 99 item carries pasal_unverified")

	var ids []string
	require.NoError(t, db.Model(&OJKChecklistItem{}).Where("run_id = ?", "r-proc").Order("id").Pluck("id", &ids).Error)
	assert.Equal(t, []string{"c-r-proc-0001", "c-r-proc-0002"}, ids, "persisted numbering must be gapless")

	var flagged OJKChecklistItem
	require.NoError(t, db.Where("run_id = ? AND id = ?", "r-proc", "c-r-proc-0001").First(&flagged).Error)
	assert.Nil(t, flagged.Flag, "prefix-matched pasal must not be flagged (Pasal 3 ayat (1) vs ref Pasal 3)")

	var badPasal OJKChecklistItem
	require.NoError(t, db.Where("run_id = ? AND requirement_id = ?", "r-proc", "R3").First(&badPasal).Error)
	require.NotNil(t, badPasal.Flag)
	assert.Contains(t, *badPasal.Flag, "pasal_unverified")
}

func TestOJKProcessRun_PromptCoversInstitutionalSkip(t *testing.T) {
	// 口径收紧的回归锚：R5 必须显式要求跳过以银行为主体的机构义务条款
	assert.Contains(t, ojkSystemPrompt, "Bank wajib",
		"R5 must name the institutional-obligation skip rule")
	assert.Contains(t, ojkSystemPrompt, "MUST NOT become ChecklistItems")
}

// 抽取 prompt 有两份拷贝：Go 常量（生产执行器）与 run.py（sandbox 参考实现），
// 靠"逐字一致"约定保持同步。此测试把约定变成机器约束——改任一侧必须同步另一侧，
// 否则测试失败。（两份 prompt 漂移 = 两条执行路径的抽取口径悄悄分叉）
func TestOJKSystemPromptMatchesRunPy(t *testing.T) {
	const rel = "../../../examples/skills/fp-rule-skill/scripts/run.py"
	raw, err := os.ReadFile(rel)
	if err != nil {
		t.Skipf("run.py not found (%v) — running outside repo checkout", err)
	}
	s := string(raw)
	const opener = `SYSTEM_PROMPT = """\`
	i := strings.Index(s, opener)
	require.True(t, i >= 0, "SYSTEM_PROMPT block not found in run.py")
	rest := s[i+len(opener):]
	end := strings.Index(rest, `"""`)
	require.True(t, end >= 0, "SYSTEM_PROMPT closing quotes not found")
	runPyPrompt := rest[:end]
	// python 三引号带首尾换行、Go raw string 不带——引号风格差异不算漂移，
	// 只对内容本体做逐字比对
	runPyPrompt = strings.TrimPrefix(runPyPrompt, "\n")
	assert.Equal(t,
		strings.TrimRight(ojkSystemPrompt, "\n"),
		strings.TrimRight(runPyPrompt, "\n"),
		"ojkSystemPrompt 与 run.py SYSTEM_PROMPT 漂移——两处必须逐字同步修改")
}
