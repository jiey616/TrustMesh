# frontend-v2 视觉规则（双主题）

> 配套文档：`docs/frontend-v2-visual-hybrid-plan.md`（改造方案）
> 令牌源码：`frontend-v2/src/theme/tokens.ts`
> 生效日期：2026-09-04

## 1. 两套主题

| 主题 | 取值 | 气质 |
| --- | --- | --- |
| `dark` | `darkTokens` | TrustMesh console：深色画布 + 霓虹玻璃 |
| `quiet` | `quietTokens` | Quiet Signal：近白 `#FAFAFA`、结构直角、无装饰 |

主题由 `ThemeProvider` 注入 CSS 变量 + `data-theme` 属性，持久化在 `localStorage['trustmesh-theme']`。
切换入口：侧边栏底部。

**两套主题共用同一套令牌名**，组件只写 `var(--x)`，不写具体色值。

## 2. 铁律

1. **禁止在内联样式里写死色值。** 一律用 `var(--x)`。
   需要新色值 → 先加到 `tokens.ts` 的两套里，再引用。
2. **禁止用效果类属性写死观感。** `backdropFilter` / `boxShadow` / 背景光晕统一走
   `--glass-blur` / `--shadow-card` / `--shadow-float` / `--canvas-glow`，
   近白主题下这些值为 `none` 或浅投影，自动降级。
3. **禁止装饰性气氛场。** 彩色光晕、浮动光球、渐变流光只在深色主题存在
   （`FloatingOrbs` 在近白主题下直接返回 `null`）。
4. **紫色只作信号。** `--signal` 用于主 CTA、当前选中、焦点环、关键状态。
   禁止大面积背景、长篇正文、纯装饰。一个视图内只有一个主要紫色焦点。
5. **禁止硬编码白字。** `color: '#fff'` 在近白主题下隐形，必须写 `var(--text-primary)`。
   （`deploy/.tmp/fix_white.py` 可批量检查/修复。）

## 3. 圆角：只有四档

| 令牌 | 值 | 用途 |
| --- | --- | --- |
| `--radius-structure` | `0` | 页面 / 布局外壳 / 大容器。**与横线相接处必须直角** |
| `--radius-control` | `4px` | 次级内容区（面板、Section）、控件（按钮、输入框、小卡） |
| `--radius-avatar` | `50%` | 头像 |
| `--radius-pill` | `999px` | 胶囊标签 |

**禁止层层圆角**：外层容器、内层表面、内部控件不要同时圆角化。

## 4. 表面：三档 + 抬升

| 令牌 | 深色 | 近白 | 用途 |
| --- | --- | --- | --- |
| `--surface` | `rgba(255,255,255,0.04)` | `#FFFFFF` | 确有独立边界的表面 |
| `--surface-sunken` | `rgba(255,255,255,0.02)` | `#F4F4F5` | 次级、内收、不可编辑区（微亮） |
| `--surface-inset` | `rgba(0,0,0,0.25)` | `#F4F4F5` | 凹陷区：代码块、内联输入、内嵌容器（压暗） |
| `--surface-raised` | `rgba(255,255,255,0.06)` | `#FFFFFF` | hover / 选中态 |

`--canvas` / `--canvas-elevated` 是画布与抬升层（侧栏、Modal、下拉），不是表面。

## 5. 线条：两档

| 令牌 | 深色 | 近白 |
| --- | --- | --- |
| `--line` | `rgba(255,255,255,0.07)` | `rgba(10,10,10,0.10)` |
| `--line-strong` | `rgba(255,255,255,0.12)` | `rgba(10,10,10,0.18)` |

hover / 选中优先用**边框 + 信号色**表达，而不是继续加深填充。

## 6. 文字：四档

`--text-primary` / `--text-secondary` / `--text-tertiary` / `--text-quaternary`。
另有 `--text-inverse` 用于反色场景（深色块上的白字）。

## 7. 效果开关

| 令牌 | dark | quiet |
| --- | --- | --- |
| `--glass-blur` | `blur(24px) saturate(150%)` | `none` |
| `--canvas-glow` | 两道 radial-gradient | `none` |
| `--shadow-card` / `--shadow-float` | 深色投影 | 浅投影 |
| `--gradient-cta` | 紫→亮紫渐变 | 纯 `--signal` |

毛玻璃只在**真浮层**保留：Drawer、Modal、Tooltip、下拉菜单、浮层卡片。
页面主内容区原则上不用。

## 8. 新增代码检查清单

- [ ] 没有写死 hex / `rgba(...)` 色值
- [ ] 圆角只用四档令牌
- [ ] 没有新增 `backdropFilter`（除非是真浮层）
- [ ] 没有新增装饰性动画 / 光晕
- [ ] 两套主题下都目视确认过

## 9. 批量治理脚本

位于 `deploy/.tmp/`，可重复运行，均默认 dry-run：

| 脚本 | 作用 |
| --- | --- |
| `tokenize_styles.py` | 内联样式色值 → 令牌（按 CSS 属性语义分表映射，渐变跳过报人工） |
| `normalize_radius.py` | 圆角收敛到四档令牌 |
| `fix_dark_bg.py` | 残留深色背景 → `--surface-inset` / `--canvas-elevated` |
| `fix_white.py` | 硬编码白字/白底 → `--text-primary` / `--canvas-elevated` |

⚠️ 脚本会跳过 `src/components/office`（AI Office 3D 独立视觉体系，不适用本规则）。
