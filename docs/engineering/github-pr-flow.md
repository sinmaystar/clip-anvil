# GitHub PR Flow

本流程给 Codex 使用。用户要求“发布”“提交”“提 PR”“创建 PR”时，按这里执行。

## 原则

- 先查清范围，再 stage。
- 不要 `git add -A`。
- 不要把无关改动、运行产物、浏览器 smoke 产物带进提交。
- detached HEAD 或默认分支上，先创建 `codex/<short-topic>` 分支。
- 提交信息使用 Conventional Commits。
- 默认创建 draft PR。

## 步骤

1. 查清当前范围：

   ```bash
   git status --short --branch
   git diff --stat
   git ls-files --others --exclude-standard
   ```

2. 如果有无关改动，先问用户，不要直接 stage。

3. 如果当前是 detached HEAD 或默认分支，创建分支：

   ```bash
   git switch -c codex/<short-topic>
   ```

4. 显式 stage 目标文件：

   ```bash
   git add <file1> <file2>
   git diff --cached --stat
   ```

5. 提交：

   ```bash
   git commit -m "docs: <summary>"
   ```

6. 推送：

   ```bash
   git push -u origin <branch>
   ```

7. 创建 draft PR。优先用 GitHub App；如果返回 `403 Resource not accessible by integration`，改用：

   ```bash
   gh pr create \
     --draft \
     --base main \
     --head <branch> \
     --title "[codex] <summary>" \
     --body-file /tmp/clip-anvil-pr-body.md
   ```

## 完成汇报

只汇报：

- PR URL
- 分支
- commit
- 验证结果
- 本地状态
