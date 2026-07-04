import React from "react";
import {AbsoluteFill, Composition, Img, interpolate, staticFile, useCurrentFrame, useVideoConfig} from "remotion";
import {registerRoot} from "remotion";

type VisualLayer = {
  role?: string;
  input_ref: string;
  fit?: "contain" | "cover";
  motion?: "slow_push_in" | "slow_pull_out" | "pan_left" | "pan_right" | "float_up" | "parallax_soft";
  start_sec?: number;
  end_sec?: number;
};

type TextLayer = {
  role?: string;
  text: string;
  start_sec?: number;
  end_sec?: number;
  animation?: "pop_slide_up" | "fade_rise" | "type_reveal" | "scale_snap" | "wipe_in";
  position?: "upper_third" | "middle_safe" | "bottom_safe";
};

type MotionShotProps = {
  duration_sec: number;
  width: number;
  height: number;
  fps: number;
  motion_style?: string;
  safe_area?: string;
  visual_layers: VisualLayer[];
  text_layers?: TextLayer[];
  brand_colors?: string[];
};

const defaultProps: MotionShotProps = {
  duration_sec: 5,
  width: 1080,
  height: 1920,
  fps: 30,
  motion_style: "premium_product_ad",
  safe_area: "caption_safe_bottom",
  visual_layers: [{role: "product", input_ref: "assets/product.png", fit: "contain", motion: "slow_push_in"}],
  text_layers: [{role: "hook", text: "轻松出发", start_sec: 0.2, end_sec: 2.4, animation: "pop_slide_up", position: "upper_third"}],
  brand_colors: ["#111827", "#F5C542"]
};

const MotionShot: React.FC<MotionShotProps> = (props) => {
  const frame = useCurrentFrame();
  const {fps, durationInFrames} = useVideoConfig();
  const primary = props.brand_colors?.[0] ?? "#111827";
  const accent = props.brand_colors?.[1] ?? "#F5C542";

  return (
    <AbsoluteFill style={{background: `linear-gradient(160deg, ${primary} 0%, #111827 58%, ${accent} 160%)`, overflow: "hidden"}}>
      {(props.visual_layers ?? []).map((layer, index) => (
        <Visual key={`${layer.input_ref}-${index}`} layer={layer} frame={frame} fps={fps} total={durationInFrames} />
      ))}
      {(props.text_layers ?? []).map((layer, index) => (
        <Text key={`${layer.text}-${index}`} layer={layer} frame={frame} fps={fps} accent={accent} />
      ))}
    </AbsoluteFill>
  );
};

const Visual: React.FC<{layer: VisualLayer; frame: number; fps: number; total: number}> = ({layer, frame, fps, total}) => {
  const start = Math.round((layer.start_sec ?? 0) * fps);
  const end = Math.round((layer.end_sec ?? total / fps) * fps);
  const progress = interpolate(frame, [start, end], [0, 1], {extrapolateLeft: "clamp", extrapolateRight: "clamp"});
  const scale = layer.motion === "slow_pull_out" ? interpolate(progress, [0, 1], [1.08, 1]) : interpolate(progress, [0, 1], [1, 1.08]);
  const x = layer.motion === "pan_left" ? interpolate(progress, [0, 1], [48, -48]) : layer.motion === "pan_right" ? interpolate(progress, [0, 1], [-48, 48]) : 0;
  const y = layer.motion === "float_up" ? interpolate(progress, [0, 1], [40, -20]) : 0;
  const opacity = interpolate(frame, [start, start + 10, end - 10, end], [0, 1, 1, 0], {extrapolateLeft: "clamp", extrapolateRight: "clamp"});
  const src = toFileURL(layer.input_ref);
  if (!src) {
    return null;
  }

  return (
    <AbsoluteFill style={{alignItems: "center", justifyContent: "center", padding: "8%", opacity}}>
      <Img
        src={src}
        style={{
          maxWidth: "92%",
          maxHeight: "68%",
          objectFit: layer.fit ?? "contain",
          filter: "drop-shadow(0 28px 50px rgba(0,0,0,0.35))",
          transform: `translate(${x}px, ${y}px) scale(${scale})`
        }}
      />
    </AbsoluteFill>
  );
};

const toFileURL = (input: string): string | null => {
  if (input.startsWith("http://") || input.startsWith("https://") || input.startsWith("data:")) {
    return input;
  }
  if (input.startsWith("/") || input.startsWith("file://")) {
    return null;
  }
  return staticFile(input);
};

const Text: React.FC<{layer: TextLayer; frame: number; fps: number; accent: string}> = ({layer, frame, fps, accent}) => {
  const start = Math.round((layer.start_sec ?? 0) * fps);
  const end = Math.round((layer.end_sec ?? 5) * fps);
  const opacity = interpolate(frame, [start, start + 8, end - 8, end], [0, 1, 1, 0], {extrapolateLeft: "clamp", extrapolateRight: "clamp"});
  const rise = interpolate(frame, [start, start + 14], [48, 0], {extrapolateLeft: "clamp", extrapolateRight: "clamp"});
  const top = layer.position === "upper_third" ? "12%" : layer.position === "bottom_safe" ? "72%" : "54%";

  return (
    <div
      style={{
        position: "absolute",
        left: "7%",
        right: "7%",
        top,
        opacity,
        transform: `translateY(${rise}px)`,
        color: "white",
        fontFamily: "Noto Sans CJK SC, Noto Sans CJK, sans-serif",
        fontSize: 76,
        lineHeight: 1.06,
        fontWeight: 800,
        textShadow: "0 6px 28px rgba(0,0,0,0.38)"
      }}
    >
      <span style={{boxDecorationBreak: "clone", WebkitBoxDecorationBreak: "clone", background: "rgba(17,24,39,0.28)", padding: "0.08em 0.18em", borderRadius: 18}}>
        {layer.text}
      </span>
      <div style={{width: 96, height: 8, marginTop: 24, background: accent, borderRadius: 999}} />
    </div>
  );
};

const Root: React.FC = () => (
  <Composition
    id="MotionShot"
    component={MotionShot}
    durationInFrames={defaultProps.duration_sec * defaultProps.fps}
    fps={defaultProps.fps}
    width={defaultProps.width}
    height={defaultProps.height}
    defaultProps={defaultProps}
    calculateMetadata={({props}) => ({
      durationInFrames: Math.round((props.duration_sec ?? defaultProps.duration_sec) * (props.fps ?? defaultProps.fps)),
      fps: props.fps ?? defaultProps.fps,
      width: props.width ?? defaultProps.width,
      height: props.height ?? defaultProps.height
    })}
  />
);

registerRoot(Root);
