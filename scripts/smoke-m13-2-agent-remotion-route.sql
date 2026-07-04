\set ON_ERROR_STOP on

-- Usage:
--   psql "$DATABASE_URL" -v workspace_id='<workspace uuid>' -f scripts/smoke-m13-2-agent-remotion-route.sql

select
  'timeline_plan' as check_name,
  count(*) as count
from timeline_plan
where workspace_id = :'workspace_id'::uuid
  and template_key = 'remotion_timeline_v1'
  and status in ('rendering', 'rendered', 'completed', 'succeeded');

select
  'seedance_generation_jobs' as check_name,
  count(*) as count
from generation_job
where workspace_id = :'workspace_id'::uuid
  and (
    lower(coalesce(provider, '')) like '%seedance%'
    or lower(coalesce(model_id, '')) like '%seedance%'
    or lower(coalesce(operation_type, '')) like '%seedance%'
  );

select
  'seedream_render_plans' as check_name,
  count(*) as count
from render_plan
where workspace_id = :'workspace_id'::uuid
  and model_prompt_profile = 'seedream_5_image'
  and status in ('submitted', 'completed', 'succeeded');

select
  'audio_render_plans' as check_name,
  count(*) as count
from render_plan
where workspace_id = :'workspace_id'::uuid
  and model_prompt_profile = 'seed_audio_1'
  and status in ('submitted', 'completed', 'succeeded');

select
  tp.id,
  tp.template_key,
  tp.status,
  tp.plan_json::jsonb #>> '{schema}' as schema,
  jsonb_array_length(coalesce(tp.plan_json::jsonb -> 'segments', '[]'::jsonb)) as segment_count,
  tp.artifact_version_id,
  tp.sandbox_job_id
from timeline_plan tp
where tp.workspace_id = :'workspace_id'::uuid
  and tp.template_key = 'remotion_timeline_v1'
order by tp.created_at desc
limit 3;
