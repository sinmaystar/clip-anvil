# M12 Dynamic Remotion Route E2E

## Purpose

Verify Agent mode keeps dynamic storyboard planning while routing no-Seedance shot videos to Remotion `motion_shot_video`, then composes a 30s+ final ad with voiceover and BGM.

## Required Runtime

- Production provider mode must be real for Seedream and Volcengine audio.
- Seedance video must not be used.
- Remotion motion shot provider must be available through the sandbox.
- Use `/Users/wanwan/Desktop/box.png` as the product input.

## Browser Prompt

用这张图生成一个 30 秒以上的悦行行李箱口播广告。不要调用 Seedance；图片可以用 Seedream；旁白和 BGM 用火山；视频用 Remotion 图片动效；需要 Agent 根据商品自动决定分镜数量和结构，多分镜、转场、字幕和最终成片都要完成。

## DB Evidence

Run the SQL audit from the active dev database:

```sql
select client_key, title, duration_sec, status
from shot
where workspace_id = $1 and archived_at is null
order by sort_order;

select semantic_key, model_prompt_profile, operation, target_phase
from render_plan
where workspace_id = $1
order by created_at;

select semantic_key, provider, model_id, operation, status
from generation_job
where workspace_id = $1
order by created_at;

select semantic_key, artifact_kind, mime_type, status
from artifact_version
where workspace_id = $1
order by created_at;
```

Expected:

- At least 5 shots.
- `render_plan` has at least 5 `motion_shot_video` rows for `shot_video`.
- No `generation_job` has `model_id` containing `seedance`.
- Final video artifact exists.

## Media Evidence

Download the final signed URL and run:

```bash
ffprobe -v error -show_entries format=duration -show_streams -of json /path/to/final.mp4
```

Expected:

- `format.duration` is at least 30.
- A video stream exists.
- An audio stream exists.
