# ClipAnvil Frontend Design System

> 这是给 AI Coding Agent 的前端视觉与交互契约。当前源码入口：
> `apps/web/src/main.css`、`apps/web/src/pages/`、`apps/web/src/components/`、
> `apps/web/src/shapes/`。

最后更新：2026-06-18 · 版本：v0.5 · 当前实现同步

## 1. 设计定位

影砧是 AI 视频生成工作台，不是传统管理系统。视觉目标是 Apple / Figma / liblib.tv 方向的克制、扁平、玻璃化工作界面：画布优先，控件像工具而不是表单；信息密度高，但每个层级清楚。

当前产品体验以 Studio 手动编辑为核心：

- Auth、Workspace、Studio 使用同一套冷调视觉系统。
- Studio 画布全屏铺底，左侧资源导航浮在画布之上。
- 节点详情通过单击节点后的画布浮层展示，不再使用右侧 Inspector。
- 主题切换入口在导航栏中，全站使用同一套 light / dark appearance。

## 2. 视觉原则

1. **画布优先**：Studio 的第一视觉主体永远是无限画布。导航、工具栏、菜单都是浮层，不与画布平级抢空间。
2. **扁平但有材质**：按钮和节点不做厚重拟物，不使用强投影、凸起、发光；通过透明度、hairline、轻微 blur 和层级差表达材质。
3. **不要渐变**：当前系统禁用线性/径向渐变作为背景、按钮或连线装饰。颜色来自 token、透明叠加和 `color-mix()`。
4. **冷调主色**：当前主色是克制蓝 `#2563eb`，辅助色是低饱和青绿 `#14b8a6`。避免回到偏橙、偏旧、偏后台系统的氛围。
5. **少而准的玻璃化**：玻璃效果用于浮层、导航、popover、toolbar；不要把每个卡片都做成高亮玻璃。
6. **节点是画布对象**：节点可以接近方形，圆角很小；控件可以 pill，但节点本体不要像圆润卡片。

## 3. Design Tokens

Token 真相源是 `apps/web/src/main.css`。不要在组件里散写新的主题色。

### 3.1 Seed Tokens

```css
--seed-bg: #0a0b0d;
--seed-fg: #f7f8fb;
--seed-primary: #2563eb;
--seed-accent: #14b8a6;
--seed-surface: #101114;
--seed-radius: 8px;
```

Light appearance 通过覆盖 seed 和派生 surface 完成。新增主题必须先改 seed，再让派生 token 自动跟随。

### 3.2 常用派生层

- `--color-canvas`：页面和 Studio 画布底色。
- `--color-panel-elevated`：浮层面板、auth 卡片、workspace 卡片。
- `--color-surface-hover`：列表项和按钮 hover 底。
- `--border-subtle/default/strong`：所有边线必须使用这些半透明 hairline。
- `--accent`：主交互色，当前为蓝。
- `--accent-secondary`：辅助氛围色，当前为青绿。
- `--accent-soft`：选中态、类型 badge、轻量强调底。

### 3.3 状态色

状态色只表示状态，不承担品牌表达：

| Token | 用途 |
|---|---|
| `--status-draft` | 草稿 |
| `--status-ready` | 就绪 |
| `--status-queued` | 队列中 |
| `--status-running` | 生成中 |
| `--status-succeeded` | 成功 / 已连接 |
| `--status-failed` | 失败 / 危险操作 |
| `--status-stale` | 上游变更后的过期 |

## 4. Layout

### 4.1 App Shell

非 Studio 页面使用 `Layout` 顶栏：

- 左侧品牌与产品名。
- 右侧全局 appearance toggle、账号信息。
- 顶栏是 sticky frosted bar，不能变成传统后台导航条。

### 4.2 Auth

登录和注册页必须保留产品叙事，不允许退回普通居中表单：

- 首屏是两栏但同一卡片内的沉浸式 auth stage。
- 左侧解释 AI Video Studio 的工作方式。
- 右侧是登录/注册表单。
- 背景和卡片都使用当前 token，不使用占位插画、渐变 hero 或廉价装饰图。

### 4.3 Workspace List

Workspace 页面是项目入口，不是后台列表：

- 页面宽度受控，信息密度适中。
- workspace card 使用扁平边线和轻 surface，不做重投影。
- 主操作按钮使用 pill，但不要浮夸发光。

### 4.4 Studio

Studio 是全屏工作台：

```text
┌──────────────────────────────────────────────┐
│  floating sidebar / peek                     │
│                                              │
│                 floating toolbar             │
│                                              │
│             infinite tldraw canvas           │
│                                              │
│             auto layout controls             │
└──────────────────────────────────────────────┘
```

当前结构：

- `.studio-shell` 是 `position: relative` 的全屏容器。
- `.studio-canvas-frame` 是 `position: absolute; inset: 0;` 的画布底。
- `.studio-sidebar` 是左上角绝对定位浮层，展开态宽 298px。
- sidebar 收起后只保留 `.studio-sidebar-peek`，展示“影 + 项目名 + 展开箭头”。
- 不再渲染右侧 Inspector；节点属性由 `NodeEditorOverlay` 承担。

## 5. Studio Navigation

展开态左侧浮层包含：

- Studio / workspace title。
- WebSocket 连接状态。
- 收起按钮。
- 项目列表按钮。
- appearance toggle。
- ResourceTree：新建分组、搜索、类型筛选、分组和节点列表。
- 用户信息与登出。

视觉要求：

- 连接状态使用状态色，不要灰色胶囊。
- 项目列表和新建分组是主要动作，可使用深色实心 pill。
- appearance toggle 是轻强调 icon button。
- 搜索框使用轻蓝 surface 和边线，不要普通灰输入框。
- 登出是低优先级危险色，不要和主要按钮同样权重。

收起态要求：

- 不保留挤压后的列表内容。
- 只保留左上角悬浮 project peek。
- peek 必须显示项目名，避免用户失去当前 workspace 上下文。
- 展开按钮使用明确箭头，不使用难懂符号。

## 6. Canvas Nodes

节点是画布对象，不是 dashboard card。

尺寸约束由节点类型决定：

| 类型 | 宽 | 高 |
|---|---:|---:|
| text | 200 | 120 |
| image | 200 | 160 |
| video | 240 | 180 |
| audio | 200 | 80 |

视觉要求：

- 节点本体小圆角，当前约 4px。
- 节点内部图标、状态 badge 也使用小圆角。
- 内容区按媒体类型展示文本、图片、视频预览或音频波形。
- 选中态使用细蓝边和低透明 halo，不做大面积发光。
- 运行/成功/失败等状态只改变边线和局部状态，不重绘整张卡片。

## 7. Connections

当前只暴露 dependency 连线。

交互规则：

- 连接起点只在节点右侧出现。
- 默认不显示左右两个端口。
- hover 或选中节点后，在节点右侧外侧显示一个 `+` 小按钮。
- `+` 不侵入节点内容区。
- 拖出连线后，释放到目标节点任意位置即可创建依赖。
- 不需要左侧接收端口。

视觉规则：

- 连线使用细蓝 stroke，箭头跟随 `--accent`。
- 选中线只略微加粗，不使用渐变描边。
- 拖拽预览可使用虚线和细流动线，但必须保持轻量。
- 成环等失败反馈用 toast，不在画布上堆叠强警告面板。

## 8. Node Editor Overlay

单击节点后的浮层是当前节点详情主入口。

它负责：

- 展示输入引用。
- 编辑标题。
- 编辑 Prompt。
- 选择模型。
- 显示自动保存状态。

设计要求：

- 浮层跟随节点位置，不固定在右侧。
- 浮层层级高于连线和节点。
- 点击画布空白、按 Escape 或选中其他对象时关闭。
- 不要再同时打开右侧 Inspector，避免重复编辑入口。

## 9. Context Menu

画布右键菜单用于精确创建节点：

- 文本节点。
- 图片节点。
- 视频节点。
- 音频节点。

交互要求：

- 菜单出现后，点击画布其他区域必须自动关闭。
- 菜单是 fixed popover，不跟随 tldraw 缩放。
- 菜单项使用类型 badge + 标题 + 描述。

## 10. Auto Layout

自动整理使用 Dagre，并且必须考虑浮层占位。

当前规则：

- sidebar 展开时，整理结果整体平移到浮层右侧安全区，当前屏幕起点约 `x=360, y=112`。
- sidebar 收起时，整理结果整体平移到左上 project peek 右侧安全区，当前屏幕起点约 `x=120, y=112`。
- 安全起点会通过 tldraw `screenToPage()` 转成 page 坐标，保证缩放/平移后仍正确。
- 布局函数 `computeDagreLayout` 支持可选 `origin`，测试见 `apps/web/src/lib/layout.test.mjs`。

不要只用 CSS padding 模拟避让；节点坐标本身需要落在可见安全区。

## 11. Motion

动效只用于解释层级和状态：

| 类型 | 时长 | 用途 |
|---|---:|---|
| hover / focus | 120-180ms | 按钮、节点连接按钮 |
| panel / popover | 180-220ms | 浮层进入和层级变化 |
| connection flow | 1.1-1.2s loop | 拖拽或已有依赖线的轻量流动 |
| error feedback | 300-4800ms | toast 可读性 |

统一使用 `--ease-out`。不要使用弹跳、大幅缩放、连续闪烁。

## 12. Anti-patterns

禁止：

- 渐变背景、渐变按钮、渐变连线。
- 紫蓝 AI generic 主视觉。
- 橙色/棕色/咖啡色作为主主题。
- 大面积灰按钮导致所有控件同权重。
- 右侧 Inspector 与节点浮层并存。
- sidebar 收起后仍显示被挤压的原内容。
- 节点左右半圆端口侵入节点本体。
- 使用 `V / T / I / A` 这类内部缩写作为可见导航或节点工具。
- 用 hero 文案解释功能代替真实可用界面。
- 在 Studio 里恢复传统管理系统三栏布局。

## 13. 实现映射

| 设计对象 | 当前源码 |
|---|---|
| tokens / 全局样式 | `apps/web/src/main.css` |
| Auth 页面 | `apps/web/src/pages/LoginPage.tsx`, `RegisterPage.tsx` |
| Workspace 页面 | `apps/web/src/pages/WorkspaceListPage.tsx` |
| Studio 页面 | `apps/web/src/pages/WorkspaceDetailPage.tsx` |
| 资源树 | `apps/web/src/components/ResourceTree.tsx` |
| 连线 overlay | `apps/web/src/components/ConnectionOverlay.tsx` |
| 媒体节点 shape | `apps/web/src/shapes/MediaShapeUtil.tsx` |
| 自动整理 | `apps/web/src/lib/layout.ts` |

## 14. 验收要求

视觉或交互改动至少验证：

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web test:connections
git diff --check
```

涉及 Studio 画布时，还需要通过浏览器实际检查：

- sidebar 展开/收起。
- project peek 是否显示项目名。
- 节点单击浮层。
- 右键菜单关闭。
- 连接 `+` 按钮和目标节点任意区域释放。
- 自动整理在展开和收起 sidebar 时都避开左上浮层。
