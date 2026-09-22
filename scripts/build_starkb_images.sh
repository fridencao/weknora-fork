#!/usr/bin/env bash
# StarKB 镜像快速构建：国内源 + 跳过可选组件 + 仅构建 app/frontend 两个改动面。
#
# 与 `make docker-build-app` 的区别：
#   1. 基础镜像先经 daocloud 镜像源拉取（比直连 Docker Hub 快 ~60 倍，见 M1 记录）
#   2. 默认 WITH_ANYDOC=0 / WITH_BROWSERSKILL=0：跳过两套 Rust 工具链（office 解析
#      走 docreader/MinerU，浏览器技能暂不需要），各省数分钟与 ~1GB 中间层
#   3. go/npm/pip/apt/rustup 全部走国内镜像
#   4. 生成 docker-compose.override.yml，让 compose 直接用本地镜像名
#
# 用法：
#   bash scripts/build_starkb_images.sh                 # app + frontend
#   WITH_ANYDOC=1 WITH_BROWSERSKILL=1 bash scripts/build_starkb_images.sh
set -euo pipefail
cd "$(dirname "$0")/.."

MIRROR="${MIRROR:-docker.m.daocloud.io}"

prefetch() { # prefetch <镜像名:tag> [镜像名@digest]
  local name="$1" digest="${2:-}"
  if [ -n "$digest" ]; then
    docker image inspect "$name@$digest" >/dev/null 2>&1 && return 0
    docker pull "$MIRROR/$name@$digest" 2>/dev/null || \
      { echo "WARN: $name 拉取失败，构建时将直连上游"; return 0; }
  else
    docker image inspect "$name" >/dev/null 2>&1 && return 0
    docker pull "$MIRROR/$name" 2>/dev/null || \
      { echo "WARN: $name 拉取失败，构建时将直连上游"; return 0; }
  fi
}

echo "== 1/3 预取基础镜像（daocloud）=="
prefetch golang:1.26-bookworm
prefetch debian:12.12-slim
# digest 固定的基础镜像按 digest 拉，BuildKit 直接命中本地
prefetch node:24-bookworm-slim ba849c60be29959425b8734d57b8b4b7d56f98edd9504c9af091d5281095a71e
prefetch nginx:1.30.3-alpine 0d3b80406a13a767339fbe2f41406d6c7da727ab89cf8fae399e81f780f814d1

echo "== 2/3 构建 app（Go 后端）=="
eval "$(./scripts/get_version.sh env)"
docker build \
  --build-arg VERSION_ARG="$VERSION" \
  --build-arg COMMIT_ID_ARG="$COMMIT_ID" \
  --build-arg BUILD_TIME_ARG="$BUILD_TIME" \
  --build-arg GO_VERSION_ARG="$GO_VERSION" \
  --build-arg GOPROXY_ARG="https://goproxy.cn,direct" \
  --build-arg APK_MIRROR_ARG="${APK_MIRROR_ARG:-mirrors.tencent.com}" \
  --build-arg PIP_INDEX_ARG="${PIP_INDEX_ARG:-https://pypi.tuna.tsinghua.edu.cn/simple}" \
  --build-arg WITH_ANYDOC="${WITH_ANYDOC:-0}" \
  --build-arg WITH_BROWSERSKILL="${WITH_BROWSERSKILL:-0}" \
  -f docker/Dockerfile.app -t starkb-app:local .

echo "== 3/3 构建 frontend（nginx + dist）=="
# C1（docs/09 WS4.1）：frontend 默认走预构建模式——dist 在宿主机构建后仅 COPY
# 进镜像。容器内 npm run build 在小内存宿主（≤6GB VM）反复 OOM-137（实测 3 次）。
# USE_PREBUILT=0 退回容器内构建（Dockerfile.frontend，机器内存充裕时可用）。
if [ "${USE_PREBUILT:-1}" = "1" ]; then
  echo "-- 预构建模式：宿主机 npm ci + build（USE_PREBUILT=0 可退回容器内构建）--"
  ( cd frontend \
    && npm ci --registry="${NPM_REGISTRY:-https://registry.npmmirror.com}" \
    && VITE_IS_DOCKER=true VITE_FRONTEND_COMMIT="$COMMIT_ID" npm run build )
  docker build \
    -f docker/Dockerfile.frontend-prebuilt \
    -t starkb-ui:local .
else
  docker build \
    --build-arg VITE_FRONTEND_COMMIT="$COMMIT_ID" \
    --build-arg NPM_REGISTRY="${NPM_REGISTRY:-https://registry.npmmirror.com}" \
    -t starkb-ui:local frontend/
fi

# 让 docker compose 直接使用本地镜像（compose 自动合并 override）
cat > docker-compose.override.yml <<'EOF'
# 由 scripts/build_starkb_images.sh 生成：本地自建镜像替换官方镜像
services:
  app:
    image: starkb-app:local
  frontend:
    image: starkb-ui:local
EOF

echo "完成：starkb-app:local / starkb-ui:local"
echo "已写入 docker-compose.override.yml，直接 docker compose up -d app frontend 即可切换。"
