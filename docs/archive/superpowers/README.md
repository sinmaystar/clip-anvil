# Archived Superpowers Records

本目录保存已经完成或过期的 Superpowers spec、implementation plan 和 graph 导出。

## 目录

| 目录 | 内容 | 何时阅读 |
|---|---|---|
| [`specs/`](specs/) | 历史设计规格 | 追溯当时为什么这样设计、范围如何确定 |
| [`plans/`](plans/) | 历史实施计划 | 追溯阶段拆解、验收命令和当时的执行约束 |
| [`graphs/`](graphs/) | 历史 Eino graph 导出 | 排查旧 Agent graph 结构或对比演进 |

## 使用规则

- 不把这里的文档当作当前实现口径。
- 当前事实以代码、迁移、sqlc 生成代码和 [`../../engineering/`](../../engineering/) 为准。
- 当前里程碑状态看 [`../../milestones/`](../../milestones/)。
- 归档文档可能保留旧路径、旧阶段名和旧命令；需要复用时先对照当前代码。
