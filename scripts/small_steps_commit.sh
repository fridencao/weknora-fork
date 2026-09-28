#!/usr/bin/env bash
# 小步提交工作流助手——把"每个逻辑单元一个提交"固化成机器约束。
#
# 用法:
#   scripts/small_steps_commit.sh --dry-run   # 只预览分组，不提交
#   scripts/small_steps_commit.sh [-p]        # 逐组交互确认提交（-p 结束后推送）
#
# 流程约定见 StarKB 仓库 docs/18-小步提交工作流.md
#
# 原则（与 docs 同步维护）:
#   1. 每完成一个"可验证单元"（构建过/单测过/功能可演示）立即提交一次
#   2. 一次提交只回答一个问题：migration 与依赖它的代码同提交；
#      UI 与其消费的 API 同提交或相邻两提交；i18n 跟随所属页面
#   3. 提交信息格式: <type>(<scope>): <中文主题>，正文可另起列要点
#
# 兼容性: bash 3.2+（不用 mapfile/关联数组）；grep 用 command grep
set -euo pipefail
cd "$(dirname "$0")/.."

DRY=0; PUSH=0
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY=1 ;;
    -p|--push) PUSH=1 ;;
    *) echo "unknown arg: $arg"; exit 1 ;;
  esac
done

# ==== 分组规则：顺序首匹配。三段式 "前缀|路径正则|默认主题" ====
# （按团队习惯增删；正则为 grep -E 语法，作用于仓库相对路径）
RULES=(
  "fix(ojk-db)|^migrations/|数据库迁移"
  "feat(ojk-svc)|^internal/application/service/.*ojk|服务层与领域逻辑"
  "feat(ojk-api)|^internal/handler/.*ojk|接口与路由"
  "feat(ojk-api)|^internal/router/routes_ojk|接口与路由"
  "test(ojk)|^internal/.*test[.]go|测试"
  "feat(ojk-ui)|^frontend/src/views/ojk/|阶段页面与组件"
  "feat(ojk-api)|^frontend/src/api/ojk/|前端 API 层"
  "i18n(ojk)|^frontend/src/i18n/|五语言文案"
  "style(theme)|^frontend/src/assets/|主题与样式覆盖"
  "build|^scripts/build|构建脚本与镜像"
  "chore(skill)|^examples/skills/|skill 参考实现同步"
  "docs|^docs/|文档"
  "chore|.*|其他杂项"
)

# 归零暂存区，保证每组的 add 精确
git reset -q

status_lines=$(git -c core.quotepath=false status --porcelain=v1)
if [ -z "$status_lines" ]; then
  echo "OK: worktree clean, nothing to commit"
  exit 0
fi

G_N=0
ASSIGNED=$'\n'
G_PREFIX=()
G_DEFSUB=()
G_FILES=()

for rule in "${RULES[@]}"; do
  prefix="${rule%%|*}"
  rest="${rule#*|}"
  pat="${rest%%|*}"
  defsub="${rest#*|}"
  files=""
  n=0
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    path="${line:3}"
    path="${path#* -> }"   # 重命名取新路径
    printf '%s' "$path" | command grep -qE "$pat" || continue
    files+="$path"$'\n'
    n=$((n + 1))
  done < <(printf '%s\n' "$status_lines")
  if [ "$n" -gt 0 ]; then
    G_N=$((G_N + 1))
    G_PREFIX+=("$prefix")
    G_DEFSUB+=("$defsub")
    G_FILES+=("$files")
    ASSIGNED+="$files"
    # 已归组的文件从后续规则里排除（按路径精确排除，而非整行）
    remaining=""
    while IFS= read -r line; do
      [ -z "$line" ] && continue
      p="${line:3}"
      printf '%s' "$ASSIGNED" | command grep -qxF "$p" && continue
      remaining+="$line"$'\n'
    done < <(printf '%s\n' "$status_lines")
    status_lines="$remaining"
  fi
done

if [ "$G_N" -eq 0 ]; then
  echo "OK: worktree clean, nothing to commit"
  exit 0
fi

# ---- 预览 ----
echo "== small-step commit groups =="
total=0
i=0
while [ "$i" -lt "$G_N" ]; do
  prefix="${G_PREFIX[$i]}"
  n=$(printf '%s' "${G_FILES[$i]}" | command grep -c . || true)
  total=$((total + n))
  echo "  [$prefix] files=$n default-subject: ${G_DEFSUB[$i]}"
  printf '%s' "${G_FILES[$i]}" | sed 's/^/      /'
  i=$((i + 1))
done
echo "  total files=$total groups=$G_N"

if [ "$DRY" = "1" ]; then
  echo "(--dry-run: nothing committed)"
  exit 0
fi

# ---- 逐组交互提交 ----
committed=0
i=0
while [ "$i" -lt "$G_N" ]; do
  prefix="${G_PREFIX[$i]}"
  defsub="${G_DEFSUB[$i]}"
  echo
  echo "== group [$prefix] =="
  printf '%s' "${G_FILES[$i]}" | sed 's/^/   /'
  printf 'commit subject [Enter = "%s" | s = skip]: ' "$defsub"
  read -r msg
  if [ "${msg:-}" = "s" ] || [ "${msg:-}" = "S" ]; then
    echo "  skipped"
    i=$((i + 1))
    continue
  fi
  [ -z "${msg:-}" ] && msg="$defsub"

  printf '%s' "${G_FILES[$i]}" | while IFS= read -r f; do
    [ -n "$f" ] && git add -- "$f"
  done

  if git diff --cached --quiet; then
    echo "  (nothing staged, skipped)"
    i=$((i + 1))
    continue
  fi
  git commit -m "$prefix: $msg"
  committed=$((committed + 1))
  i=$((i + 1))
done

echo
echo "DONE: $committed small-step commits on $(git branch --show-current)"
if [ "$PUSH" = "1" ] && [ "$committed" -gt 0 ]; then
  branch="$(git branch --show-current)"
  printf 'push to fork/%s? [y/N]: ' "$branch"
  read -r yn
  case "${yn:-}" in
    [yY]*) git push fork "$branch" ;;
  esac
fi
