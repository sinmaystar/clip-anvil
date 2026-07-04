import React, {CSSProperties} from "react";
import {
  AbsoluteFill,
  Composition,
  Html5Audio,
  Img,
  Sequence,
  Video,
  interpolate,
  registerRoot,
  staticFile,
  useCurrentFrame
} from "remotion";
import {assertTimelinePlan, Segment, TextLayer, TimelineAsset, TimelinePlan} from "./schema";

const defaultPlan: TimelinePlan = {
  schema: "clipanvil.remotion_timeline.v1",
  composition: "MarketingTimeline",
  output: {width: 1080, height: 1920, fps: 30, duration_sec: 10, codec: "h264", audio_codec: "aac"},
  theme: {brand_colors: ["#111827", "#F5C542"], font_family: "Noto Sans CJK SC", style: "premium_product_ad"},
  segments: [
    {
      id: "default-segment",
      start_sec: 0,
      end_sec: 10,
      layout: "hero_packshot",
      assets: [{role: "primary", type: "image", workspace_path: "/workspace/input/product.png"}],
      caption: {source: "audio_cue", text: "Ready to go", start_sec: 0, end_sec: 10, position: "subtitle_bottom"}
    }
  ],
  audio_tracks: []
};

const fontFamily = "Noto Sans CJK SC, Noto Sans CJK, Inter, sans-serif";

const MarketingTimeline: React.FC<TimelinePlan> = (rawProps) => {
  const plan = assertTimelinePlan(rawProps);
  const primary = plan.theme?.brand_colors?.[0] ?? "#111827";
  const accent = plan.theme?.brand_colors?.[1] ?? "#F5C542";
  const fps = plan.output.fps;

  return (
    <AbsoluteFill style={{background: `linear-gradient(155deg, ${primary} 0%, #101827 58%, ${accent} 170%)`, overflow: "hidden"}}>
      {plan.segments.map((segment) => {
        const from = Math.round(segment.start_sec * fps);
        const durationInFrames = Math.max(1, Math.round((segment.end_sec - segment.start_sec) * fps));
        return (
          <Sequence key={segment.id} from={from} durationInFrames={durationInFrames} name={segment.id}>
            <SegmentView segment={segment} accent={accent} durationInFrames={durationInFrames} maxCharsPerLine={plan.captions?.max_chars_per_line ?? 18} />
          </Sequence>
        );
      })}
      {plan.audio_tracks?.map((track, index) => {
        const from = Math.max(0, Math.round((track.start_sec ?? 0) * fps));
        return (
          <Sequence key={`${track.role}-${index}`} from={from} layout="none" name={track.id ?? track.role}>
            <Html5Audio src={assetSrc(track.workspace_path)} volume={track.volume ?? (track.role === "bgm" ? 0.22 : 1)} loop={track.loop ?? track.role === "bgm"} />
          </Sequence>
        );
      })}
    </AbsoluteFill>
  );
};

const SegmentView: React.FC<{segment: Segment; accent: string; durationInFrames: number; maxCharsPerLine: number}> = ({segment, accent, durationInFrames, maxCharsPerLine}) => {
  const frame = useCurrentFrame();
  const progress = interpolate(frame, [0, Math.max(1, durationInFrames - 1)], [0, 1], {extrapolateLeft: "clamp", extrapolateRight: "clamp"});
  return (
    <AbsoluteFill style={transitionStyle(segment, frame, durationInFrames)}>
      <div style={{position: "absolute", inset: 0, background: "radial-gradient(circle at 50% 18%, rgba(255,255,255,0.12), transparent 36%), linear-gradient(180deg, rgba(255,255,255,0.06), rgba(0,0,0,0.22) 70%, rgba(0,0,0,0.44))"}} />
      {renderLayout(segment, accent, progress)}
      {segment.text_layers?.map((layer, index) => <TextLayerView key={`${segment.id}-text-${index}`} layer={layer} accent={accent} />)}
      {segment.caption?.text ? <CaptionView text={segment.caption.text} maxCharsPerLine={maxCharsPerLine} /> : null}
    </AbsoluteFill>
  );
};

const transitionStyle = (segment: Segment, frame: number, durationInFrames: number): CSSProperties => {
  const entranceFrames = Math.max(1, Math.round((segment.transition_in?.duration_sec ?? 0.34) * 30));
  const exitFrames = Math.min(8, Math.max(0, durationInFrames - 1));
  const entrance = interpolate(frame, [0, entranceFrames], [0, 1], {extrapolateLeft: "clamp", extrapolateRight: "clamp"});
  const exitOpacity = interpolate(frame, [Math.max(0, durationInFrames - exitFrames), durationInFrames], [1, 0.94], {extrapolateLeft: "clamp", extrapolateRight: "clamp"});
  const transition = segment.transition_in?.type ?? "crossfade";
  const base: CSSProperties = {opacity: exitOpacity, overflow: "hidden"};

  if (transition === "cut") {
    return base;
  }
  if (transition === "slide") {
    return {...base, opacity: entrance * exitOpacity, transform: `translateY(${interpolate(entrance, [0, 1], [52, 0])}px)`};
  }
  if (transition === "wipe") {
    return {...base, opacity: exitOpacity, clipPath: `inset(0 ${interpolate(entrance, [0, 1], [100, 0])}% 0 0)`};
  }
  if (transition === "zoom_blur") {
    return {...base, opacity: entrance * exitOpacity, transform: `scale(${interpolate(entrance, [0, 1], [1.08, 1])})`, filter: `blur(${interpolate(entrance, [0, 1], [8, 0])}px)`};
  }
  return {...base, opacity: entrance * exitOpacity};
};

const renderLayout = (segment: Segment, accent: string, progress: number) => {
  switch (segment.layout) {
    case "detail_focus":
      return <DetailFocus segment={segment} accent={accent} progress={progress} />;
    case "benefit_card":
      return <BenefitCard segment={segment} accent={accent} progress={progress} />;
    case "split_compare":
      return <SplitCompare segment={segment} accent={accent} progress={progress} />;
    case "scenario_card":
      return <ScenarioCard segment={segment} accent={accent} progress={progress} />;
    case "open_storage":
      return <OpenStorage segment={segment} accent={accent} progress={progress} />;
    case "cta_endcard":
      return <CtaEndcard segment={segment} accent={accent} progress={progress} />;
    case "hero_packshot":
    default:
      return <HeroPackshot segment={segment} accent={accent} progress={progress} />;
  }
};

const HeroPackshot: React.FC<LayoutProps> = ({segment, accent, progress}) => (
  <>
    <ProductMedia asset={primaryAsset(segment)} style={{left: "7%", right: "7%", top: "20%", bottom: "24%", transform: motionTransform(segment, progress)}} />
    <Kicker accent={accent} text={headlineText(segment, "轻松出发")} top="10%" />
  </>
);

const DetailFocus: React.FC<LayoutProps> = ({segment, accent, progress}) => (
  <>
    <ProductMedia asset={primaryAsset(segment)} style={{left: "3%", right: "3%", top: "12%", bottom: "20%", transform: motionTransform(segment, progress), filter: "drop-shadow(0 34px 70px rgba(0,0,0,0.44))"}} mediaStyle={{maxWidth: "98%", maxHeight: "98%"}} />
    <SmallLabel accent={accent} text={headlineText(segment, "细节更顺手")} top="11%" />
  </>
);

const BenefitCard: React.FC<LayoutProps> = ({segment, accent, progress}) => (
  <>
    <div style={{position: "absolute", left: "7%", top: "13%", width: "52%", color: "white", fontFamily}}>
      <div style={{width: 92, height: 8, background: accent, borderRadius: 999, marginBottom: 28}} />
      <div style={{fontSize: 68, lineHeight: 1.04, fontWeight: 860, textShadow: "0 10px 36px rgba(0,0,0,0.44)"}}>{headlineText(segment, "卖点清晰")}</div>
    </div>
    <ProductMedia asset={primaryAsset(segment)} style={{left: "22%", right: "2%", top: "30%", bottom: "22%", transform: motionTransform(segment, progress)}} mediaStyle={{maxWidth: "94%", maxHeight: "92%"}} />
  </>
);

const SplitCompare: React.FC<LayoutProps> = ({segment, accent, progress}) => {
  const first = primaryAsset(segment);
  const second = segment.assets[1] ?? first;
  return (
    <>
      <Panel left="6%" top="15%" width="42%" height="58%" accent={accent}>
        <MediaElement asset={first} style={{width: "100%", height: "100%", objectFit: "cover", transform: `scale(${interpolate(progress, [0, 1], [1.04, 1.12])})`}} />
      </Panel>
      <Panel left="52%" top="21%" width="42%" height="52%" accent={accent}>
        <MediaElement asset={second} style={{width: "100%", height: "100%", objectFit: "cover", transform: `scale(${interpolate(progress, [0, 1], [1.12, 1.04])})`}} />
      </Panel>
      <Kicker accent={accent} text={headlineText(segment, "一眼对比")} top="9%" />
    </>
  );
};

const ScenarioCard: React.FC<LayoutProps> = ({segment, accent, progress}) => (
  <>
    <ProductMedia asset={primaryAsset(segment)} style={{left: "8%", right: "8%", top: "28%", bottom: "21%", transform: motionTransform(segment, progress)}} />
    <div style={{position: "absolute", left: "7%", right: "7%", top: "11%", color: "white", fontFamily}}>
      <div style={{fontSize: 42, fontWeight: 780, color: accent, marginBottom: 12}}>SCENE</div>
      <div style={{fontSize: 62, lineHeight: 1.08, fontWeight: 850, textShadow: "0 8px 30px rgba(0,0,0,0.5)"}}>{headlineText(segment, "出行场景")}</div>
    </div>
  </>
);

const OpenStorage: React.FC<LayoutProps> = ({segment, accent, progress}) => (
  <>
    <Kicker accent={accent} text={headlineText(segment, "打开就能装")} top="10%" />
    <ProductMedia asset={primaryAsset(segment)} style={{left: "6%", right: "6%", top: "27%", bottom: "19%", transform: motionTransform(segment, progress)}} mediaStyle={{maxWidth: "96%", maxHeight: "96%"}} />
  </>
);

const CtaEndcard: React.FC<LayoutProps> = ({segment, accent, progress}) => (
  <>
    <ProductMedia asset={primaryAsset(segment)} style={{left: "11%", right: "11%", top: "18%", bottom: "30%", transform: motionTransform(segment, progress)}} />
    <div style={{position: "absolute", left: "8%", right: "8%", bottom: "19%", color: "white", fontFamily, textAlign: "center"}}>
      <div style={{fontSize: 74, lineHeight: 1.02, fontWeight: 900, textShadow: "0 10px 34px rgba(0,0,0,0.5)"}}>{headlineText(segment, "现在出发")}</div>
      <div style={{height: 10, width: 180, background: accent, borderRadius: 999, margin: "24px auto 0"}} />
    </div>
  </>
);

type LayoutProps = {segment: Segment; accent: string; progress: number};

const ProductMedia: React.FC<{asset: TimelineAsset; style: CSSProperties; mediaStyle?: CSSProperties}> = ({asset, style, mediaStyle}) => (
  <div style={{position: "absolute", display: "flex", alignItems: "center", justifyContent: "center", ...style}}>
    <MediaElement
      asset={asset}
      style={{
        maxWidth: "92%",
        maxHeight: "92%",
        objectFit: "contain",
        filter: "drop-shadow(0 30px 58px rgba(0,0,0,0.34))",
        ...mediaStyle
      }}
    />
  </div>
);

const MediaElement: React.FC<{asset: TimelineAsset; style: CSSProperties}> = ({asset, style}) => {
  const src = assetSrc(asset.workspace_path);
  if (asset.type === "video") {
    return <Video src={src} muted loop style={style} />;
  }
  return <Img src={src} style={style} />;
};

const Panel: React.FC<{left: string; top: string; width: string; height: string; accent: string; children: React.ReactNode}> = ({left, top, width, height, accent, children}) => (
  <div style={{position: "absolute", left, top, width, height, overflow: "hidden", border: `5px solid ${accent}`, boxShadow: "0 26px 70px rgba(0,0,0,0.42)"}}>
    {children}
  </div>
);

const Kicker: React.FC<{accent: string; text: string; top: string}> = ({accent, text, top}) => (
  <div style={{position: "absolute", left: "7%", right: "7%", top, color: "white", fontFamily}}>
    <div style={{width: 92, height: 8, background: accent, borderRadius: 999, marginBottom: 24}} />
    <div style={{fontSize: 76, lineHeight: 1.02, fontWeight: 900, textShadow: "0 10px 36px rgba(0,0,0,0.5)"}}>{text}</div>
  </div>
);

const SmallLabel: React.FC<{accent: string; text: string; top: string}> = ({accent, text, top}) => (
  <div style={{position: "absolute", left: "7%", right: "7%", top, color: "white", fontFamily}}>
    <span style={{display: "inline-block", padding: "12px 22px", border: `3px solid ${accent}`, fontSize: 42, lineHeight: 1, fontWeight: 820, background: "rgba(0,0,0,0.28)"}}>{text}</span>
  </div>
);

const headlineText = (segment: Segment, fallback: string) => {
  const headline = segment.text_layers?.find((layer) => layer.role === "headline" || layer.role === "hook" || layer.role === "cta")?.text;
  if (headline?.trim()) {
    return headline.trim();
  }
  if (segment.visual_focus?.trim() && segment.visual_focus.trim().length <= 12) {
    return segment.visual_focus.trim();
  }
  return fallback;
};

const motionTransform = (segment: Segment, progress: number) => {
  const preset = segment.motion?.preset ?? "push_in";
  const scale = preset === "pull_out" ? interpolate(progress, [0, 1], [1.08, 1]) : preset === "cta_pop" ? interpolate(progress, [0, 0.18, 1], [0.94, 1.04, 1]) : interpolate(progress, [0, 1], [1, 1.08]);
  const x = preset === "pan_left" ? interpolate(progress, [0, 1], [42, -42]) : preset === "pan_right" ? interpolate(progress, [0, 1], [-42, 42]) : preset === "float_parallax" ? interpolate(progress, [0, 1], [-18, 18]) : 0;
  const y = preset === "spotlight_reveal" ? interpolate(progress, [0, 1], [28, 0]) : 0;
  return `translate(${x}px, ${y}px) scale(${scale})`;
};

const TextLayerView: React.FC<{layer: TextLayer; accent: string}> = ({layer, accent}) => {
  const frame = useCurrentFrame();
  const start = Math.round(layer.start_sec * 30);
  const end = Math.round(layer.end_sec * 30);
  const opacity = interpolate(frame, [start, start + 8, Math.max(start + 9, end - 8), end], [0, 1, 1, 0], {extrapolateLeft: "clamp", extrapolateRight: "clamp"});
  const y = interpolate(frame, [start, start + 14], [34, 0], {extrapolateLeft: "clamp", extrapolateRight: "clamp"});
  const top = layer.position === "upper_third" ? "12%" : layer.position === "side_label" ? "42%" : "54%";
  return (
    <div style={{position: "absolute", left: "7%", right: "7%", top, opacity, transform: `translateY(${y}px)`, color: "white", fontFamily, fontSize: 58, lineHeight: 1.08, fontWeight: 850, textShadow: "0 8px 30px rgba(0,0,0,0.45)"}}>
      <span style={{boxDecorationBreak: "clone", WebkitBoxDecorationBreak: "clone", background: "rgba(17,24,39,0.32)", padding: "0.08em 0.18em"}}>{layer.text}</span>
      <div style={{width: 72, height: 7, marginTop: 20, background: accent, borderRadius: 999}} />
    </div>
  );
};

const CaptionView: React.FC<{text: string; maxCharsPerLine: number}> = ({text, maxCharsPerLine}) => {
  const lines = wrapChineseCaption(text, maxCharsPerLine);
  return (
    <div style={{position: "absolute", left: "8%", right: "8%", bottom: "6%", textAlign: "center", color: "white", fontFamily, fontSize: 44, lineHeight: 1.18, fontWeight: 760, textShadow: "0 5px 24px rgba(0,0,0,0.58)"}}>
      <div style={{display: "inline-block", background: "rgba(0,0,0,0.42)", padding: "14px 28px", maxWidth: "100%"}}>
        {lines.map((line, index) => (
          <div key={`${line}-${index}`}>{line}</div>
        ))}
      </div>
    </div>
  );
};

const wrapChineseCaption = (text: string, maxChars = 18): string[] => {
  const clean = text.trim();
  if (clean.length <= maxChars) {
    return [clean];
  }
  const chunks: string[] = [];
  for (let index = 0; index < clean.length; index += maxChars) {
    chunks.push(clean.slice(index, index + maxChars));
  }
  return chunks.slice(0, 2);
};

const primaryAsset = (segment: Segment) => segment.assets[0];

const assetSrc = (workspacePath: string): string => {
  if (workspacePath.startsWith("/workspace/")) {
    return staticFile(workspacePath.slice("/workspace/".length));
  }
  return staticFile(workspacePath);
};

const Root: React.FC = () => (
  <Composition
    id="MarketingTimeline"
    component={MarketingTimeline}
    durationInFrames={defaultPlan.output.duration_sec * defaultPlan.output.fps}
    fps={defaultPlan.output.fps}
    width={defaultPlan.output.width}
    height={defaultPlan.output.height}
    defaultProps={defaultPlan}
    calculateMetadata={({props}) => {
      const plan = assertTimelinePlan(props as TimelinePlan);
      return {
        durationInFrames: Math.round(plan.output.duration_sec * plan.output.fps),
        fps: plan.output.fps,
        width: plan.output.width,
        height: plan.output.height
      };
    }}
  />
);

registerRoot(Root);
