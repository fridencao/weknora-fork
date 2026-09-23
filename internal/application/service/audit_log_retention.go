package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// AuditLogRetentionRunner sweeps audit_logs once a day, deleting rows
// older than `retentionDays`. It is a small, self-contained
// background goroutine — no robfig/cron, no asynq — because retention
// has no wall-clock alignment requirement: we just need "approximately
// daily, eventually". A bare time.Ticker keeps the dependency surface
// minimal.
//
// retentionDays <= 0 makes Start a no-op; this is the configured way
// to disable retention entirely. Validation happens at config-load
// time so by the time we're here a non-positive value is intentional.
type AuditLogRetentionRunner struct {
	svc      interfaces.AuditLogService
	settings interfaces.SystemSettingService
	// retentionDays is the config.yaml tier, kept as the resolver's def;
	// the effective value is resolved per sweep (see runOnce).
	retentionDays int
	interval      time.Duration

	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
	doneCh    chan struct{}
	// started is set inside startOnce.Do BEFORE doneCh is wired to a
	// goroutine, so Stop() can tell "Start was never called" apart from
	// "Start is running" without blocking on doneCh. Without this, a
	// runner that was constructed but never Start()'d (early container
	// init failure, test setup that skips Start) would deadlock Stop()
	// on a doneCh nobody ever closes.
	started atomic.Bool

	// retentionSource is best-effort provenance for the last resolved
	// value, used in startup logs only.
	retentionSource string
}

// auditLogPurgeInterval is the gap between sweeps. 24h is enough for
// a per-day retention horizon — the cutoff moves by 24h between runs
// so each sweep deletes one day's worth of rolled-off rows. Shortening
// this would just cause empty sweeps; lengthening it would pile up
// stale rows for a day.
const auditLogPurgeInterval = 24 * time.Hour

// auditLogPurgeStartupDelay holds the very first sweep until shortly
// after boot so we don't compete with migration-up traffic or other
// startup work. Long enough that the first DELETE doesn't fight the
// initial request flood; short enough that operators see the sweep
// fire on the same day they restart.
const auditLogPurgeStartupDelay = 10 * time.Minute

// NewAuditLogRetentionRunner constructs the runner with production
// defaults. retention_days resolves per sweep from system_settings
// (audit.retention_days), falling back through the env var to the
// config.yaml value baked in at startup — so a UI edit lands on the next
// sweep (≤24h) without a restart. Passing the full *config.Config keeps
// the dig wiring trivial and supplies the YAML tier as the resolver's
// def. The constructor only stores wiring; nothing fires until Start.
func NewAuditLogRetentionRunner(
	cfg *config.Config, svc interfaces.AuditLogService,
	settings interfaces.SystemSettingService,
) *AuditLogRetentionRunner {
	retentionDays := 0
	if cfg != nil && cfg.Audit != nil {
		retentionDays = cfg.Audit.RetentionDays
	}
	return &AuditLogRetentionRunner{
		svc:           svc,
		settings:      settings,
		retentionDays: retentionDays,
		interval:      auditLogPurgeInterval,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
}

// Start spins up the background goroutine. Calling it more than once
// is a no-op (sync.Once), so container wiring that mistakenly invokes
// us twice doesn't double-purge. retentionDays <= 0 means the runner
// stays dormant — Stop will still complete cleanly.
func (r *AuditLogRetentionRunner) Start(ctx context.Context) {
	if r == nil || r.svc == nil {
		return
	}
	r.startOnce.Do(func() {
		r.started.Store(true)
		// 刻意不在启动期因 retention<=0 休眠：生效值每次 sweep 动态解析，
		// 运维中途把 0 改成 90（或反之）都应在下个 sweep 生效，而不是
		// 被启动时那一次判断锁死。
		days := r.effectiveRetentionDays(ctx)
		if days <= 0 {
			logger.Infof(ctx,
				"[audit-retention] sweep loop started, purge currently disabled "+
					"(retention_days=%d from %s)", days, r.retentionSource)
		} else {
			logger.Infof(ctx,
				"[audit-retention] starting daily sweep: retention_days=%d (from %s) interval=%s",
				days, r.retentionSource, r.interval)
		}
		go r.loop()
	})

}

// retentionSource records where the last resolved value came from, for
// startup logging. Not authoritative — resolved fresh each sweep.
var _ = ""

// effectiveRetentionDays resolves the effective retention window for the
// current sweep: DB system_settings > ENV > config.yaml > 0 (disabled).
func (r *AuditLogRetentionRunner) effectiveRetentionDays(ctx context.Context) int {
	def := r.retentionDays
	if r.settings == nil {
		r.retentionSource = "config.yaml"
		return def
	}
	v := r.settings.GetInt(ctx,
		types.SettingKeyAuditRetentionDays, types.SettingEnvAuditRetentionDays, int64(def))
	if v == int64(def) {
		// GetInt 无法区分「DB 恰好等于 def」与「未设置回落 def」；
		// 仅用于日志措辞，不影响行为。
		r.retentionSource = "settings/env/config(等值)"
	} else {
		r.retentionSource = "settings/env"
	}
	return int(v)
}

// Stop signals the loop to exit and blocks until it returns. Idempotent.
// If Start was never called, Stop returns immediately (no doneCh to
// wait on — see the `started` flag in the struct comment).
func (r *AuditLogRetentionRunner) Stop() {
	if r == nil {
		return
	}
	if !r.started.Load() {
		return
	}
	r.stopOnce.Do(func() {
		close(r.stopCh)
	})
	<-r.doneCh
}

// loop runs the actual sweep cadence. Uses a fresh context.Background
// per iteration because the request-scoped ctx from Start would be
// cancelled the moment Start's caller returned — and that caller is
// container init.
func (r *AuditLogRetentionRunner) loop() {
	defer close(r.doneCh)

	startupTimer := time.NewTimer(auditLogPurgeStartupDelay)
	defer startupTimer.Stop()
	select {
	case <-startupTimer.C:
	case <-r.stopCh:
		return
	}

	r.runOnce()

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.runOnce()
		case <-r.stopCh:
			return
		}
	}
}

// runOnce performs a single sweep. The DB call is given a generous
// timeout (30 s) so a stuck connection doesn't hold the goroutine
// hostage forever — if the sweep doesn't finish in 30 s we'll log
// and try again 24 h later. Errors are logged at WARN, not ERROR,
// because the table just keeps growing one more day; nothing breaks.
func (r *AuditLogRetentionRunner) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	days := r.effectiveRetentionDays(ctx)
	if days <= 0 {
		logger.Debugf(ctx,
			"[audit-retention] purge disabled (retention_days=%d), sweep skipped", days)
		return
	}

	deleted, err := r.svc.Purge(ctx, days)
	if err != nil {
		logger.Warnf(ctx,
			"[audit-retention] sweep failed: retention_days=%d err=%v",
			days, err)
		return
	}
	if deleted > 0 {
		logger.Infof(ctx,
			"[audit-retention] sweep complete: deleted=%d retention_days=%d",
			deleted, r.retentionDays)
	} else {
		logger.Debugf(ctx,
			"[audit-retention] sweep complete: deleted=0 retention_days=%d",
			days)
	}
}
