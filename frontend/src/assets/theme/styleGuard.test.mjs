import assert from 'node:assert/strict'
import { test } from 'node:test'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

/**
 * 样式守卫（棘轮）：统计 views/ components/ assets/ 里绕过设计令牌或和 TDesign 打架的写法，
 * 只允许计数下降。新增任何一处都会让测试失败；清理后请把 baseline 调低。
 *
 * 令牌定义在 assets/theme/theme.css（--app-radius-* / --app-text-* / --app-space-* /
 * --app-motion-* / --z-*），TDesign 全局覆盖放在 assets/theme/tdesign-overrides.less。
 * 用 .mjs 是因为 `npm test`（tsx --test）在 Node 20 下只自动发现 .mjs 测试文件。
 */
const SRC_ROOT = join(dirname(fileURLToPath(import.meta.url)), '..', '..')
const SCAN_DIRS = ['views', 'components', 'assets']
const EXTS = new Set(['.vue', '.less', '.css'])
const EXEMPT_FILES = new Set(['assets/theme/theme.css'])

const RULES = [
  {
    name: 'brand-rgba',
    why: '品牌色透明叠加请用 color-mix(in srgb, var(--td-brand-color) N%, transparent)，否则深色模式不跟随',
    pattern: /rgba\(\s*7\s*,\s*192\s*,\s*95\s*,/g,
    baseline: 0,
  },
  {
    name: 'legacy-blue-rgba',
    why: '旧版 TDesign 蓝 rgba(0,82,217) 已不是品牌色',
    pattern: /rgba\(\s*0\s*,\s*82\s*,\s*217\s*,/g,
    baseline: 18,
  },
  {
    name: 'td-token-fallback',
    why: 'theme.css 保证 --td-* 令牌存在，fallback 永远不会生效且容易写错颜色',
    pattern: /var\(\s*--td-(?!purple-5|cyan-6|font-family-code)[A-Za-z0-9-]+\s*,/g,
    baseline: 0,
  },
  {
    name: 'radius-literal',
    why: '圆角请用 var(--app-radius-xs|sm|md|lg|xl|pill)（4/6/8/10/12/999px）',
    pattern: /border(?:-[a-z]+)*-radius\s*:\s*\d+(?:\.\d+)?px/g,
    baseline: 162,
  },
  {
    name: 'font-size-literal',
    why: '字号请用 var(--app-text-2xs … 4xl)（10~24px）',
    pattern: /font-size\s*:\s*\d+(?:\.\d+)?px/g,
    baseline: 28,
  },
  {
    name: 'motion-literal',
    why: '过渡时长请用 var(--app-motion-instant|fast|base|slow)（120/150/200/300ms）',
    pattern: /transition[^;{]*?(?<![\d.])(?:0?\.\d+s|\d+ms)/g,
    baseline: 65,
  },
  {
    name: 'transition-all',
    why: 'transition: all 会让无关属性也参与动画（含 layout 属性），请列出具体属性',
    pattern: /transition\s*:\s*all\b/g,
    baseline: 78,
  },
  {
    name: 'important',
    why: '!important 通常意味着在和 TDesign 或自己的样式打架；全局意图的覆盖放 tdesign-overrides.less',
    pattern: /!important/g,
    baseline: 510,
  },
  {
    name: 'z-index-important',
    why: 'z-index 不应靠 !important 取胜，改用 t-popup attach="body" 或 --z-* 层级令牌',
    pattern: /z-index\s*:\s*-?\d+\s*!important/g,
    baseline: 26,
  },
  {
    name: 'raster-icon',
    why: 'more.png / circle.png 位图图标请换成 t-icon',
    pattern: /(more|circle)\.png/g,
    baseline: 10,
  },
]

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name)
    if (name === 'node_modules') continue
    if (statSync(full).isDirectory()) {
      yield* walk(full)
    } else if (EXTS.has(full.slice(full.lastIndexOf('.'))) && !/\.test\./.test(name)) {
      yield full
    }
  }
}

function countMatches(rule) {
  const byFile = new Map()
  let total = 0
  for (const dir of SCAN_DIRS) {
    for (const file of walk(join(SRC_ROOT, dir))) {
      const rel = relative(SRC_ROOT, file)
      if (EXEMPT_FILES.has(rel)) continue
      const text = readFileSync(file, 'utf8')
      const n = (text.match(rule.pattern) ?? []).length
      if (n > 0) {
        byFile.set(rel, n)
        total += n
      }
    }
  }
  return { total, byFile }
}

for (const rule of RULES) {
  test(`style guard: ${rule.name} does not exceed baseline (${rule.baseline})`, () => {
    const { total, byFile } = countMatches(rule)
    const top = [...byFile.entries()]
      .sort((a, b) => b[1] - a[1])
      .slice(0, 8)
      .map(([f, n]) => `  ${n}\t${f}`)
      .join('\n')
    assert.ok(
      total <= rule.baseline,
      `${rule.name}: ${total} occurrences > baseline ${rule.baseline}.\n${rule.why}\n${top}`,
    )
  })
}

// ── 字号阶梯锚点（2026-09-22 zoom→token 重映射）────────────────────────
// theme.css 末尾的三档 token 覆盖块是「字体大小」设置的实现载体，
// 这里锁住四个角色锚点（辅助 sm / 正文 base / 标题小 xl / 标题中 3xl）
// 在三档下的渲染值，防止后续改版把阶梯悄悄改掉。
const THEME_CSS = readFileSync(join(SRC_ROOT, 'assets', 'theme', 'theme.css'), 'utf8')

const LADDER_ANCHORS = {
  normal: { sm: 12, base: 13, xl: 14, '3xl': 15 },
  small: { sm: 11, base: 12, xl: 13, '3xl': 14 },
  large: { sm: 13, base: 14, xl: 15, '3xl': 16 },
}

function ladderBlock(mode) {
  const selector =
    mode === 'normal' ? ':root:root:root' : `:root:root[data-font-size='${mode}']`
  const start = THEME_CSS.indexOf(selector + ' {')
  assert.ok(start >= 0, `theme.css 缺少 ${selector} 覆盖块`)
  const end = THEME_CSS.indexOf('}', start)
  return THEME_CSS.slice(start, end)
}

for (const [mode, anchors] of Object.entries(LADDER_ANCHORS)) {
  test(`font-size ladder anchors (${mode})`, () => {
    const block = ladderBlock(mode)
    for (const [token, px] of Object.entries(anchors)) {
      const re = new RegExp(`--app-text-${token}:\\s*${px}px`)
      assert.ok(re.test(block), `theme.css ${mode} 档 --app-text-${token} 应为 ${px}px`)
    }
  })
}
