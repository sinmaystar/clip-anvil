#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPORT_PATH="$ROOT_DIR/docs/superpowers/reports/2026-06-29-m8-3-skill-quality-loop.md"

python3 - "$ROOT_DIR" "$REPORT_PATH" <<'PY'
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
report_path = pathlib.Path(sys.argv[2])
skill_root = root / "apps/server/internal/agent/skills/library"

brief = "做一条 15 秒 9:16 电商短视频，推广银灰色登机箱。风格参考为快节奏机场出发场景，卖点是顺滑万向轮、轻量、商务质感。需要旁白和轻快 BGM，最终用于抖音。"

roles = {
    "Producer": {
        "skills": [
            "commerce-ad-producer",
            "reference-video-analysis-producer",
            "audio-plan-producer",
            "hitl-checkpoint-producer",
        ],
        "baseline": "Plan a commerce video from the brief, create scenes, and ask the user if needed.",
        "dimensions": {
            "durable facts": ["CreativeBrief", "ProjectMemory", "Storyboard", "AudioPlan"],
            "reference adaptation": ["reference video", "style", "copying"],
            "HITL checkpoint": ["request_user_decision", "confirm", "decision"],
            "dispatch boundary": ["Dispatch", "Craftsman", "Reviewer", "Composer"],
        },
    },
    "Craftsman": {
        "skills": [
            "seedream-renderplan-craftsman",
            "seedance-renderplan-craftsman",
            "audio-renderplan-craftsman",
            "renderplan-repair-craftsman",
        ],
        "baseline": "Write a good prompt for image, video, or audio generation.",
        "dimensions": {
            "subject bindings": ["subject_bindings", "subject consistency"],
            "reference strategy": ["reference strategy", "reference bindings"],
            "operation contract": ["operation", "output_type", "model_prompt_profile"],
            "risk notes": ["risk notes", "generation risks"],
        },
    },
    "Reviewer": {
        "skills": [
            "reviewer-quality-gate",
            "commerce-delivery-promise-reviewer",
            "reference-consistency-reviewer",
            "final-video-audio-reviewer",
        ],
        "baseline": "Check quality and say whether the video is good.",
        "dimensions": {
            "issue specificity": ["concrete", "specific", "repair"],
            "commerce promise": ["selling", "CTA", "conversion", "product"],
            "reference consistency": ["subject_consistency", "continuity", "KeyElement"],
            "audio review": ["audio_sync", "voiceover", "BGM", "ducking"],
        },
    },
    "Composer": {
        "skills": [
            "composer-timeline-director",
            "ffmpeg-audio-mix-composer",
            "platform-export-composer",
            "composer-blocker-escalation",
        ],
        "baseline": "Concatenate clips and render the final video.",
        "dimensions": {
            "timeline plan": ["TimelinePlan", "shot order", "audio_tracks"],
            "audio mix": ["voiceover", "BGM", "ducking", "AAC"],
            "platform export": ["aspect ratio", "safe", "platform"],
            "blocked escalation": ["blocked", "missing", "unusable", "next action"],
        },
    },
}


def read_skill(name: str) -> str:
    path = skill_root / name / "SKILL.md"
    if not path.exists():
        raise SystemExit(f"missing skill: {path}")
    return path.read_text(encoding="utf-8")


def covers(text: str, terms: list[str]) -> bool:
    lowered = text.lower()
    return any(term.lower() in lowered for term in terms)


lines = [
    "# M8.3 Skill Quality Loop Smoke Report",
    "",
    "**Brief**：" + brief,
    "",
    "This deterministic smoke compares a no-skill baseline against the loaded M8 skill pack. It does not call paid providers or nondeterministic models; it verifies whether the loaded skill manuals add the concrete production dimensions required by the M8 milestone.",
    "",
    "| Role | Baseline coverage | Skill-enabled coverage | Result |",
    "|---|---:|---:|---|",
]

failures: list[str] = []
details: list[str] = []

for role, config in roles.items():
    baseline = config["baseline"]
    enabled = "\n\n".join(read_skill(name) for name in config["skills"])
    dimensions = config["dimensions"]
    baseline_hits = [name for name, terms in dimensions.items() if covers(baseline, terms)]
    enabled_hits = [name for name, terms in dimensions.items() if covers(enabled, terms)]
    missing = sorted(set(dimensions) - set(enabled_hits))
    if missing:
        failures.append(f"{role} missing skill-enabled dimensions: {', '.join(missing)}")
    if len(enabled_hits) <= len(baseline_hits):
        failures.append(f"{role} skill-enabled coverage did not improve baseline")
    result = "PASS" if not missing and len(enabled_hits) > len(baseline_hits) else "FAIL"
    lines.append(f"| {role} | {len(baseline_hits)}/{len(dimensions)} | {len(enabled_hits)}/{len(dimensions)} | {result} |")
    details.extend([
        "",
        f"## {role}",
        "",
        f"- Baseline dimensions: {', '.join(baseline_hits) if baseline_hits else 'none'}",
        f"- Skill-enabled dimensions: {', '.join(enabled_hits) if enabled_hits else 'none'}",
        f"- Skills loaded: {', '.join(config['skills'])}",
    ])

lines.extend(details)
lines.extend([
    "",
    "## Verdict",
    "",
])
if failures:
    lines.append("FAIL")
    lines.extend(f"- {failure}" for failure in failures)
else:
    lines.append("PASS")
    lines.append("- Every role has strictly better dimension coverage with the M8 skill pack than the no-skill baseline.")
    lines.append("- Craftsman coverage includes RenderPlan audit dimensions required by M8.3.")
    lines.append("- Reviewer and Composer coverage includes repairable issue and blocked/finalization dimensions.")

report_path.parent.mkdir(parents=True, exist_ok=True)
report_path.write_text("\n".join(lines) + "\n", encoding="utf-8")

if failures:
    raise SystemExit("\n".join(failures))

print(f"M8.3 skill quality smoke PASS: {report_path}")
PY
