export const layoutKeys = ["hero_packshot", "detail_focus", "benefit_card", "split_compare", "scenario_card", "open_storage", "cta_endcard"] as const;
export const motionKeys = ["push_in", "pull_out", "pan_left", "pan_right", "float_parallax", "spotlight_reveal", "kinetic_text", "cta_pop"] as const;
export const transitionKeys = ["cut", "crossfade", "slide", "wipe", "zoom_blur"] as const;
export const assetTypeKeys = ["image", "video"] as const;
export type LayoutKey = (typeof layoutKeys)[number];
export type MotionKey = (typeof motionKeys)[number];
export type TransitionKey = (typeof transitionKeys)[number];
export type AssetTypeKey = (typeof assetTypeKeys)[number];

export type TimelinePlan = {
  schema: string;
  composition: string;
  output: {
    width: number;
    height: number;
    fps: number;
    duration_sec: number;
    codec?: string;
    audio_codec?: string;
  };
  theme?: {
    brand_colors?: string[];
    font_family?: string;
    style?: string;
  };
  segments: Segment[];
  audio_tracks?: AudioTrack[];
  captions?: {
    source?: string;
    single_lane?: boolean;
    max_chars_per_line?: number;
    style?: string;
  };
};

export type Segment = {
  id: string;
  shot_ref?: string;
  start_sec: number;
  end_sec: number;
  layout: LayoutKey;
  visual_focus?: string;
  assets: TimelineAsset[];
  motion?: {
    preset?: MotionKey;
    intensity?: string | number;
    direction?: string;
  };
  text_layers?: TextLayer[];
  caption?: Caption;
  transition_in?: Transition;
  transition_out?: Transition;
};

export type TimelineAsset = {
  role: string;
  node_ref?: string;
  workspace_path: string;
  type: AssetTypeKey;
};

export type TextLayer = {
  role?: string;
  text: string;
  start_sec: number;
  end_sec: number;
  position?: string;
  animation?: string;
};

export type Caption = {
  source?: string;
  text?: string;
  start_sec?: number;
  end_sec?: number;
  position?: string;
};

export type Transition = {
  type?: TransitionKey;
  duration_sec?: number;
};

export type AudioTrack = {
  id?: string;
  role: "voiceover" | "bgm" | string;
  node_ref?: string;
  workspace_path: string;
  start_sec?: number;
  volume?: number;
  fade_in_sec?: number;
  fade_out_sec?: number;
  loop?: boolean;
};

export const assertTimelinePlan = (value: TimelinePlan): TimelinePlan => {
  if (!value || value.schema !== "clipanvil.remotion_timeline.v1") {
    throw new Error("Invalid timeline schema");
  }
  if (value.composition !== "MarketingTimeline") {
    throw new Error("Invalid timeline composition");
  }
  if (!value.output || value.output.width <= 0 || value.output.height <= 0 || value.output.fps <= 0 || value.output.duration_sec <= 0) {
    throw new Error("Invalid timeline output");
  }
  if (!Array.isArray(value.segments) || value.segments.length === 0) {
    throw new Error("Timeline requires at least one segment");
  }
  for (const segment of value.segments) {
    if (!segment.id || segment.end_sec <= segment.start_sec || !Array.isArray(segment.assets) || segment.assets.length === 0) {
      throw new Error(`Invalid segment ${segment.id || "unknown"}`);
    }
    if (!layoutKeys.includes(segment.layout)) {
      throw new Error(`Invalid segment layout ${segment.layout}`);
    }
    if (segment.motion?.preset && !motionKeys.includes(segment.motion.preset)) {
      throw new Error(`Invalid motion preset ${segment.motion.preset}`);
    }
    if (segment.transition_in?.type && !transitionKeys.includes(segment.transition_in.type)) {
      throw new Error(`Invalid transition_in ${segment.transition_in.type}`);
    }
    if (segment.transition_out?.type && !transitionKeys.includes(segment.transition_out.type)) {
      throw new Error(`Invalid transition_out ${segment.transition_out.type}`);
    }
    for (const asset of segment.assets) {
      if (!assetTypeKeys.includes(asset.type)) {
        throw new Error(`Invalid asset type ${asset.type}`);
      }
      assertWorkspacePath(asset.workspace_path);
    }
  }
  for (const track of value.audio_tracks ?? []) {
    if (track.role !== "voiceover" && track.role !== "bgm") {
      throw new Error(`Invalid audio role ${track.role}`);
    }
    assertWorkspacePath(track.workspace_path);
  }
  return value;
};

const assertWorkspacePath = (value: string) => {
  if (!value || !value.startsWith("/workspace/")) {
    throw new Error(`Path must be under /workspace: ${value}`);
  }
};
