package service

// 配置治理批一的测试辅助：迁移后，开关不再直读环境变量，而是经
// SystemSettingService 的三层解析（DB > ENV > 默认）。既有单测用
// t.Setenv 驱动开关，需要一个「DB 层为空」的设置服务实例，解析结果
// 就退化成 ENV > 内置默认——与迁移前语义等价。

import (
	"context"
	"sync/atomic"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// emptySettingRepo 永远报告"无该行"，让解析链直接落到 ENV/default。
type emptySettingRepo struct{}

func (emptySettingRepo) Get(context.Context, string) (*types.SystemSetting, error) {
	return nil, nil
}

func (emptySettingRepo) List(context.Context) ([]*types.SystemSetting, error) {
	return nil, nil
}

func (emptySettingRepo) Upsert(context.Context, *types.SystemSetting) error { return nil }

func (emptySettingRepo) Delete(context.Context, string) (bool, error) { return false, nil }

// newEnvOnlySettings 返回只走 ENV/default 的设置服务。
//
// 直接构造而不走 NewSystemSettingService：后者会起 preload goroutine 并在
// 无 DB 时刷错误日志。这里把 loaded 置 true 且 cache 为空，resolveRaw 视作
// 权威 miss，不触发任何 DB 访问。
func newEnvOnlySettings() interfaces.SystemSettingService {
	s := &systemSettingService{
		repo:       emptySettingRepo{},
		instanceID: "test",
		cache:      make(map[string]*types.SystemSetting),
	}
	s.loaded.Store(true)
	return s
}

// 保证 atomic 被引用（systemSettingService.loaded 已是 atomic.Bool，
// 此处仅为显式声明测试对该行为的依赖，避免未来重构静默改变语义）。
var _ = atomic.Bool{}
