// Bootstrap-time hooks that run after the DI container is built but
// before the HTTP server starts listening. These are deliberately
// best-effort: any failure here only warns and does NOT abort startup.
// The reasoning is that an operator running with a misconfigured env
// var should still be able to bring the server up (and fix the issue
// from the running instance) rather than have a typo brick the deploy.
package main

import (
	"context"
	"os"
	"strings"

	"go.uber.org/dig"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// bootstrapEnvVar is the env var that names the email of the user who
// may be promoted to system administrator when the deployment has no
// existing system administrators.
//
// Why an env var (vs a CLI subcommand)?
//   - Zero-friction in docker-compose / k8s deploys: set it once in the
//     manifest and the very first user account that signs up with that
//     email is auto-promoted, with no extra ops step.
//   - Idempotent: if the user is already a system admin, bootstrapping is
//     a no-op.
//   - Safe to leave set: once at least one system admin exists, the env
//     var stops granting privileges. That prevents a UI revoke from being
//     silently undone on the next restart.
const bootstrapEnvVar = "WEKNORA_BOOTSTRAP_SYSTEM_ADMIN_EMAIL"

// runStartupBootstrap consults the env and applies any one-shot
// bootstrap actions. Currently it only handles system-admin promotion;
// future bootstrap steps (default model seeding, etc.) can be added
// here as additional dig.Invoke calls.
func runStartupBootstrap(c *dig.Container) {
	ctx := context.Background()

	// Legacy hash repair for migration 000065 placeholder rows. Invoked each
	// startup but short-circuits with a cheap EXISTS once every row is
	// backfilled (no api_key decryption on the steady-state path).
	if err := c.Invoke(func(apiKeySvc interfaces.TenantAPIKeyService) {
		if n, err := apiKeySvc.BackfillMissingKeyHashes(ctx); err != nil {
			logger.Warnf(ctx, "[bootstrap] tenant api key hash backfill failed: %v", err)
		} else if n > 0 {
			logger.Infof(ctx, "[bootstrap] backfilled %d legacy tenant api key hash(es)", n)
		}
	}); err != nil {
		logger.Warnf(ctx, "[bootstrap] failed to resolve TenantAPIKeyService: %v", err)
	}

	// Tenant security gates: system_settings > env > config.yaml. Runs
	// unconditionally and BEFORE the email early-return below, so a
	// deployment that never sets the bootstrap admin var still gets the
	// DB-backed values applied.
	if err := c.Invoke(func(cfg *config.Config, svc interfaces.SystemSettingService) {
		applyTenantSettingOverrides(ctx, cfg, svc)
	}); err != nil {
		logger.Warnf(ctx, "[bootstrap] failed to apply tenant setting overrides: %v", err)
	}

	email := strings.TrimSpace(os.Getenv(bootstrapEnvVar))
	if email == "" {
		return
	}
	// dig.Invoke resolves UserService from the container; if user
	// service registration is broken we want to know loudly, but still
	// not abort startup — bootstrap is best-effort.
	if err := c.Invoke(func(userSvc interfaces.UserService) {
		bootstrapSystemAdmin(ctx, userSvc, email)
	}); err != nil {
		logger.Warnf(ctx, "[bootstrap] failed to resolve UserService: %v", err)
	}
}

// applyTenantSettingOverrides pushes the two tenant security gates from
// system_settings onto the live *config.Config singleton.
//
// Why this runs here rather than in systemSettingService:
//
//	The consumers of these two flags are the RBAC middleware and the
//	router guards, and they read plain struct fields off the shared
//	*config.Config (rbacGuards{cfg} holds the dig singleton pointer).
//	There is no call site that could resolve the setting itself, and
//	LoadConfig runs long before the DB is reachable.
//
// Why not in systemSettingService.preload (which already pushes several
// settings into package-level overrides):
//
//	preload runs in its own goroutine and can land *after* the listener
//	binds, so writing these fields there would race with the request
//	path's reads. These are security gates, so "bool reads are atomic in
//	practice" is not good enough. runStartupBootstrap is called after
//	BuildContainer (which migrates the DB) but before the HTTP listener
//	binds, so at this point there are no concurrent readers.
//
// The `def` passed to GetBool is the value already resolved from
// config.yaml + env, which makes the effective chain:
//
//	system_settings  >  env  >  config.yaml / built-in default
//
// so deployments that configure these via config.yaml keep working, and
// the env override documented in .env.example still wins over the file.
//
// Because the write happens only here, changing either key in the UI
// takes effect on the next restart — matching RequiresRestart: true on
// the registry entries (systemSettingService.dispatchSideEffects logs a
// reminder instead of pushing).
func applyTenantSettingOverrides(
	ctx context.Context,
	cfg *config.Config,
	svc interfaces.SystemSettingService,
) {
	if cfg == nil || cfg.Tenant == nil {
		return
	}

	// IsRBACEnforced() folds "nil pointer" into the built-in default
	// (true), so an unset config.yaml field behaves as before.
	rbac := svc.GetBool(
		ctx,
		types.SettingKeyTenantEnableRBAC,
		types.SettingEnvTenantEnableRBAC,
		cfg.Tenant.IsRBACEnforced(),
	)
	cfg.Tenant.EnableRBAC = &rbac

	cfg.Tenant.EnableCrossTenantAccess = svc.GetBool(
		ctx,
		types.SettingKeyTenantEnableCrossTenantAccess,
		types.SettingEnvTenantEnableCrossTenantAccess,
		cfg.Tenant.EnableCrossTenantAccess,
	)

	logger.Infof(ctx,
		"[bootstrap] tenant gates: enable_rbac=%t enable_cross_tenant_access=%t "+
			"(source: system_settings > env > config.yaml)", rbac, cfg.Tenant.EnableCrossTenantAccess)
}

// bootstrapSystemAdmin promotes the user identified by `email` to system
// administrator only when the deployment currently has no system admins.
// The function is idempotent and non-fatal — it warns and returns on
// every error path.
//
// The bootstrap intentionally does NOT create a user when the email is
// not yet registered: account creation is a workflow with side effects
// (password hashing, tenant assignment, audit) that we don't want to
// short-circuit. Operators should sign up normally first, then set the
// env var on the next restart.
func bootstrapSystemAdmin(ctx context.Context, userSvc interfaces.UserService, email string) {
	user, err := userSvc.GetUserByEmail(ctx, email)
	if err != nil {
		// "not found" surfaces as an error in this codebase; treat it
		// gently — operators commonly set the var before the user has
		// signed up. The next restart after registration will succeed.
		logger.Warnf(ctx,
			"[bootstrap] %s=%s: user lookup failed (have they signed up yet?): %v",
			bootstrapEnvVar, email, err)
		return
	}
	if user == nil {
		logger.Warnf(ctx,
			"[bootstrap] %s=%s: no matching user (will retry on next restart)",
			bootstrapEnvVar, email)
		return
	}
	if user.IsSystemAdmin {
		logger.Infof(ctx,
			"[bootstrap] %s=%s: user %s is already a system admin (no-op)",
			bootstrapEnvVar, email, user.ID)
		return
	}
	_, total, err := userSvc.ListSystemAdmins(ctx, 0, 1)
	if err != nil {
		logger.Warnf(ctx,
			"[bootstrap] %s=%s: cannot verify existing system admins, skipping promotion: %v",
			bootstrapEnvVar, email, err)
		return
	}
	if total > 0 {
		logger.Infof(ctx,
			"[bootstrap] %s=%s: %d system admin(s) already exist; not promoting user %s",
			bootstrapEnvVar, email, total, user.ID)
		return
	}
	user.IsSystemAdmin = true
	if err := userSvc.UpdateUser(ctx, user); err != nil {
		logger.Warnf(ctx,
			"[bootstrap] %s=%s: failed to promote user %s: %v",
			bootstrapEnvVar, email, user.ID, err)
		return
	}
	logger.Infof(ctx,
		"[bootstrap] promoted user %s (%s) to system admin via %s",
		user.ID, email, bootstrapEnvVar)
}
