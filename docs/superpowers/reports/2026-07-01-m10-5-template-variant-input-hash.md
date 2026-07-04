# M10.5 Template Variant Input Hash Report

**日期**：2026-07-01
**状态**：通过

## 运行范围

M10.5 为后续 Variant Factory 铺底：同一素材可以通过不同 template params / variables 生成不同 artifact versions，并且 template input refs 的角色、顺序和 winner 变化会进入 production input hash。

本阶段完成：

- `InputHashFacts` 增加 `input_refs` 紧凑摘要。
- `InputHashFactsForNode` 从 `GenerationIntent.InputRefs` 生成稳定 input ref hash facts。
- input ref hash facts 覆盖：
  - `order_index`
  - `node_id`
  - `kind`
  - `required`
  - `node_type`
  - `current_version_id`
  - `asset_id`
  - `asset_type`
  - `mime`
  - `content_type`
  - `model_role`
  - `input_hash`
- 不把 `StorageURL` 或文本内容直接放入 hash facts，避免环境路径和大文本进入 hash；内容身份仍由 upstream `input_hash` 表示。
- 增加 service 级测试，证明 `intentForNode` 可以从 `media_node.model_provider/model_id/operation_type/model_params` 恢复 template video config 和 variables，用于 stale 重算。

## Hash 语义

Template video 的 artifact version hash 现在覆盖：

- provider / model：`internal_template_video/hyperframes-html`
- operation：`image_to_template_video` 或 `template_to_video`
- params：`template_key`、`duration_sec`、`fps`、`ratio`、`variables`
- upstream winner hash：依赖节点的 `current_version_id` 和 `input_hash`
- input binding facts：输入顺序、素材节点、required、content type、model role、winner version/input hash

这意味着：

- 同一 image winner + 不同 variables 会产生不同 hash。
- 同一 image winner + 不同 `model_role` 会产生不同 hash。
- 相同素材但输入顺序变化会产生不同 hash。
- 上游 winner version/input hash 变化会产生不同 hash。

## TDD 记录

Template params / variables 测试直接通过，说明 M10.2/M10.3 已经把 params 纳入 hash：

```text
TestComputeInputHashChangesWhenTemplateKeyChanges
TestComputeInputHashChangesWhenTemplateVariablesChange
TestComputeInputHashStableForTemplateVariableOrdering
```

Input ref 测试先红：

```text
--- FAIL: TestComputeInputHashChangesWhenTemplateInputRefRoleChanges
    hash did not change after template input ref role changed
--- FAIL: TestComputeInputHashChangesWhenTemplateInputRefOrderChanges
    hash did not change after template input ref order changed
--- FAIL: TestComputeInputHashChangesWhenTemplateInputRefWinnerChanges
    hash did not change after template input ref winner changed
```

实现 `input_refs` hash facts 后转绿：

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run 'TestComputeInputHashChangesWhenTemplate|TestComputeInputHashStableForTemplate'
```

```text
ok  	github.com/sinmaystar/clip-anvil/internal/production	0.947s
```

## 验证命令

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run 'TestComputeInputHash|TestIntentForNodeRestoresTemplateVideoConfig'
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production ./internal/agent/worker
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web... lint
git diff --check
```

## 验证结果

```text
ok  	github.com/sinmaystar/clip-anvil/internal/production	1.035s
```

```text
ok  	github.com/sinmaystar/clip-anvil/internal/production	0.605s
ok  	github.com/sinmaystar/clip-anvil/internal/agent/worker	(cached)
```

```text
GOCACHE=/private/tmp/clipanvil-go-build make server-test
cd apps/server && go test ./...
ok  	github.com/sinmaystar/clip-anvil/cmd/server	(cached)
ok  	github.com/sinmaystar/clip-anvil/internal/agent/producer	(cached)
ok  	github.com/sinmaystar/clip-anvil/internal/agent/renderplan	(cached)
ok  	github.com/sinmaystar/clip-anvil/internal/agent/reviewer	(cached)
ok  	github.com/sinmaystar/clip-anvil/internal/agent/tools	(cached)
ok  	github.com/sinmaystar/clip-anvil/internal/agent/worker	(cached)
ok  	github.com/sinmaystar/clip-anvil/internal/production	(cached)
ok  	github.com/sinmaystar/clip-anvil/internal/sandbox	(cached)
ok  	github.com/sinmaystar/clip-anvil/internal/templatevideo	(cached)
```

```text
pnpm --filter @clip-anvil/web... build
Unsupported engine: wanted: {"node":">=26 <27"} (current: {"node":"v24.14.0","pnpm":"11.7.0"})
✓ built in 409ms
```

```text
pnpm --filter @clip-anvil/web... lint
Unsupported engine: wanted: {"node":">=26 <27"} (current: {"node":"v24.14.0","pnpm":"11.7.0"})
$ eslint .
```

前端 build/lint 均 exit 0。Node 24 engine warning 是当前 shell 环境已知现象，repo 目标仍是 Node 26。

## 结论

M10.5 gate 通过。Template video 已经具备 Variant Factory 所需的基础 hash 语义：template key、variables、input refs 和 upstream winner 都会影响 artifact version identity。下一阶段可以在此基础上做批量变量生成、模板选择器或预算 UI，而不需要重新设计 production versioning。
