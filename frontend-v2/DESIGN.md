---
version: alpha
name: TrustMesh-design-analysis
description: "A near-black multi-agent command-console canvas built around #0a0a12 (deep charcoal with a faint blue-violet tint), light gray text (#f4f4f8), and the TrustMesh signature violet (#6d5ff5) as the single chromatic accent. The system reads as a dense technical operations console for a multi-agent protocol platform: agents, task flows, meetings, and node status rendered as charcoal panels with hairline borders. Display type sits at weight 500-600 with measured negative tracking. Task lifecycle states borrow the Cursor timeline palette (Thinking / Reading / Editing / Done) mapped to planning / in_progress / awaiting_review / done, with a soft violet reserved for the primary CTA, focus rings, brand mark, and agent 'online' status."
colors:
  primary: "#6d5ff5"
  on-primary: "#ffffff"
  primary-hover: "#8b7ff8"
  primary-active: "#5e52d8"
  primary-soft: "rgba(109,95,245,0.14)"
  primary-border: "rgba(109,95,245,0.35)"
  ink: "#f4f4f8"
  ink-muted: "#c8ccd8"
  ink-subtle: "#8b8f9e"
  ink-tertiary: "#5f6372"
  canvas: "#0a0a12"
  surface-1: "#12121d"
  surface-2: "#171722"
  surface-3: "#1c1c28"
  surface-4: "#232331"
  hairline: "#232330"
  hairline-strong: "#34344a"
  hairline-tertiary: "#45455e"
  semantic-success: "#27a644"
  semantic-warning: "#f59e0b"
  semantic-error: "#ef4444"
  semantic-info: "#3b82f6"
  status-planning: "#b795dd"
  status-in-progress: "#6f8ff0"
  status-awaiting-review: "#e0a35e"
  status-done: "#6dc67f"
  status-failed: "#e25563"
  status-canceled: "#6b6f7d"
  node-online: "#4ade80"
  node-busy: "#60a5fa"
  node-offline: "#71717a"
  overlay: "#000000"

typography:
  display-lg:
    fontFamily: Inter
    fontSize: 44px
    fontWeight: 600
    lineHeight: 1.10
    letterSpacing: -1.4px
  display-md:
    fontFamily: Inter
    fontSize: 32px
    fontWeight: 600
    lineHeight: 1.15
    letterSpacing: -0.9px
  headline:
    fontFamily: Inter
    fontSize: 24px
    fontWeight: 600
    lineHeight: 1.25
    letterSpacing: -0.5px
  card-title:
    fontFamily: Inter
    fontSize: 18px
    fontWeight: 500
    lineHeight: 1.30
    letterSpacing: -0.3px
  body:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: 400
    lineHeight: 1.50
    letterSpacing: 0
  body-sm:
    fontFamily: Inter
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.50
    letterSpacing: 0
  caption:
    fontFamily: Inter
    fontSize: 12px
    fontWeight: 400
    lineHeight: 1.40
    letterSpacing: 0
  button:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: 500
    lineHeight: 1.20
    letterSpacing: 0
  mono:
    fontFamily: JetBrains Mono
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.50
    letterSpacing: 0

rounded:
  xs: 4px
  sm: 6px
  md: 8px
  lg: 12px
  xl: 16px
  pill: 9999px

spacing:
  xxs: 4px
  xs: 8px
  sm: 12px
  md: 16px
  lg: 24px
  xl: 32px
  xxl: 48px

components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
    padding: 8px 14px
  button-primary-hover:
    backgroundColor: "{colors.primary-hover}"
    textColor: "{colors.on-primary}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
  button-secondary:
    backgroundColor: "{colors.surface-1}"
    textColor: "{colors.ink}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
    border: "1px {colors.hairline}"
    padding: 8px 14px
  button-tertiary:
    backgroundColor: "transparent"
    textColor: "{colors.ink-muted}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
    padding: 8px 14px
  panel-card:
    backgroundColor: "{colors.surface-1}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.lg}"
    border: "1px {colors.hairline}"
    padding: 20px
  panel-card-hover:
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.lg}"
    border: "1px {colors.hairline-strong}"
    padding: 20px
  input:
    backgroundColor: "{colors.surface-1}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.md}"
    border: "1px {colors.hairline}"
    padding: 8px 12px
  input-focused:
    backgroundColor: "{colors.surface-1}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.md}"
    border: "1px {colors.primary-border}"
    boxShadow: "0 0 0 2px {colors.primary-soft}"
    padding: 8px 12px
  status-badge:
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.ink-muted}"
    typography: "{typography.caption}"
    rounded: "{rounded.pill}"
    padding: 2px 10px
  agent-online-dot:
    backgroundColor: "{colors.node-online}"
    rounded: "{rounded.pill}"
    size: 8px
    boxShadow: "0 0 8px {colors.node-online}"
  sidebar:
    backgroundColor: "{colors.canvas}"
    textColor: "{colors.ink-subtle}"
    typography: "{typography.body-sm}"
    border: "1px {colors.hairline}"
    width: 200px
---

## Overview

TrustMesh is a multi-agent protocol platform: projects, agents, task flows, meetings, and node status. The UI is a **near-black operations console** — `{colors.canvas}` (#0a0a12) is a deep charcoal with a faint blue-violet tint that quietly signals the TrustMesh violet brand without shouting. On top sits a four-step surface ladder (`{colors.surface-1}` through `{colors.surface-4}`) for cards, panels, and lifted tiles, with hairline borders running `{colors.hairline}` → `{colors.hairline-strong}` → `{colors.hairline-tertiary}`.

The single chromatic accent is **TrustMesh violet** `{colors.primary}` (#6d5ff5) — used on the primary CTA, focus rings, the brand mark, agent "online" indicators, and selected nav states. Never decoratively. Status colors are borrowed from the Cursor timeline palette and mapped to the task lifecycle so users can read agent work state at a glance.

**Key Characteristics:**
- **Dark-canvas console** — near-black background across the whole application; no light mode.
- **Violet brand accent** (`{colors.primary}`) used scarcely: CTA, focus, brand, online status.
- Four-step surface ladder carries hierarchy without drop shadows.
- Task lifecycle uses the Cursor timeline palette (soft pastels), NOT saturated neon.
- Hairline 1px borders define card edges; radius stays 8px (controls) / 12px (panels).
- Motion is subtle and purposeful: surface lift on hover, soft pulses for live status, number count-up for KPIs.

## Colors

### Brand & Accent
- **Violet** (`{colors.primary}` #6d5ff5): primary CTA, brand mark, selected nav, online status, link emphasis.
- **Violet Hover** (`{colors.primary-hover}` #8b7ff8): hovered primary CTA.
- **Violet Active** (`{colors.primary-active}` #5e52d8): pressed primary CTA.
- **Violet Soft** (`{colors.primary-soft}`): focus-ring halo, selected-row tint, "online" pill backgrounds.

### Surface (the console ladder)
- **Canvas** (`{colors.canvas}` #0a0a12): app background.
- **Surface 1** (`{colors.surface-1}` #12121d): cards, panels, inputs.
- **Surface 2** (`{colors.surface-2}` #171722): hovered cards, dropdown menus.
- **Surface 3** (`{colors.surface-3}` #1c1c28): sub-panels, nested surfaces.
- **Surface 4** (`{colors.surface-4}` #232331): deepest lifted surface.
- **Hairline** / **Strong** / **Tertiary**: 1px borders at three emphasis levels.

### Text
- **Ink** (#f4f4f8): headlines and emphasized body.
- **Ink Muted** (#c8ccd8): secondary type.
- **Ink Subtle** (#8b8f9e): tertiary type, footer, disabled.
- **Ink Tertiary** (#5f6372): quaternary, placeholders.

### Semantic
- **Success** #27a644 · **Warning** #f59e0b · **Error** #ef4444 · **Info** #3b82f6.

### Task Lifecycle (Cursor-inspired pastels)
- **Planning** (#b795dd soft violet): agent decomposing the task.
- **In Progress** (#6f8ff0 soft blue): agent executing.
- **Awaiting Review** (#e0a35e soft amber): handoff waiting for review.
- **Done** (#6dc67f soft green): completed.
- **Failed** (#e25563): errored out.
- **Canceled** (#6b6f7d neutral gray).

### Node Status
- **Online** #4ade80 with a soft glow halo.
- **Busy** #60a5fa.
- **Offline** #71717a.

## Typography

### Font Family
- **Inter** (weight 400/500/600) for everything — display, body, buttons. Inter is the closest free substitute to Linear's proprietary typeface.
- **JetBrains Mono** (weight 400) for node IDs, code, timestamps, and technical tokens.

### Hierarchy
| Token | Size | Weight | Use |
|---|---|---|---|
| `{typography.display-lg}` | 44px | 600 | Page-level hero numbers |
| `{typography.display-md}` | 32px | 600 | Section headlines |
| `{typography.headline}` | 24px | 600 | Page titles, dashboard titles |
| `{typography.card-title}` | 18px | 500 | Card titles |
| `{typography.body}` | 14px | 400 | Default body |
| `{typography.body-sm}` | 13px | 400 | Table cells, list items |
| `{typography.caption}` | 12px | 400 | Meta, status, timestamps |
| `{typography.button}` | 14px | 500 | Buttons |
| `{typography.mono}` | 13px | 400 | Node IDs, code |

### Principles
- Aggressive negative tracking on display (-1.4px at 44px).
- Never weight 700+. The console stays calm.
- Mono only for technical tokens (node_id, timestamps, code).

## Layout

### Spacing System
- Base unit: 4px. Tokens xxs(4) → xs(8) → sm(12) → md(16) → lg(24) → xl(32) → xxl(48).
- Card interior padding: 20px (panels), 24px (large cards).
- Content max width ~1280px.

### Whitespace Philosophy
The dark canvas IS the whitespace. Sections separate by lifting onto surface-1 panels, not by gaps in white. Within a panel, 24px gaps between content blocks.

## Elevation & Depth

| Level | Treatment | Use |
|---|---|---|
| 0 (flat) | No shadow, no border | Body type, plain text |
| 1 (charcoal lift) | surface-1 bg + 1px hairline | Cards, panels, inputs |
| 2 (lift) | surface-2 bg + hairline-strong | Hovered cards, dropdowns |
| 3 (lift) | surface-3 bg | Sub-panels |
| 4 (focus) | 2px primary-soft ring + primary-border | Focused inputs, buttons |

No drop shadows on dark. Hierarchy comes from the surface ladder + hairlines.

## Shapes

| Token | Value | Use |
|---|---|---|
| `{rounded.xs}` | 4px | Small chips |
| `{rounded.sm}` | 6px | Inline tags |
| `{rounded.md}` | 8px | Buttons, inputs, selects |
| `{rounded.lg}` | 12px | Cards, panels, dialogs |
| `{rounded.xl}` | 16px | Large panels, modals |
| `{rounded.pill}` | 9999px | Status badges, avatars |

## Components

### Buttons
- **Primary**: violet bg (#6d5ff5), white text, 8px radius, padding 8px 14px. Hover → #8b7ff8, pressed → #5e52d8.
- **Secondary**: surface-1 bg, ink text, 1px hairline border, 8px radius.
- **Tertiary / Ghost**: transparent bg, ink-muted text; hover → ink + subtle surface.
- **Danger**: transparent, error text; hover → error-soft bg.

### Cards / Panels
- **Panel card**: surface-1 bg, 1px hairline, 12px radius, padding 20px. Hover lifts to surface-2 with hairline-strong.
- **Featured card**: surface-2 bg (one step up the ladder).

### Inputs
- surface-1 bg, 1px hairline, 8px radius, padding 8px 12px. Focus: 1px primary-border + 2px primary-soft ring.

### Status Badges
- surface-2 bg, ink-muted text, pill radius, padding 2px 10px. Colored variants use the task lifecycle palette for the dot/label.

### Sidebar
- canvas bg, ink-subtle text, 1px hairline right border, width 200px (collapsed 80px). Selected item: primary-soft bg + primary text.

### Tables
- Header: caption-size, ink-subtle, 40px height, hairline bottom border. Rows: body-sm, hairline row dividers, hover → surface-2.

## Motion

Motion is subtle, fast, and purposeful — the console should feel alive, never bouncy.

| Interaction | Duration | Easing | Effect |
|---|---|---|---|
| Card hover | 120ms | ease-out | Lift to surface-2 + hairline-strong |
| Button hover | 120ms | ease-out | Background shift only |
| Page enter | 200ms | ease-out | Content fades + 4px rise |
| Modal open | 180ms | ease-out | 4px rise + fade |
| KPI number | 800ms | ease-out | Count-up from 0 to value |
| Live agent pulse | 1.5s | linear | Online dot glow pulse (infinite) |
| Toast | 200ms | ease-out | Slide-in from top |
| Sidebar collapse | 200ms | ease | Width transition |

### Rules
- All motion < 250ms except count-up and pulse.
- No bounce/spring physics.
- No continuous animation except the online-status pulse.
- Every state change (hover, focus, active, selected) transitions at 120ms.

## Do's and Don'ts

### Do
- Keep the whole app on the near-black console; no light mode.
- Reserve violet for CTA, focus, brand, selected nav, online status.
- Use the surface ladder for hierarchy; never skip levels.
- Use the Cursor-inspired pastel palette for task states.
- Use hairline borders to define card edges.
- Let node_id / timestamps render in JetBrains Mono.
- Animate state changes subtly at 120ms.

### Don't
- Don't use saturated neon on large surfaces (save pulse glow for tiny online dots only).
- Don't add atmospheric gradients or spotlight cards.
- Don't pill-round CTAs.
- Don't use true #000000 as the canvas.
- Don't introduce a second bright accent beyond violet + semantic colors.
- Don't use bounce/springy motion.
