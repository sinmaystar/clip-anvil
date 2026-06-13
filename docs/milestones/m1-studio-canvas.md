# M1 Studio 画布基础 — 交付总结

**状态**：✅ 核心链路已落地
**日期**：2026-06-13
**范围来源**：[M1 Studio 画布基础规格](../superpowers/specs/2026-06-12-m1-studio-canvas-design.md)

## 交付内容

1. **基础设施**：goose 迁移、sqlc 查询生成、JWT 配置扩展、Makefile 迁移/生成命令。
2. **注册登录**：注册、登录、`/me`、JWT 路由守卫、登录/注册页。
3. **Workspace**：项目列表、创建项目弹窗、项目详情入口、后端 Workspace API。
4. **Studio Canvas**：tldraw 自定义 `MediaShape`、右键创建文本节点、节点拖拽位置持久化、Camera 持久化。
5. **节点编辑**：单击节点后在节点下方显示编辑面板，支持标题、Prompt、引用占位和模型选择；标题/Prompt 自动保存。
6. **画布体验**：隐藏 tldraw 原生顶部/底部/右侧工具 UI，保留必要快捷键；支持明亮/暗夜外观切换。

## 当前实现边界

M1 是文本节点画布的最小闭环，不包含完整 Studio DAG：

- 仅支持 `text` 节点。
- 暂无 image/video/audio 节点。
- 暂无 MediaEdge 连线、MediaGroup 分组、完整资源树、右侧属性面板。
- 暂无生成任务、版本管理、对象存储业务流。
- 暂无 WebSocket，当前画布同步是 REST-only。
- 暂无 Agent 模式。

## 验收命令

后端：

```bash
make migrate-up
make sqlc-generate
make server-build
make server-test
```

前端：

```bash
pnpm --version
pnpm --filter @clip-anvil/web build
```

本地 sandbox 环境中 `pnpm` 可能受权限或运行时限制，需要在宿主环境执行；以宿主环境输出为准。

## 构建优化记录

- 已升级到 `@vitejs/plugin-react@6`，消除旧 React Babel 插件在 Vite 8 下的 deprecated option warning。
- 已将 Studio 画布页改为路由级懒加载，首屏 JS chunk 从约 2MB 降到约 331KB。
- Studio 画布 chunk 仍约 1.7MB，这是 tldraw 进入画布页时按需加载的成本；`vite.config.ts` 已按该 lazy chunk 设置 `chunkSizeWarningLimit`，避免把非首屏画布包误报为构建风险。
- Node 25 下仍可能显示 `DEP0205 module.register()`，trace 显示来源是 `@tailwindcss/node@4.3.0`，不是应用代码或 Vite 配置。

## 后续建议

下一阶段建议进入 M1.x Studio 增量，优先顺序：

1. image/video/audio 节点和对应视觉预览。
2. MediaEdge 连线和 DAG 环检测。
3. 完整左侧资源树与资源筛选。
4. 右侧属性面板、模型参数和版本列表。
5. WebSocket 事件流与生成任务状态。
