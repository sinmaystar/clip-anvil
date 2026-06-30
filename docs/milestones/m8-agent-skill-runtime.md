# M8 Agent Skill Runtime 与 OpenMontage 本地化 — 里程碑

**状态**：M8.1、M8.2、M8.3、M8.4 与端到端全链路测试已完成（2026-06-29）
**日期**：2026-06-29
**目标**：为 ClipAnvil 的 Producer / Craftsman / Reviewer / Composer 引入渐进式 Skill 加载能力，把 OpenMontage 最有价值的生产知识本地化为 role-scoped skill，并用质量闭环验证它确实改善商业短视频生产效果。

参考文档：

- [Agent Skill Runtime 与 ClipAnvil 本地化 Skill 方案设计](../archive/superpowers/specs/2026-06-29-agent-skill-runtime-design.md)
- [Agent MultiAgent 架构现状](../engineering/agent-multiagent-architecture.md)
- [M7 Agent AudioPlan 与 Composer 音频成片](./m7-agent-audio-plan-composer.md)

## Codex Goal 建议

按阶段完成 M8 Agent Skill Runtime。每个阶段必须先写该阶段实施计划，再开发、验证、记录验收结果；上一阶段未通过验收时，不进入下一阶段。

第一版只交付 **repo 内置、只读、不可执行脚本的本地 skill**。不要做远程 skill 市场、用户上传 skill、skill 脚本执行、管理端 UI，也不要把 OpenMontage pipeline manifest 整体搬进 ClipAnvil。

## 已确认口径

- Skill 解决“如何做”，不解决“真实能不能做”。真实能力仍由 DB、`model_capability`、RenderPlan validation、provider adapter、production service 和 sandbox 结果校验。
- system prompt 只常驻当前角色可见 skill 的 `name` 和 `description`，不注入 skill 正文。
- 完整 `SKILL.md` 正文必须通过 `load_agent_skill(name)` native tool 渐进加载，并作为 tool result 回灌模型。
- `load_agent_skill` 必须按 role / task type 做权限过滤，不能给模型暴露任意文件读取能力。
- OpenMontage 的借鉴重点是“生产知识外置并强制加载”，不是照搬它的 Python tool registry、pipeline manifest 或 Remotion / HyperFrames runtime。
- Producer / Craftsman / Reviewer / Composer 的边界不能被 skill 改写。skill 只能指导当前角色更好地使用其已有 native tools。

## 阶段里程碑

| 阶段 | 里程碑 | 可验收标准 |
|---|---|---|
| M8.1 Skill Runtime 基座 | 服务端能扫描内置 skill library、解析 YAML frontmatter、生成 role-scoped `Skills Library` prompt block，并提供 `load_agent_skill` native tool。 | ✅ 已完成；system prompt 只包含 `name` / `description`；四个 Agent 都注册 `load_agent_skill`；正确 role 可加载正文和 `version` / `source_hash`；错误 role / task type 无法读取正文；不新增 DB 表，先复用现有 tool trace；通过 registry、prompt block、tool load、role filter 单测。验证：`GOCACHE=/private/tmp/clipanvil-go-build make server-test`。 |
| M8.2 商业短视频 Skill Pack v0 | 本地化第一批 OpenMontage 生产知识，覆盖电商 / 营销短视频从 brief、参考视频、RenderPlan、review 到成片混音的主路径。 | ✅ 已完成；Producer、Craftsman、Reviewer、Composer 各 4 个高价值 skill；所有 skill 只引用当前角色真实 native tools；每个 skill 都包含 `Use When`、`Do`、`Do Not`、`Tool Protocol`、`Quality Bar`；frontmatter 全部通过测试；默认 prompt 不膨胀。验证：`GOCACHE=/private/tmp/clipanvil-go-build make server-test`。 |
| M8.3 Skill 使用质量闭环 | 用 trace 和 smoke case 验证 skill 对 Agent 生产质量有实际收益。 | ✅ 已完成；tool trace 可看到 skill load reason、version、hash；固定营销视频 brief 已通过 deterministic smoke 对比有 skill / 无 skill；Craftsman 覆盖 RenderPlan audit 维度，Reviewer 覆盖可修复 issue 维度，Composer 覆盖 blocked / finalization 维度。验证：`cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools ./internal/agent/producer -run 'LoadAgentSkill\|SkillTrace' -count=1`、`bash -n scripts/smoke-m8-3-skill-quality-loop.sh`、`./scripts/smoke-m8-3-skill-quality-loop.sh`、`GOCACHE=/private/tmp/clipanvil-go-build make server-test`、`git diff --check`。 |
| M8.4 Skill 资源与治理增强 | 在 M8.1-M8.3 被证明有价值后，补充受控资源加载、role / task enablement checks、统计和一致性检查。 | ✅ 已完成；实现 `load_agent_skill_resource`，只能加载 skill 目录内 `.md` 资源；路径穿越、绝对路径、非 markdown 和缺失资源会被拒绝；可查看 in-process usage stats；skill 引用 tool 名与当前角色 native registry 白名单保持一致；workspace / tenant 持久开关延后到有明确产品需求后再做；仍不支持脚本执行和远程安装。验证：`cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/skills ./internal/agent/tools ./cmd/server -run 'Resource\|ToolReferences\|NativeToolInfos\|LoadAgentSkill' -count=1`、`GOCACHE=/private/tmp/clipanvil-go-build make server-test`、`git diff --check`。 |
| M8.E2E 端到端全链路 | 串起 prompt metadata、`load_agent_skill`、`load_agent_skill_resource`、质量 smoke 和 server regression。 | ✅ 已完成；四个角色都能在默认 prompt 中看到 role-scoped skill metadata，能按 role / task type 渐进加载允许的 `SKILL.md` 正文；Producer 受控 resource 可加载，越权资源路径仍由 M8.4 测试覆盖；质量 smoke 和全量 server regression 通过。验证：`bash -n scripts/smoke-m8-skill-runtime-e2e.sh`、`./scripts/smoke-m8-skill-runtime-e2e.sh`。 |

## 阶段验收建议

### M8.1

✅ 已完成（2026-06-29）。目标是把机制做稳，不追求 skill 数量。

必须交付：

- `apps/server/internal/agent/skills` 包。
- repo 内置 `library/**/SKILL.md`，通过 Go embed 发布。
- frontmatter parser 和启动 / 测试期校验。
- `PromptBlock(role)` 只输出当前角色可见 skill 的 `name` / `description`。
- `load_agent_skill` native tool。
- Producer / Craftsman / Reviewer / Composer system prompt 接入 `Skills Library`。
- 四个角色 native registry 注册 `load_agent_skill`。

验收命令：

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

完成记录：

- 新增 `apps/server/internal/agent/skills` 包，支持内置 `SKILL.md` 解析、role / task type 过滤、metadata-only prompt block 和正文加载 hash。
- 新增四个 starter skill：`commerce-ad-producer`、`seedance-renderplan-craftsman`、`reviewer-quality-gate`、`composer-timeline-director`。
- 新增 `load_agent_skill` native tool，四个 Agent registry 均已注册。
- Producer / Craftsman / Reviewer / Composer system prompt 均接入 `Skills Library`，且不泄露 skill 正文。
- Producer native runtime context 补充 `TaskType`，四个 Agent runtime context 均可用于 task type 过滤。
- 已执行验证：`GOCACHE=/private/tmp/clipanvil-go-build make server-test`。

### M8.2

✅ 已完成（2026-06-29）。目标是先让最有商业价值的短视频路径变专业，不做 OpenMontage 全量迁移。

推荐首批 skill：

| 角色 | Skill |
|---|---|
| Producer | `commerce-ad-producer`、`reference-video-analysis-producer`、`audio-plan-producer`、`hitl-checkpoint-producer` |
| Craftsman | `seedream-renderplan-craftsman`、`seedance-renderplan-craftsman`、`audio-renderplan-craftsman`、`renderplan-repair-craftsman` |
| Reviewer | `reviewer-quality-gate`、`commerce-delivery-promise-reviewer`、`reference-consistency-reviewer`、`final-video-audio-reviewer` |
| Composer | `composer-timeline-director`、`ffmpeg-audio-mix-composer`、`platform-export-composer`、`composer-blocker-escalation` |

验收命令：

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

完成记录：

- Producer 新增 / 完善 4 个 skill：`commerce-ad-producer`、`reference-video-analysis-producer`、`audio-plan-producer`、`hitl-checkpoint-producer`。
- Craftsman 新增 / 完善 4 个 skill：`seedream-renderplan-craftsman`、`seedance-renderplan-craftsman`、`audio-renderplan-craftsman`、`renderplan-repair-craftsman`。
- Reviewer 新增 / 完善 4 个 skill：`reviewer-quality-gate`、`commerce-delivery-promise-reviewer`、`reference-consistency-reviewer`、`final-video-audio-reviewer`。
- Composer 新增 / 完善 4 个 skill：`composer-timeline-director`、`ffmpeg-audio-mix-composer`、`platform-export-composer`、`composer-blocker-escalation`。
- 新增 M8.2 skill pack 测试，断言每个角色 skill 名称、必需章节、工具白名单和 prompt body 不泄露。
- 已执行验证：`GOCACHE=/private/tmp/clipanvil-go-build make server-test`。

### M8.3

✅ 已完成（2026-06-29）。目标是证明 skill 不是“多一层文档”，而是能改善 Agent 行为。

建议 smoke：

1. 准备同一个电商营销短视频 brief，包含产品素材、参考风格和成片要求。
2. 跑一轮关闭 skill 的 Agent fixture 或 mock responder 路径，记录 RenderPlan / review / composer 行为。
3. 跑一轮开启 skill 的路径，记录同样产物。
4. 对比 RenderPlan 是否包含更完整的 subject bindings、reference strategy、operation、output type、model prompt profile 和 risk notes。
5. 对比 Reviewer issue 是否更具体、可修复。
6. 对比 Composer blocked / finalization 文案是否更能指导 Producer 下一步。

验收命令：

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

如果新增 smoke 脚本，需要同时验证：

```bash
bash -n scripts/<m8-smoke-script>.sh
./scripts/<m8-smoke-script>.sh
```

完成记录：

- 加强 `load_agent_skill` 测试，覆盖 `reason` 必填、`source_hash` / `version` 返回、sections warning、错误不泄露正文。
- 新增 Producer native trace 测试，证明 `load_agent_skill` 的 `name`、`reason`、`version`、`source_hash` 会进入 tool trace。
- 新增 `scripts/smoke-m8-3-skill-quality-loop.sh`，用固定电商 brief 做无 skill / 有 skill 的 deterministic 维度覆盖对比。
- 新增 smoke 报告：[M8.3 Skill Quality Loop Smoke Report](../superpowers/reports/2026-06-29-m8-3-skill-quality-loop.md)。
- 已执行验证：`cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools ./internal/agent/producer -run 'LoadAgentSkill\|SkillTrace' -count=1`、`bash -n scripts/smoke-m8-3-skill-quality-loop.sh`、`./scripts/smoke-m8-3-skill-quality-loop.sh`、`GOCACHE=/private/tmp/clipanvil-go-build make server-test`、`git diff --check`。

### M8.4

✅ 已完成（2026-06-29）。目标是治理和扩展，不是提前做平台化。

只有 M8.1-M8.3 证明 skill 有价值后再进入。第一批增强优先级：

1. `load_agent_skill_resource`：受控加载 skill 目录内附加 markdown。
2. skill 使用统计：按 role / skill / task type 汇总加载次数和失败原因。
3. role / task enablement checks：沿用 M8.1 的角色和 task type 过滤作为第一版启用边界。
4. 一致性检查：测试中断言 skill frontmatter `tools` 全部存在于对应 role registry。

workspace / tenant 级持久开关暂缓，不在 M8.4 新增 DB migration 或管理 UI。

验收命令按实际改动范围选择：

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

如果没有前端改动，不需要跑 web build / lint。

完成记录：

- 新增 `load_agent_skill_resource` native tool，四个 Agent registry 均已注册。
- skill registry 支持受控 markdown resource 加载，只能读取 skill 目录内 `.md` 文件。
- resource path 会拒绝空路径、绝对路径、`..` 越界、非 markdown 和缺失文件。
- 新增 in-process usage stats，测试覆盖 skill 正文加载和 resource 加载计数。
- 新增 `commerce-ad-producer/references/checklist.md` 作为第一版受控资源样例。
- 新增 tool-reference 一致性测试，断言内置 skill frontmatter `tools` 不越过对应角色 native registry 白名单。
- 已执行验证：`cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/skills ./internal/agent/tools ./cmd/server -run 'Resource\|ToolReferences\|NativeToolInfos\|LoadAgentSkill' -count=1`、`GOCACHE=/private/tmp/clipanvil-go-build make server-test`、`git diff --check`。

### M8.E2E

✅ 已完成（2026-06-29）。目标是证明 M8 不是单点功能，而是四个 Agent 都可用的完整 skill runtime。

必须覆盖：

- Producer / Craftsman / Reviewer / Composer 的 prompt metadata 可见，但不泄露 skill body。
- 四个 Agent 都能通过 `load_agent_skill` 加载各自允许的 skill 正文。
- `load_agent_skill` 返回 `version` 和 `source_hash`，便于 trace 和回放。
- `load_agent_skill_resource` 能加载受控 markdown resource。
- M8.3 quality smoke 和全量 server regression 与 E2E 一起跑。

验收命令：

```bash
bash -n scripts/smoke-m8-skill-runtime-e2e.sh
./scripts/smoke-m8-skill-runtime-e2e.sh
git diff --check
```

完成记录：

- 新增 `apps/server/internal/agent/tools/skill_runtime_e2e_test.go`，覆盖四角色 prompt metadata、skill body 渐进加载、版本和 hash 输出。
- 新增 resource E2E，覆盖 `commerce-ad-producer/references/checklist.md` 的受控加载。
- 新增 `scripts/smoke-m8-skill-runtime-e2e.sh`，串起 focused E2E、M8.3 quality smoke、`make server-test` 和 diff check。
- 已执行验证：`bash -n scripts/smoke-m8-skill-runtime-e2e.sh`、`./scripts/smoke-m8-skill-runtime-e2e.sh`、`git diff --check`。

## 完成定义

- M8.1-M8.4 全部通过各自验收，且验收结果写入对应阶段总结或 PR 描述。
- 四个 Agent 都能在默认 prompt 中看到当前角色可用 skill 索引。
- 四个 Agent 都能通过 `load_agent_skill` 加载允许范围内的 `SKILL.md` 正文。
- Skill 加载不会绕过 role boundary、tool schema、DB 事实源或 production validation。
- 商业短视频主路径至少有一组可复用 skill，能提升 RenderPlan、Reviewer issue 和 Composer blocked / finalization 的质量。
- OpenMontage 被本地化为 ClipAnvil 的专业工作手册，而不是引入无法执行的外部 runtime 假设。

## 暂不做

- 用户上传 skill。
- 远程 skill marketplace。
- skill 脚本执行。
- 让模型读取任意本地文件。
- 把 OpenMontage pipeline manifest 系统整体迁入 ClipAnvil。
- 管理端 skill 编辑器。
- 复制 OpenMontage 的 Remotion / HyperFrames runtime 选择规则。
- 用 skill 替代 `model_capability`、PromptCompiler、RenderPlan validation 或 provider adapter。
