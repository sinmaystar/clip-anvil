# 影砧 · Clip Anvil — Frontend Design System

> 这是给 AI Coding Agent（Cursor / Claude Code / Codex / Cline / Aider 等）的**单文件交付契约**。
> 读完这一份就能复刻整套视觉语言、布局节奏、组件骨架与无限画布交互。
> 当前配套源码：`apps/web/src/main.css`、`apps/web/src/components/`、`apps/web/src/pages/`、`apps/web/src/shapes/`。

最后更新：2026-06-13 · 维护者：设计自治 · 版本：v0.4 (M1 implementation sync)

---

## 0. 在动手前必须读懂的三句话

1. **品牌隐喻**：影砧 = 锻造铁砧 + 影像剪辑。所有视觉都从「锻造车间在深夜工作 / 火星迸溅 / 高精度仪表盘」这条隐喻出发，绝不是「AI generic neon」。
2. **风格定位**：Forge Workshop（Linear/Vercel 纯黑 + 火星橙）作为**底**，叠加 Storyboard Atelier（暖琥珀次色 + 胶片孔）和 Spatial OS（毛玻璃 + 视差悬浮）作为**调味层**。最终目标是**Apple-grade 克制感**——所有"高级"的来源是材质、阴影、字距，不是颜色饱和度。
3. **三禁忌**：
   - ❌ 默认紫色 / 蓝紫渐变背景（AI generic palette）
   - ❌ 大面积 `0 0 16px brand` 外发光（看起来像游戏 UI）
   - ❌ Lorem ipsum / 占位图 / `#A855F7` 等被 lint 标记的廉价色

---

## 1. 设计哲学 · Forge × Storyboard × Spatial OS 三层叠加

| 层 | 来源 | 在 UI 中的体现 |
|---|---|---|
| **底色 · Forge** | Linear / Vercel 纯黑工作台 | 背景 `#050507`，火星橙 `#FF6A2A` 作单一品牌色，极薄 `rgba(255,255,255,0.06)` 边线 |
| **次色 · Storyboard** | 电影分镜本 / 胶片穿孔 | 画布组容器使用虚线"齿孔"重复渐变，琥珀色 `#FFB347` 在节点头部、灯光台高光、运行进度上点缀 |
| **悬浮 · Spatial OS** | visionOS / macOS Sonoma | Topbar、ChatPanel、Floating Action 用 `backdrop-filter: blur(20px) saturate(1.6-1.8)` 做毛玻璃；最终成片节点 `.is-final` 用 `translateY(-2px)` 制造"悬浮台"感 |

**叠加规则**：底层永远是 Forge（不可被覆盖）；次色和悬浮只在限定区域出现，避免均匀涂抹稀释紧张感。

---

## 2. Design Tokens（唯一真相源）

所有颜色、间距、半径、动效**必须**通过 Token 引用。直接写 hex 等于违约。

当前项目没有独立的 `tokens.css` / `hybrid.css`，M1 的 token 真相源集中在 `apps/web/src/main.css`。后续如拆分样式文件，必须保持 token 名称兼容，避免组件层重写裸色值。

### 2.1 Seed Tokens（六个种子，覆盖一切）

```css
--seed-bg:       #050507;   /* 工作台底色 */
--seed-fg:       #f5f5f7;   /* 主前景文本 */
--seed-primary:  #ff6a2a;   /* 火星橙 — 品牌主色 */
--seed-accent:   #ffb347;   /* 琥珀 — 次品牌色，灯光/运行进度/胶片孔 */
--seed-surface:  #0b0b0e;   /* 面板表面色（比 bg 浅一档） */
--seed-radius:   8px;       /* 基础圆角 */
```

> Appearance 切换（Light / Forge 暖深 / 等）只 patch 这 6 个 seed，所有派生色用 `color-mix()` 自动跟随。**永远不要新增第七个 seed。**

### 2.2 派生 Tokens（参考表，落地见 tokens.css）

```css
/* 表面亮度阶梯 — Linear 风格的 +N% 白叠加 */
--color-canvas:          var(--seed-bg);
--color-panel:           var(--seed-surface);
--color-panel-elevated:  color-mix(in srgb, white 4%, var(--seed-surface));
--color-surface-2:       color-mix(in srgb, white 6%, var(--seed-surface));
--color-surface-hover:   color-mix(in srgb, white 8%, var(--seed-surface));

/* 边框 — 用半透明白做物质化 hairline，不要硬灰 */
--border-subtle:   rgba(255, 255, 255, 0.05);
--border-default:  rgba(255, 255, 255, 0.08);
--border-strong:   rgba(255, 255, 255, 0.12);

/* 文字四阶 */
--fg-primary:      #f5f5f7;
--fg-secondary:    #c9c9d1;
--fg-tertiary:     #8a8a93;
--fg-quaternary:   #5a5a63;
--fg-on-primary:   #1a0d05;   /* 橙色按钮上的深棕字，不要纯白 */

/* 品牌派生 */
--accent:          var(--seed-primary);
--accent-hover:    color-mix(in srgb, white 10%, var(--seed-primary));
--accent-glow:     color-mix(in srgb, var(--seed-primary) 35%, transparent);
--accent-soft:     color-mix(in srgb, var(--seed-primary) 12%, transparent);
--accent-amber:    var(--seed-accent);

/* 边线语义色（DAG 边） */
--edge-dependency: #3b82f6;   /* 蓝实线 — 依赖 */
--edge-reference:  #b26bff;   /* 紫虚线 — 引用（注意是 b26bff 不是 a855f7） */
--edge-sequence:   #22c55e;   /* 绿实线 — 时序 */

/* 节点状态色（State Machine） */
--status-draft:      #3a3a42;
--status-ready:      #6b6b75;
--status-queued:     #8a8a93;
--status-running:    #3b82f6;
--status-succeeded:  #22c55e;
--status-failed:     #ef4444;
--status-stale:      #f59e0b;
```

### 2.3 间距系统（8px base）

```
--space-1: 4px   --space-2: 8px    --space-3: 12px
--space-4: 16px  --space-5: 20px   --space-6: 24px
--space-8: 32px  --space-10: 40px  --space-12: 48px
```

**只用以上这 9 档**。不要写 `padding: 7px` / `margin: 13px` 这种自由值。

### 2.4 圆角阶梯

```
--radius-xs: 4px      小徽章、标签
--radius-sm: 6px      Input、小按钮
--radius-md: 8px      面板、节点卡片  ← 默认
--radius-lg: 12px     大对话框
--radius-xl: 16px     Hero / 介绍卡
--radius-pill: 9999px 主按钮、芯片、玻璃胶囊  ← Apple 标志
```

### 2.5 布局变量

```
--topbar-h:   56px   顶栏高度（M1 仅列表/认证壳使用；Studio 画布页不再叠多层顶栏）
--sidebar-w:  256px  左侧资源树/媒体侧栏宽
--chat-w:     380px  右侧对话面板宽（M2 Agent 使用）
```

目标主工作区是 `[topbar][sidebar | canvas | chat]` 三列布局；M1 Studio 当前使用 `[sidebar | canvas]` 两列专注画布布局，避免用户感知过多工具栏。

---

## 3. Typography · 字体系统

```css
font-family:
  'Inter',                       /* 主字体 */
  'SF Pro Display',              /* 苹果系统 fallback */
  system-ui,
  -apple-system,
  sans-serif;

font-feature-settings: 'cv11' 1, 'ss01' 1, 'ss03' 1;
/* 这三个 Inter feature 让 1/I/0 更接近 SF Pro 的几何形 */
```

### 字号阶梯（不要自由发挥）

| 用途 | size | weight | letter-spacing |
|---|---|---|---|
| Hero 标题 / 大数字 | 28-32px | 600 | -0.02em |
| 区块大标 | 18px | 600 | -0.018em |
| 卡片标题 | 14px | 600 | -0.015em |
| 正文 | 13px | 400 | -0.01em |
| 次要正文 / 标签 | 12px | 500 | -0.008em |
| 元信息 / 时间戳 | 11.5px | 500 | 0 |
| Mono（数字、ID、时长） | 11.5px | 500 | JetBrains Mono |

**关键规则**：所有 ≥14px 的展示文本必须 `letter-spacing: -0.015em`。这是从无设计感到 Apple 感的最便宜跃迁。

---

## 4. Material · 玻璃 / 阴影 / 边线系统

### 4.1 三种"材质"

| 材质 | 用途 | 配方 |
|---|---|---|
| **实心面板** | 资源树、左侧导航 | `background: var(--color-panel)` + 1px hairline |
| **抬升面板** | 卡片、消息气泡 | `var(--color-panel-elevated)` + `--shadow-card` |
| **毛玻璃** | Topbar、ChatPanel、Floating Actions、Popover | `backdrop-filter: blur(20-24px) saturate(1.6-1.8)` + `rgba(15,15,17, 0.55)` 半透明黑底 |

### 4.2 阴影栈（用 Token，不要自创）

```css
/* 卡片：物质边线 + 1px 黑色基线 */
--shadow-card:
  0 0 0 1px var(--border-default),
  0 1px 0 0 rgba(0, 0, 0, 0.4);

/* 抬升：浮起感，4-16px 阴影 */
--shadow-elevated:
  0 0 0 1px var(--border-default),
  0 4px 16px rgba(0, 0, 0, 0.4),
  0 1px 0 0 rgba(255, 255, 255, 0.04) inset;

/* Popover：最强浮起 */
--shadow-popover:
  0 0 0 1px var(--border-strong),
  0 12px 32px rgba(0, 0, 0, 0.55),
  0 1px 0 0 rgba(255, 255, 255, 0.05) inset;
```

### 4.3 火花强调（仅用于"运行中"节点 / 主 CTA hover）

```css
--shadow-spark:
  0 0 0 1px color-mix(in srgb, var(--accent) 50%, transparent),
  0 0 24px var(--accent-glow);
```

⚠️ 火花阴影**全站 ≤2 处**（运行中节点 + 主 CTA），多了就廉价。

### 4.4 边线哲学

**永远不用** `border: 1px solid #2a2a2a`。**永远用** `border: 1px solid rgba(255,255,255,0.06)`。
理由：半透明白会跟随底色变化（dark / light / forge appearance），永远协调；硬灰只在一个底色下好看。

---

## 5. 按钮系统 · Apple-grade Pills

### 5.1 主 CTA（"预览成片"、"发送"、primary action）

```css
.btn-primary {
  border: 0;
  border-radius: 999px;                    /* pill 是高级感的免费票 */
  letter-spacing: -0.015em;
  font-weight: 590;                        /* 介于 medium 和 semibold */
  padding: 0 18px;
  height: 36px;

  /* 三段渐变模拟 tinted glass，不是平涂橙 */
  background: linear-gradient(
    180deg,
    color-mix(in srgb, var(--accent) 100%, white 14%) 0%,
    var(--accent) 50%,
    color-mix(in srgb, var(--accent) 100%, black 6%) 100%
  );

  /* 五层阴影栈：内顶高光 + 内底暗线 + 1px ambient + 品牌色软投影 + 1px 品牌色 base */
  box-shadow:
    inset 0 0.5px 0 rgba(255, 255, 255, 0.22),
    inset 0 -0.5px 0 rgba(0, 0, 0, 0.16),
    0 1px 1px rgba(0, 0, 0, 0.22),
    0 6px 18px color-mix(in srgb, var(--accent) 24%, transparent),
    0 1px 0 color-mix(in srgb, var(--accent) 18%, transparent);

  transition:
    transform 220ms cubic-bezier(0.22, 1, 0.36, 1),
    box-shadow 200ms cubic-bezier(0.22, 1, 0.36, 1);
}

.btn-primary:hover  { transform: translateY(-1px); /* 同时投影从 18px 涨到 28px */ }
.btn-primary:active { transform: translateY(0.5px) scale(0.985); transition-duration: 80ms; }
```

**口诀**：内 0.5px 高光、不要 1px；ambient shadow 颜色 = 按钮颜色；hover 上浮 1px 永远比变色高级。

### 5.2 次级 CTA（"复制分享链接"、outline）

```css
.btn-secondary {
  border: 0;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.05);
  backdrop-filter: blur(20px) saturate(1.6);
  font-weight: 540;
  letter-spacing: -0.012em;
  color: var(--fg-primary);

  box-shadow:
    inset 0 0.5px 0 rgba(255, 255, 255, 0.10),
    inset 0 0 0 1px rgba(255, 255, 255, 0.06),
    0 1px 2px rgba(0, 0, 0, 0.30);
}
.btn-secondary:hover { background: rgba(255, 255, 255, 0.09); transform: translateY(-1px); }
```

### 5.3 Ghost / Icon Button

默认完全透明，hover 才浮现 `rgba(255,255,255,0.05)` 圆角 8px 底。**Topbar 所有图标按钮、聊天 composer 内的小图标全部用这个。**

### 5.4 按钮反模式

| ❌ 不要 | ✅ 改成 |
|---|---|
| `box-shadow: 0 0 16px brand` 大外发光 | `0 6px 18px color-mix(in srgb, brand 24%, transparent)` 投影 |
| `border: 1px solid brand` 硬色边 | `box-shadow: inset 0 0 0 1px rgba(255,255,255,0.06)` |
| `border-radius: 4-6px` 工业小圆角 | `999px` pill |
| `inset 0 1px 0 rgba(255,255,255,0.25)` 强顶光 | `inset 0 0.5px 0 rgba(255,255,255,0.22)` 极细 |
| Hover 改 background-color | Hover 改 transform + brightness + 阴影距离 |

---

## 6. 主布局 · Agent 模式工作台

```
┌──────────────────── Topbar 56px (frosted) ─────────────────────┐
│ 🪨 影砧 ClipAnvil  ☕ 项目名 ▾  | Agent ◉  |  ⌘K   🔔 ⚙️ 👤  │
├────────────┬────────────────────────────────┬──────────────────┤
│            │                                │                  │
│ Resource   │      Infinite Canvas           │   Agent Chat     │
│ Tree 256px │      (dot-grid 26px,           │   Panel 380px    │
│            │       pan/zoom)                │   (frosted)      │
│ • Search   │                                │   • Status       │
│ • Filters  │      [Nodes + Edges            │   • Messages     │
│ • Groups   │       + Group containers]      │   • GateCard     │
│   - 素材   │                                │   • Composer     │
│   - 镜头   │      [Floating Actions:        │                  │
│   - 成片   │       预览成片 / 复制分享]     │                  │
│            │                                │                  │
└────────────┴────────────────────────────────┴──────────────────┘
```

CSS Grid 写法：

```css
.workspace {
  display: grid;
  grid-template-columns: var(--sidebar-w) 1fr var(--chat-w);
  grid-template-rows: var(--topbar-h) 1fr;
  height: 100vh;
}
```

---

## 7. 关键组件契约

每个组件都有**必须保留的 className 接口**——其他 Coding Agent 应优先复用这些类，避免重命名。

### 7.1 Topbar

```
.topbar (frosted) ─── .brand-wordmark + AnvilLogo SVG
                  └── .project-pill (☕ 项目名 ▾)
                  └── .mode-switch (Studio / Agent toggle)
                  └── .quick-search-trigger (⌘K)
                  └── .topbar-icons (notif / settings / avatar)
```

- 图标按钮全部 ghost 样式
- 品牌 wordmark 字距 `-0.02em`、weight 600
- 模式切换是带胶囊背景的 segmented control，激活段使用 `--accent-soft` 底 + 1px `--accent` 边

### 7.2 ResourceTree

```
.resource-tree
├── .search-input (圆角 8px, 透明 5% 白底)
├── .filter-chips (5 segmented chips, 激活使用 accent 下划线)
├── .tree-group
│   ├── .group-header (uppercase, 11.5px, mono count badge)
│   └── .tree-item
│       ├── .item-media   (28×20 缩略图: 真实 PNG > 品牌 logo glyph > 类型 icon tile)
│       ├── .item-name    (13px / 500)
│       └── .item-status  (单点 6px 状态色圆点)
```

**缩略图优先级**（重要）：
1. 真实 PNG（产品主图、关键 shot）
2. 品牌 logo 渐变块（带文字首字母）
3. 引用片 mini graphic（4:3 + 圆点表示按钮）
4. 音频波形 SVG tile
5. 类型 icon fallback（保留 TypeIcon 组件别删）

### 7.3 MediaNode（画布节点） — 见第 9 节详述

### 7.4 ChatPanel

```
.chat-panel (frosted, 380px)
├── .chat-header
│   ├── AgentStatus pulse + "Producer 思考中"
│   └── 进度芯片 "3/5 生成中"
├── .chat-stream
│   ├── .phase-divider ("—— 编剧阶段 ——" 全大写 11.5px)
│   ├── .msg-user (右对齐, max-width 80%, 品牌色边发光极弱 12%)
│   ├── .msg-agent (左对齐, 引用节点时附 NodeReferenceCard)
│   └── .gate-card (内嵌, 见 7.5)
└── .chat-composer
    ├── .composer-input ("@" 触发节点引用)
    ├── .ci-buttons (附件 / 提示词模板, ghost)
    └── .send-btn (主 CTA pill)
```

### 7.5 GateCard（关卡审批卡）

DAG 跑到关卡（如 storyboard_review、final_review）时插入对话流。

```
┌─ 故事板审核 ─────────────────────────────┐
│ 🎬 5 个镜头  ⏱ 30s 总时长  💰 ¥3-5      │
│                                          │
│ [shot01] [shot02] [shot03] [shot04] [shot05]
│  3s      7s       6s       8s       5s    │
│                                          │
│ [审核通过]  [继续编辑]  [退回重写]      │
└──────────────────────────────────────────┘
```

- 容器使用 `--shadow-elevated` + `--radius-lg`
- 三按钮第一个 primary，后两个 secondary glass
- ShotMini 是 SVG（不要用 emoji 占位）

---

## 8. 无限画布 · 核心要点

### 8.1 画布背景

```css
.canvas-area {
  background-color: var(--color-canvas);
  background-image: radial-gradient(
    circle at 1px 1px,
    color-mix(in srgb, var(--accent-amber) 10%, var(--fg-quaternary)) 0.8px,
    transparent 0
  );
  background-size: 26px 26px;   /* 不是 32 不是 20，就是 26 */
}
```

26px 间距 + 0.8px 点 + 10% 琥珀混入 = "深夜车间地砖"质感。**点阵不要太亮，否则像 Excel。**

### 8.2 节点尺寸（严格）

| 类型 | 宽 | 高 | 备注 |
|---|---|---|---|
| text | 200 | 120 | 提示词 / 旁白 |
| image | 200 | 160 | 静帧 / 参考图 |
| video | 240 | 180 | 镜头 / 成片 |
| audio | 200 | 80  | BGM / 配音 |

### 8.3 节点卡片结构（4 段）

```
┌──────────────────────────┐ ← 状态边框（见 8.4）
│ Header 32px              │ icon + 标题 + 类型标签 + 状态点
├──────────────────────────┤
│ Content (变高)           │ thumbnail / 文本 / 波形
├──────────────────────────┤
│ Refs bar 20px            │ 引用资源链 chips（可省）
├──────────────────────────┤
│ Action bar 32px          │ ▶ 生成 · ✎ 编辑 · ⋯ 更多
└──────────────────────────┘
        ◉ ports（左入右出）
```

### 8.4 状态机 · 边框 / 装饰必须严格匹配

| 状态 | 边框 | 附加视觉 | 触发条件 |
|---|---|---|---|
| **Draft** | `1px dashed --status-draft` | 浅灰，content 文字 placeholder 风 | 用户刚创建未生成 |
| **Ready** | `1px solid --status-ready` | 无 | 输入完整、可生成 |
| **Queued** | `1px solid --status-queued` | 头部 ⏳ 旋转 | 在 Agent 任务队列 |
| **Running** | `2px solid --status-running` + spark glow | 头部 1px 进度条（top edge）+ ETA chip | Producer 正在执行 |
| **Succeeded** | `2px solid --status-succeeded` | 右上角 score 徽章（如 "评分 4.8"） | 生成完成 |
| **Failed** | `2px solid --status-failed` | 头部 ✗ + 错误浮层 | 模型/渲染失败 |
| **Stale** | `2px dashed --status-stale` | 半透明琥珀蒙版 + ⚠ 角标 | 上游变更后未重跑 |
| **UserEditing** | `2px solid #f97316` | 头部 ✎ 编辑光标动画 | 多人协作锁 |

**关键**：状态变化由 React state 驱动 className，CSS 通过 `[data-status="running"]` 选择器切换。**不要把状态 hardcode 进 className**（避免 7 套类名爆炸）。

```jsx
<div className="media-node" data-status={node.status} data-type={node.type}>
```

### 8.5 三种边类型（DAG）

```
依赖边 — 蓝实线 #3b82f6  → 这个节点必须等上游完成才能跑
引用边 - 紫虚线 #b26bff  → 此节点用上游作为风格/素材参考
时序边 → 绿实线 #22c55e  → 镜头组合的播放顺序
```

SVG 实现：每条边一个 `<path>` + 末端 `<marker>` 箭头。运行中节点的输出边可以叠 `stroke-dasharray` 流动动画。

### 8.6 组容器（Group）

用作"5 个镜头组成一段成片"等聚合。

```css
.canvas-group {
  border: 1px dashed color-mix(in srgb, var(--accent-amber) 14%, var(--border-default));
  background:
    /* 顶部 1px 胶片孔图案 */
    repeating-linear-gradient(90deg,
      transparent 0, transparent 12px,
      color-mix(in srgb, var(--accent-amber) 5%, transparent) 12px,
      color-mix(in srgb, var(--accent-amber) 5%, transparent) 13px
    ) top / 100% 1px no-repeat,
    /* 整体超薄琥珀染 */
    color-mix(in srgb, var(--accent-amber) 1.5%, transparent);
  border-radius: 12px;
  padding: 24px;
}
```

### 8.7 最终成片节点（特殊）

`.is-final` 节点享受 Spatial OS 待遇：

```css
.media-node.is-final {
  transform: translateY(-2px);
  box-shadow:
    0 0 0 1px var(--border-strong),
    0 16px 48px rgba(0, 0, 0, 0.6),
    0 0 32px color-mix(in srgb, var(--accent-amber) 18%, transparent);
}
```

---

## 9. Motion · 动效原则

| 类型 | 时长 | 缓动 | 用途 |
|---|---|---|---|
| 微交互（hover、focus） | 140ms | `cubic-bezier(0.22, 1, 0.36, 1)` | 按钮、icon |
| 卡片 / 面板进入 | 220ms | 同上 | drawer、popover |
| 状态切换（running 进度条） | 1200ms 循环 | `linear` | shimmer、pulse |
| 错误抖动 | 300ms | `cubic-bezier(0.36, 0, 0.66, -0.56)` | failed 状态 |

**永远使用 `--ease-out` token**（`cubic-bezier(0.22, 1, 0.36, 1)`），不要写 `ease-in-out` 之类的浏览器默认。

**Active 反馈三件套**：`transform: scale(0.985) translateY(0.5px)` + `transition-duration: 80ms` + 阴影距离压缩。

---

## 10. 反模式总表（Anti-Patterns）

| 反模式 | 修复 |
|---|---|
| 紫色渐变 hero / dashboard 背景 | 纯黑 + 火星橙 accent，渐变只在按钮表面用三段微变 |
| AI 生成图都用 oklch / 紫蓝 vapor | 火星橙 + 琥珀 + 暖灯光摄影 |
| 大段 `0 0 16-32px brand` 外发光 | 投影 = 按钮颜色 24% 透明度，6-18px 距离 |
| 1px solid #2a2a2a 等硬灰边 | `rgba(255,255,255,0.06)` 半透明白 |
| 所有按钮 6-8px 圆角 | 主 CTA 必 999px pill |
| Hover 改 background-color | Hover 改 transform + 阴影距离 + brightness |
| 状态用文字 "Running..." | 状态用边框 + 颜色 + 角标，文字仅作辅助 |
| 缩略图统统用 emoji 🎬 | 优先真 PNG，其次 SVG 自绘 mini graphic，最后 TypeIcon tile |
| 字体不指定 letter-spacing | 所有 ≥14px 必 `-0.015em`；display 文本 `-0.02em` |
| 把 hex 写在组件里 | **只准引 var(--token-xxx)** |

---

## 11. 给其他 Coding Agent 的提示词模板

本项目的视觉契约入口是 `docs/design/frontend.md`。如果外部工具硬编码读取旧设计系统入口，保留转向说明文件即可，不要维护两份互相漂移的设计系统。

### 11.1 M1.x 当前 CSS 命名范围

当前已落地页面优先使用以下 class 前缀，新增同类组件时沿用现有风格：

| 前缀 | 用途 |
|---|---|
| `auth-*` | 登录/注册页 |
| `app-*` | 应用壳、导航和通用布局 |
| `workspace-*` | 项目列表、创建项目弹窗 |
| `studio-*` | Studio 画布页、左侧栏、右键菜单、节点编辑面板 |
| `media-node-*` | tldraw 自定义媒体节点 |
| `group-container-*` | tldraw 自定义分组容器 |
| `resource-tree-*` | 左侧资源树 |
| `auto-layout-*` | 自动布局控件 |
| `modal-*` | 弹窗和遮罩 |

把这份文档作为上下文，然后给 AI 这样的指令：

```
请阅读 docs/design/frontend.md 作为本项目的视觉契约。
新组件必须：
1. 仅使用 apps/web/src/main.css 中的 CSS 变量，禁止写裸 hex
2. 主 CTA 使用 999px pill + Apple-grade 阴影栈（见文档 §5.1）
3. 边线全部 rgba(255,255,255,0.06) hairline，不准 #2a2a2a
4. 节点状态变化通过 data-status 属性 + CSS 切换，禁止 7 套类名
5. 字距：所有 ≥14px 文本 letter-spacing: -0.015em
6. 任何"高级感"诉求 → 增加 backdrop-filter / 收紧字距 / 减少边框对比，绝不增加颜色饱和度

参考文件：
- apps/web/src/main.css                   ← Token + 全局视觉真相源
- apps/web/src/components/Layout.tsx      ← 应用壳
- apps/web/src/pages/WorkspaceListPage.tsx
- apps/web/src/pages/WorkspaceDetailPage.tsx
- apps/web/src/shapes/MediaShapeUtil.tsx
```

---

## 12. 文件索引（Coding Agent 优先读这些）

```
docs/
├── README.md
├── design/
│   ├── frontend.md          ← 本文档（视觉契约总入口）
│   ├── overview.md          架构（4 层）+ 双模式
│   ├── canvas.md            MediaShape props + 节点尺寸细节
│   ├── agent-mode.md        Agent 多角色 + Skills YAML + Gate 流程
│   └── studio-mode.md       Studio 模式三栏 + 属性面板
└── engineering/
    ├── architecture.md
    ├── database.md
    ├── deployment.md
    └── development.md

apps/web/src/
├── main.css                        Seed + 派生 + 状态色 + 页面样式
├── components/
│   ├── Layout.tsx
│   ├── CreateWorkspaceDialog.tsx
│   ├── ResourceTree.tsx
│   ├── PropertyPanel.tsx
│   ├── FileDropZone.tsx
│   ├── ProtectedRoute.tsx
│   └── GuestRoute.tsx
├── pages/
│   ├── LoginPage.tsx
│   ├── RegisterPage.tsx
│   ├── WorkspaceListPage.tsx
│   └── WorkspaceDetailPage.tsx
├── shapes/
│   ├── MediaShapeUtil.tsx
│   └── GroupContainerShapeUtil.tsx
└── App.tsx                         RouterProvider 路由入口
```

---

## 13. 版本变更（保留供回溯）

- **v0.1** · 初版，Forge Workshop 单一方向，按钮使用 `0 0 16px brand` 外发光
- **v0.2** · 加入 hybrid.css，叠加 Storyboard（胶片孔）+ Spatial OS（毛玻璃）层
- **v0.3** · Apple polish，按钮去外发光改投影，pill 化，全局字距收紧，点阵降亮
- **v0.4** · 同步 M1 实际文件结构、Studio 两列画布布局、当前 CSS class 契约
- **v0.5** · 同步 M1.x 文档目录、资源树、属性面板、分组容器、上传和自动布局入口 ← **当前**

---

> 复刻这套设计的"高级感"，本质是三件事：**materials over colors（材质胜过颜色）、shadows over borders（阴影胜过边框）、letter-spacing over font-weight（字距胜过字重）**。把这三条刻进每个组件，影砧的视觉语言就稳了。
