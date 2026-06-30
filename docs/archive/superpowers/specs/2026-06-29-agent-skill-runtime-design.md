# Agent Skill Runtime 与 ClipAnvil 本地化 Skill 方案设计

**状态**：待评审
**日期**：2026-06-29
**适用范围**：ClipAnvil Agent 模式，Producer / Craftsman / Reviewer / Composer，Eino native tool loop，OpenMontage 能力本地化

## 结论

ClipAnvil 应该引入一个本地 `Agent Skill Runtime`，但不要照搬 OpenMontage 的文件系统自由读取模式，也不要把所有视频生产知识继续塞进四个 Agent 的 system prompt。

推荐第一版方案：

```text
启动时扫描本地 skill library
  -> 解析每个 SKILL.md 的 YAML frontmatter
  -> 每个角色 system prompt 只注入 role-scoped name + description
  -> 四个 Agent 都获得 load_agent_skill 原生工具
  -> Agent 在计划或行动前按 name 加载完整 SKILL.md
  -> skill 正文作为 tool result 回灌模型上下文
  -> 真实能力仍由 DB / model_capability / RenderPlan / production service / sandbox tools 校验
```

这条路的价值不是“多一层提示词文件”，而是把 ClipAnvil 当前最稀缺的制作方法论拆成可按需加载、可版本化、可审查、可灰度的生产知识。它能让 Producer 更像制片人，Craftsman 更像模型导演，Reviewer 更像质量门，Composer 更像后期导演，而不是让四个 Agent 共享一个越来越臃肿的静态 system prompt。

## 官方口径与本方案取舍

参考资料：

- Anthropic Agent Skills overview：https://platform.claude.com/docs/en/agents-and-tools/agent-skills/overview
- Anthropic Skills best practices：https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices
- 用户提供的 Planora skill prompt：`/Users/wanwan/Desktop/seedance_seedream/planora_skill_prompt.txt`
- OpenMontage 本地参考：`/Users/wanwan/Desktop/opensource/OpenMontage/AGENT_GUIDE.md`、`PROJECT_CONTEXT.md`、`skills/INDEX.md`

Anthropic 官方模型可以抽象成三层加载：

| 层级 | 常驻上下文 | 触发时机 | 内容 |
|---|---|---|---|
| Level 1 | 是 | 启动 / 构造 system prompt 时 | `SKILL.md` YAML frontmatter 里的 `name` 和 `description` |
| Level 2 | 否 | 用户任务匹配 description 后 | `SKILL.md` 正文里的工作流、规则、示例 |
| Level 3 | 否 | `SKILL.md` 明确引用时 | 附加 markdown、脚本、模板、参考资料 |

Planora prompt 的关键约束也应吸收：

- Skill 是模块化能力，目录里必须有 `SKILL.md`。
- Agent 在 Think & Plan 阶段识别任务与 skill 的匹配关系。
- 一旦匹配，必须在执行相关动作前读取 skill。
- 如果读 skill 后发现原计划不符合 skill，应更新计划。
- 已加载的 skill 文档是该类工作流的优先操作指南。

ClipAnvil 的取舍：

- 不给模型暴露任意 `file` / `bash` 读取 skill。ClipAnvil 服务端 Agent 不应该拿到通用文件系统能力。
- 使用现有 Eino native tool loop 暴露 `load_agent_skill`，让 skill 加载成为普通 tool call / tool result，可审计、可限权、可追踪。
- system prompt 只注入每个角色可见的 `name` 和 `description`，不注入 skill 正文。
- Skill 正文是工作方法，不是真实能力声明。真实可用模型、参数、生产状态必须继续以 DB、provider adapter、PromptCompiler、RenderPlan validation 和 sandbox 结果为准。

## 第一性原理

### Skill 解决的是“如何做”，不是“能不能做”

Agent 常犯的错不是不知道工具名，而是不知道该如何把业务目标拆成高质量制作步骤。Skill 应承载：

- 商业短视频的策划顺序。
- 参考视频和产品素材如何转成 brief / storyboard / AudioPlan。
- Seedream / Seedance 的提示词结构。
- 什么时候该请求用户确认。
- 什么时候该派 Reviewer，而不是盲目继续生成。
- Composer 如何在素材缺失、音频时长不匹配、平台规格变化时做判断。

Skill 不应承载：

- “模型一定支持某参数”这类易漂移事实。
- 当前 workspace 真实 production 状态。
- 任意可执行脚本的隐式权限。
- 覆盖 DB 事实源的创作事实。

### 渐进加载的核心收益是上下文预算和角色专注

当前 Producer system prompt 已经承载大量业务规则；随着音频、成片、评审、参考视频、模型提示词继续进入，静态 prompt 会越来越难维护。Skill runtime 让系统 prompt 回到“角色边界 + 工具协议 + skill 索引”，把细分方法论按需加载。

### Skill 必须按角色分发

OpenMontage 的 stage director skill 很强，但它默认是一个通用 agent 读完 manifest 后顺序执行。ClipAnvil 已经有四个职责明确的 Agent：

- Producer 是 workspace 级创作状态 owner。
- Craftsman 只把创意事实翻译成 RenderPlan。
- Reviewer 只提交 review / issue / retry recommendation。
- Composer 只负责 final_output 的时间线和成片渲染。

因此 Skill 不能做成一个全局池让所有角色都读同一套长文档。每个 skill 都必须声明 role scope，system prompt 也只展示当前角色的可用 skill。

### OpenMontage 的杀手锏是“生产知识外置”，不是 Python 工具数量

OpenMontage 对 ClipAnvil 最值得借鉴的不是 Remotion / HyperFrames / provider 列表，而是：

- Layer 1 工具能力登记。
- Layer 2 项目自己的制作方法。
- Layer 3 provider / runtime 技术知识。
- 每个 pipeline stage 都有 director skill。
- 每个重要阶段都有 checkpoint / reviewer / quality gate。
- Agent 在动作前读取对应 skill，而不是凭临场提示词 improvisation。

ClipAnvil 已有更强的事实源、画布投影、生产服务、Eino checkpoint 和多 Agent 分工。最该借鉴的是 OpenMontage 的“知识装载结构”，而不是照搬它的 orchestration 形态。

## 范围

范围内：

- 定义 ClipAnvil 本地 skill 文件格式。
- 定义 skill registry、frontmatter parser、system prompt 注入方式。
- 定义 `load_agent_skill` native tool。
- 定义 role-scoped skill catalog。
- 定义初始 OpenMontage 本地化 skill pack。
- 定义安全、审计、测试和验收标准。

范围外：

- 让用户在 UI 里创建 skill。
- 从远程市场安装 skill。
- 让模型读取任意本地文件。
- 让 skill 执行任意脚本。
- 把 OpenMontage 的 pipeline manifest 系统整体搬进 ClipAnvil。
- 引入新的 render runtime，例如 HyperFrames / Remotion，除非后续 Composer 能力单独立项。
- 用 skill 替代 model capability、PromptCompiler、RenderPlan validation 或 provider adapter。

## Skill 文件格式

第一版使用文件内嵌库，随服务端二进制发布：

```text
apps/server/internal/agent/skills/
  registry.go
  prompt_block.go
  load_tool.go
  library/
    commerce-ad-producer/
      SKILL.md
    commerce-ad-reference-video/
      SKILL.md
    seedance-renderplan-craftsman/
      SKILL.md
    seedream-renderplan-craftsman/
      SKILL.md
    reviewer-quality-gate/
      SKILL.md
    composer-timeline-director/
      SKILL.md
```

每个 skill 必须包含 `SKILL.md`。`name` 和 `description` 跟随 Anthropic 官方约束，是唯一注入 system prompt 的必需字段；ClipAnvil 可扩展字段用于服务端过滤和审计，但默认不进入 system prompt。

```markdown
---
name: commerce-ad-producer
description: Use when Producer is turning a user marketing-video request, product material, or reference video into CreativeBrief, ProjectMemory, Storyboard, AudioPlan, dispatch decisions, and HITL checkpoints.
role_scope:
  - producer
task_types:
  - producer_turn
  - decision_resume
domain:
  - commerce_ad
tools:
  - read_project_context
  - upsert_project_brief
  - update_project_memory
  - upsert_storyboard
  - upsert_audio_plan
  - dispatch_craftsman
  - request_user_decision
source:
  kind: openmontage-adapted
  refs:
    - AGENT_GUIDE.md
    - skills/pipelines/explainer/proposal-director.md
version: 0.1.0
---

# Commerce Ad Producer

## Use When

...

## Operating Protocol

...

## Quality Bar

...

## Tool Notes

...
```

字段规则：

| 字段 | 必需 | 用途 |
|---|---|---|
| `name` | 是 | 稳定调用名。第一版使用 lowercase letters / numbers / hyphen，最长 64。 |
| `description` | 是 | 说明做什么、什么时候用。只要这一段写得差，skill 就不会被正确触发。 |
| `role_scope` | 是 | `producer`、`craftsman`、`reviewer`、`composer`。用于系统提示词和工具权限过滤。 |
| `task_types` | 否 | 限制 `producer_turn`、`decision_resume`、`craftsman_turn`、`reviewer_turn`、`composer_turn` 等 `agent_task.task_type`。 |
| `domain` | 否 | `commerce_ad`、`reference_video`、`audio_plan`、`final_video` 等业务标签。 |
| `tools` | 否 | 该 skill 允许或期望使用的 ClipAnvil native tools，只作为提示和审计线索，不直接授权。 |
| `source` | 否 | 改造来源，例如 OpenMontage 文件或官方模型文档。 |
| `version` | 是 | skill 语义版本。tool result 必须返回版本和 hash。 |

禁止事项：

- `SKILL.md` 不得要求 Agent 调用不存在的工具。
- `SKILL.md` 不得声称绕过 Producer / Craftsman / Reviewer / Composer 边界。
- `SKILL.md` 不得引用服务端工作目录之外的文件。
- 第一版不允许 `scripts/` 执行。可以保留目录约定，但 `load_agent_skill` 不加载也不执行脚本。

## System Prompt 设计

四个 Agent 的 system prompt 里新增统一的 `Skills Library` 区块。该区块只包含当前角色可见 skill 的 `name` 和 `description`，加上使用协议。

模板：

```text
## Skills Library

Agent Skills are modular production instructions that extend this role's operating knowledge.
Each skill is represented by a local directory with a SKILL.md file.
Only skill metadata is visible here; full instructions must be loaded with load_agent_skill.

Skill Utilization Protocol:

1. During Think & Plan, identify whether the task matches any available skill description.
2. If a relevant skill exists, call load_agent_skill(name=...) before executing related tools or finalizing the plan.
3. If the loaded skill changes the plan, update the plan before acting.
4. Follow loaded skill instructions unless they conflict with ClipAnvil system rules, role boundaries, tool schemas, DB facts, or user instructions.
5. Never invent skills. Use only the names listed below.

Available Skills:

- commerce-ad-producer: Use when Producer is turning...
- reference-video-analysis-producer: Use when Producer needs...
```

中文实现可以保留英文 `name` 和英文 / 中文混合 description。推荐 description 用英文，因为当前主力模型和工具 schema 已经大量英文命名；正文可以中文为主，关键 schema / tool 名保留英文。

角色可见性：

| 角色 | system prompt 可见内容 |
|---|---|
| Producer | Producer scope skill 的 name / description |
| Craftsman | Craftsman scope skill 的 name / description |
| Reviewer | Reviewer scope skill 的 name / description |
| Composer | Composer scope skill 的 name / description |

不要在 Producer prompt 里塞 Craftsman 的 Seedance 详细 prompt 规则。Producer 只需要知道“遇到 shot video production，需要派 Craftsman，并可提醒其加载对应 skill”。

## Native Tool 设计

### `load_agent_skill`

所有四个 Agent 都注册该工具，但按 role / task / workspace runtime 上下文过滤。

输入：

```json
{
  "name": "commerce-ad-producer",
  "reason": "Need to turn user product brief and reference video into storyboard and dispatch plan.",
  "sections": ["Operating Protocol", "Quality Bar"]
}
```

参数：

| 字段 | 必需 | 说明 |
|---|---|---|
| `name` | 是 | 必须匹配当前角色可见 skill name。 |
| `reason` | 是 | Agent 为什么要加载该 skill。用于审计和调试。 |
| `sections` | 否 | 第一版可忽略或只做标题过滤。若为空，返回完整 `SKILL.md` 正文。 |

输出：

```json
{
  "name": "commerce-ad-producer",
  "version": "0.1.0",
  "source_hash": "sha256:...",
  "role_scope": ["producer"],
  "loaded_at": "2026-06-29T00:00:00Z",
  "content": "# Commerce Ad Producer\n\n...",
  "warnings": []
}
```

错误：

| 情况 | 行为 |
|---|---|
| skill 不存在 | 返回 typed tool error，提示可用 name 列表。 |
| role 不允许 | 返回 typed tool error，不泄露正文。 |
| task type 不允许 | 返回 typed tool error，说明当前 task 不适用。 |
| frontmatter 无效 | 服务启动时失败；不要运行到生产时才发现。 |
| 正文超过 token 上限 | 返回错误或要求用 `sections` 加载。第一版建议单个 skill 正文控制在 4k tokens 内。 |

审计：

- tool call / result 已会进入 `agent_message` trace。
- `source_hash` 和 `version` 必须进入 tool result。
- 后续可增加 `agent_skill_load` 表，但第一版不需要先加表。先复用现有 tool trace，避免过早数据库扩展。

### 是否需要 `list_agent_skills`

第一版可以不做，因为 system prompt 已经列出可见 skill。若实现成本很低，可增加只读调试工具：

```json
{
  "role": "producer",
  "skills": [
    {"name": "commerce-ad-producer", "description": "...", "version": "0.1.0"}
  ]
}
```

但 `list_agent_skills` 不是核心能力。核心是 `load_agent_skill(name)`。

## Runtime 集成点

当前代码事实：

- 四个主图是 `producer_turn`、`craftsman_render_plan`、`reviewer_gate`、`composer_timeline`。
- Producer / Craftsman / Reviewer / Composer 都使用 Eino `compose.ToolsNode` 和 `NativeRegistry`。
- `apps/server/cmd/server/main.go` 已分别注册四个角色的 native tool registry。
- system prompt 分散在 `apps/server/internal/agent/{producer,craftsman,reviewer,composer}/system_prompt.go`。

第一版实现建议：

1. 新增 `apps/server/internal/agent/skills` 包。
2. 使用 Go embed 嵌入 `library/**/SKILL.md`。
3. `Registry` 在服务启动时解析并校验所有 frontmatter。
4. 提供 `PromptBlock(role string) string`，生成当前角色的 `Skills Library` prompt block。
5. 在四个 system prompt 构造处追加 `PromptBlock(role)`。
6. 新增 `LoadAgentSkillNativeTool`，实现现有 `agenttools.NativeTool` 接口。
7. 在 `main.go` 的 Producer / Craftsman / Reviewer / Composer native registry 中注册同一个 skill load tool，但传入不同 role context 或由 middleware 注入 role。
8. tool handler 从 native tool runtime context 获取 workspace / thread / task / role，校验 role_scope 和 task_types。
9. tool result 作为普通 Eino tool result 回灌模型。

测试建议：

- `TestSkillRegistryParsesFrontmatter`：解析 `name`、`description`、`role_scope`、`version`。
- `TestSkillPromptBlockIncludesOnlyMetadata`：system prompt 只包含 name / description，不包含正文标题或操作步骤。
- `TestSkillPromptBlockFiltersByRole`：Producer 看不到 Composer-only skill。
- `TestLoadAgentSkillReturnsBodyAndHash`：工具按 name 返回正文、version、hash。
- `TestLoadAgentSkillRejectsWrongRole`：Reviewer 无法加载 Craftsman-only skill。
- 四个 Agent 的 prompt 单测各增加一个断言：包含 `Skills Library` 和 `load_agent_skill` 协议。

## ClipAnvil 本地化 Skill Pack

第一版不要迁移 OpenMontage 全部 skill。先围绕 ClipAnvil 当前商业可行性最高的路径：电商 / 营销短视频，从用户 brief / 素材 / 参考视频到分镜视频、旁白、BGM、成片、评审。

### Producer Skills

| Skill | 触发 | 借鉴来源 | ClipAnvil 本地化重点 |
|---|---|---|---|
| `commerce-ad-producer` | 用户要做产品营销短视频、广告片、种草视频 | OpenMontage proposal / executive producer / checkpoint protocol | 生成 CreativeBrief、ProjectMemory、Storyboard、AudioPlan；决定何时 HITL；派 Craftsman / Reviewer / Composer |
| `reference-video-analysis-producer` | 用户上传参考视频或要求“照这个风格” | OpenMontage video understanding / b-roll / cinematic director | 把参考视频拆成可复用的节奏、镜头语言、卖点结构，而不是逐帧抄袭 |
| `audio-plan-producer` | 项目需要旁白、BGM、最终混音 | OpenMontage sound design / edit director | 维护全片 AudioPlan，确认旁白脚本和 BGM 方向，派发 audio RenderPlan |
| `hitl-checkpoint-producer` | 关键创作或成本决策前 | OpenMontage checkpoint protocol | 明确哪些决策必须问用户，如何生成 decision card |

Producer skill 不应该包含 Seedance 细节提示词。它应该写清楚什么时候派 Craftsman、什么时候等待用户、什么时候读 Reviewer 结果。

### Craftsman Skills

| Skill | 触发 | 借鉴来源 | ClipAnvil 本地化重点 |
|---|---|---|---|
| `seedream-renderplan-craftsman` | 创建 reference image / preview image / image variant RenderPlan | OpenMontage image gen usage / provider prompting | 把 brief、key element、reference pack 转成结构化 image RenderPlan |
| `seedance-renderplan-craftsman` | 创建 shot video RenderPlan | OpenMontage video-gen prompting / Seedance prompting | 主体、动作、场景、空间、镜头、节奏、参考图绑定、首帧 / 首尾帧策略 |
| `audio-renderplan-craftsman` | 创建 voiceover_audio / bgm_audio RenderPlan | OpenMontage sound design / music gen | 把 AudioPlan 翻译成 audio_generation RenderPlan |
| `renderplan-repair-craftsman` | Reviewer 指出问题后修订 RenderPlan | OpenMontage reviewer send-back | 读取 review_record / artifact_issue，生成最小修复，不重写已通过部分 |

Craftsman skill 是短期最能提升质量的区域。它可以把 Seedream / Seedance 的模型工程规则从静态 prompt 里拆出来，并与 RenderPlan schema 对齐。

### Reviewer Skills

| Skill | 触发 | 借鉴来源 | ClipAnvil 本地化重点 |
|---|---|---|---|
| `reviewer-quality-gate` | 任何 preview image / shot video / final video review | OpenMontage meta reviewer | 用 ClipAnvil 10 轴 rubric 写 review_record 和 artifact_issue |
| `commerce-delivery-promise-reviewer` | 营销片交付承诺需要验证 | OpenMontage delivery promise / proposal director | 判断视频是否兑现卖点、CTA、目标平台和用户 brief |
| `reference-consistency-reviewer` | 有参考图、参考视频、关键元素一致性要求 | OpenMontage style / character consistency | 判断人物、产品、品牌、镜头语言是否漂移 |
| `final-video-audio-reviewer` | final video review | OpenMontage sound design / compose director | 检查旁白、BGM、ducking、字幕安全区、节奏匹配 |

Reviewer skill 必须强调边界：只提交 review result，不直接触发重跑、不直接选择 winner、不改 ProjectMemory。

### Composer Skills

| Skill | 触发 | 借鉴来源 | ClipAnvil 本地化重点 |
|---|---|---|---|
| `composer-timeline-director` | 创建 TimelinePlan | OpenMontage edit / compose director | shot winner 顺序、duration、transition、subtitle、安全区 |
| `ffmpeg-audio-mix-composer` | 混音、ducking、fade、音频对齐 | OpenMontage FFmpeg / sound design | stage/probe 媒体，生成稳定 ffmpeg 策略 |
| `platform-export-composer` | 输出抖音 / TikTok / 小红书 / YouTube Shorts 等规格 | OpenMontage media profiles / publish director | aspect ratio、码率、字幕位置、片头片尾容错 |
| `composer-blocker-escalation` | 缺素材、音频时长不匹配、probe 失败 | OpenMontage blocker protocol | 返回 blocked 给 Producer，明确下一步选项 |

Composer skill 不应该引入 OpenMontage 的 Remotion / HyperFrames 决策规则，除非 ClipAnvil 后续真的接入这些 runtime。第一版只围绕已有 sandbox ffmpeg 和 TimelinePlan。

## OpenMontage 到 ClipAnvil 的概念映射

| OpenMontage 概念 | ClipAnvil 对应 | 本地化方式 |
|---|---|---|
| `idea` / `brief` | `creative_brief` | Producer skill 指导 brief 提取和更新 |
| `script` | `ProjectMemory` + `Storyboard` + `AudioPlan` | 不引入单独 script artifact；旁白进入 AudioPlan |
| `scene_plan` | `scene` / `shot` / `storyboard` | Producer 维护，Craftsman 只读取 |
| `asset_manifest` | `media_node` / `artifact_version` / reference pack | 由 production 和 canvas DB 作为事实源 |
| `edit_decisions` | `timeline_plan` | Composer 创建和更新 |
| `render_report` | `generation_job` / `timeline_plan` / agent events | 不重复造报告表 |
| `publish_log` | 后续 publish 模块 | 第一版不做 |
| pipeline manifest | Producer task + skill catalog + Agent tools | 不搬 manifest 系统，先用 role-scoped skill |
| stage director skill | role skill | 拆到 Producer / Craftsman / Reviewer / Composer |
| checkpoint protocol | `request_user_decision` + Eino checkpoint | Producer skill 明确触发点 |
| meta reviewer | Reviewer Agent + reviewer skills | 写入 `review_record` / `artifact_issue` |
| tool registry | `NativeRegistry` + `model_capability` + provider config | 不引入 Python registry |
| cost tracker | 后续 billing / budget | 第一版只在 skill 里要求生成前说明重要选择 |

## 对抗性审查

### 风险 1：Skill 变成新的 prompt 垃圾桶

如果每个 skill 都写成 200 行泛泛原则，Agent 只会多一次工具调用，不会更专业。

缓解：

- 单个 skill 只服务一个角色和一个明确场景。
- skill 正文控制在 2k 到 4k tokens。
- 每个 skill 必须包含 `Use When`、`Do`、`Do Not`、`Tool Protocol`、`Quality Bar`。
- 没有可执行差异的内容不进 skill。

### 风险 2：Skill 与真实工具能力冲突

OpenMontage 里有大量 ClipAnvil 当前没有的工具和 runtime。直接迁移会让 Agent 幻觉调用 Remotion、HyperFrames、stock provider、cost tracker。

缓解：

- `tools` frontmatter 只允许列出现有 native tool。
- skill 正文禁止要求不存在的工具。
- 迁移 OpenMontage skill 时先做能力裁剪，只保留 ClipAnvil 当前生产链路能执行的部分。
- 真实能力由 RenderPlan validation / provider adapter / sandbox 返回结果确认。

### 风险 3：Role boundary 被 skill 破坏

如果 Producer skill 写了“直接写 ffmpeg 命令”，或者 Reviewer skill 写了“自动触发重跑”，系统会回到单 Agent 大脑。

缓解：

- `role_scope` 是硬过滤。
- 每个角色有专属 skill 模板。
- system prompt 明确：skill 不能覆盖角色边界。
- Reviewer / Craftsman / Composer 的 tool whitelist 本身继续限制越权。

### 风险 4：Skill 加载循环和 token 浪费

模型可能每轮反复加载同一个 skill。

缓解：

- `load_agent_skill` 可以在同一 task 内返回 `already_loaded=true` 和短摘要，或直接返回缓存正文。
- system prompt 要求每个相关 skill 每个 task 通常只加载一次。
- tool trace 可用于发现高频无效加载。

### 风险 5：Skill 陈旧

模型 capability 或 RenderPlan schema 改了，skill 没同步会误导 Agent。

缓解：

- `version` 和 `source_hash` 每次加载返回。
- 相关 schema / tool 变更时测试断言 skill 里的 tool 名仍存在。
- 每个 skill 的 `Tool Protocol` 尽量引用稳定业务意图，不硬编码易漂移 provider 参数。

### 风险 6：安全与供应链

Anthropic 官方也强调 skill 具备指令和代码风险。OpenMontage skill 是外部参考，不能不审查直接进入生产。

缓解：

- 第一版只允许 repo 内置 skill。
- 不支持远程下载、用户上传、脚本执行。
- 每个 OpenMontage 改造 skill 都写 `source`，但正文必须重写为 ClipAnvil 本地语义。
- 不把外部 skill 的未知路径、网络调用、执行命令迁入。

## 分阶段计划

### M8.1：Skill Runtime 基座

目标：让四个 Agent 能看到可用 skill 元数据，并按 name 加载正文。

交付：

- `agent/skills` registry。
- frontmatter parser 和校验。
- role-scoped prompt block。
- `load_agent_skill` native tool。
- 四个角色注册该工具。
- 单测覆盖解析、prompt metadata、role filter、tool load。

验收：

- 默认 system prompt 只包含 skill name / description。
- 调用 `load_agent_skill(name)` 后，skill 正文以 tool result 进入同一轮上下文。
- 错误 role 不能读取正文。

### M8.2：电商营销短视频 Skill Pack v0

目标：把 OpenMontage 最有价值的 production knowledge 本地化到 ClipAnvil 的商业短视频主路径。

交付：

- Producer：`commerce-ad-producer`、`reference-video-analysis-producer`、`audio-plan-producer`。
- Craftsman：`seedream-renderplan-craftsman`、`seedance-renderplan-craftsman`、`audio-renderplan-craftsman`。
- Reviewer：`reviewer-quality-gate`、`commerce-delivery-promise-reviewer`、`final-video-audio-reviewer`。
- Composer：`composer-timeline-director`、`ffmpeg-audio-mix-composer`。

验收：

- 每个 skill 都只引用该角色真实可用工具。
- 每个 skill 都能解释自己何时加载、加载后要怎样改变行动。
- 通过单测验证所有 frontmatter 合法。

### M8.3：Skill 使用质量闭环

目标：观察 skill 是否真的改善 Agent 行为。

交付：

- 在 tool trace 中保留 skill load reason、version、hash。
- smoke case：同一营销视频 brief，比较有 skill / 无 skill 的 RenderPlan 完整度、Reviewer issue 命中率、Composer blocked 表达质量。
- 增加失败样例：skill 不存在、role 不允许、正文超长。

验收：

- Craftsman 生成的 RenderPlan 更稳定包含 subject bindings、reference strategy、operation、output type、model prompt profile。
- Reviewer issue 更可执行，Producer 能根据 review 派发修订。
- Composer 对缺素材和音频不匹配能明确 blocked，而不是生成坏 ffmpeg。

### M8.4：后续增强

可选方向：

- `load_agent_skill_resource`：只允许加载 skill 目录内白名单 markdown 资源。
- 管理端查看 skill catalog 和版本。
- workspace / tenant 级 skill 开关。
- skill 使用统计和质量评估。
- 将模型 prompt profile 与 skill 互相校验。

暂不建议：

- 用户上传任意 skill。
- skill 脚本执行。
- 远程 skill marketplace。
- 把 OpenMontage pipeline manifest 搬进数据库。

## 推荐的第一批 SKILL.md 模板

```markdown
---
name: seedance-renderplan-craftsman
description: Use when Craftsman must create or repair a shot video RenderPlan for Seedance-style video generation from storyboard, key elements, reference images, or reviewer feedback.
role_scope:
  - craftsman
task_types:
  - craftsman_turn
domain:
  - commerce_ad
  - shot_video
tools:
  - read_project_memory
  - upsert_render_plan
source:
  kind: openmontage-adapted
  refs:
    - skills/creative/video-gen-prompting.md
    - skills/creative/prompting/seedance-prompting.md
version: 0.1.0
---

# Seedance RenderPlan Craftsman

## Use When

Load this skill before creating or repairing a shot video RenderPlan.

## Do

- Translate the shot goal into subject, action, scene, spatial composition, camera movement, and temporal order.
- Preserve key elements from ProjectMemory and explicit reference bindings.
- Prefer first-frame or first/last-frame strategy when the shot depends on a generated preview image.
- Keep model-specific prompt details inside RenderPlan prompt parts and params.

## Do Not

- Do not submit generation jobs directly.
- Do not change Storyboard or AudioPlan.
- Do not invent unavailable models or provider parameters.
- Do not ignore Reviewer issues when repairing.

## Tool Protocol

1. Call read_project_memory if task context is insufficient.
2. Build a RenderPlan with operation, output_type, prompt_parts, input_node_refs, params, and risk notes.
3. Call upsert_render_plan once the plan is internally consistent.

## Quality Bar

- The RenderPlan can be validated without relying on hidden assumptions.
- The prompt is concrete enough for video generation, not a vague marketing sentence.
- Reference images and key elements are mapped to explicit roles.
```

## 验收标准

第一版完成后，必须满足：

- 四个 Agent system prompt 都包含 `Skills Library` 协议。
- system prompt 只包含 role 可见 skill 的 `name` 和 `description`。
- `load_agent_skill` 在四个 Agent 的 native registry 中可用。
- `load_agent_skill` 返回完整正文、version、source_hash，并通过 role_scope 校验。
- 所有内置 skill frontmatter 在服务启动或测试中被校验。
- 所有 skill 只引用当前角色真实可用 native tools。
- 文档和测试明确：skill 不是真实能力源，真实能力仍由 ClipAnvil production substrate 校验。

最小验证命令：

```bash
make server-test
git diff --check
```

只写本 spec 时，运行：

```bash
git diff --check
```

## 设计判断

这条路线值得做，而且应该尽快做基座，但要克制地做。

从商业可行性看，ClipAnvil 现阶段的差距不是“还缺一个更聪明的总控 Agent”，而是缺可复用的、能稳定提高视频制作质量的领域方法论。Skill runtime 正好解决这个问题：它让制作知识可以像代码一样版本化，又不会吞掉所有上下文。

从对抗性角度看，最大的失败模式是把 OpenMontage 全量照搬，导致 ClipAnvil 多一堆无法执行的仪式和工具幻觉。正确的切口是先做 `load_agent_skill` 基座，再围绕电商营销短视频写少量高密度本地 skill。等这些 skill 能真实改善 RenderPlan、review 和 final composition，再考虑资源文件、管理端和更复杂 pipeline。

一句话：OpenMontage 的降维打击在于“把制作知识系统化地外置并强制加载”；ClipAnvil 的优势在于“已有真实 production substrate 和多 Agent 边界”。把两者结合，最优形态不是复制 OpenMontage，而是让 ClipAnvil 的四个 Agent 各自拥有可渐进加载的专业工作手册。
