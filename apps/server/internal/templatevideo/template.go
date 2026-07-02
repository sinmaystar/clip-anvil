package templatevideo

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"regexp"
	"strings"
)

type RenderInput struct {
	TemplateKey string
	DurationSec int
	Ratio       string
	Resolution  string
	FPS         int
	Variables   map[string]any
	Assets      []Asset
}

type Asset struct {
	ClientKey     string
	WorkspacePath string
	Mime          string
}

type Meta struct {
	TemplateKey    string `json:"template_key"`
	DurationSec    int    `json:"duration_sec"`
	Ratio          string `json:"ratio"`
	Resolution     string `json:"resolution"`
	FPS            int    `json:"fps"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	DurationFrames int    `json:"duration_frames"`
}

type renderData struct {
	Meta         Meta
	Headline     string
	Caption      string
	CTA          string
	Primary      string
	Accent       string
	Product      string
	Scenes       []sceneData
	VariantLabel string
	VariantProof string
}

type sceneData struct {
	Index    int
	Headline string
	Caption  string
	Badge    string
}

var hexColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func Render(input RenderInput) (string, Meta, error) {
	input.TemplateKey = strings.TrimSpace(input.TemplateKey)
	if !isKnownTemplate(input.TemplateKey) {
		return "", Meta{}, fmt.Errorf("unknown template_key %q", input.TemplateKey)
	}
	meta, err := normalizeMeta(input)
	if err != nil {
		return "", Meta{}, err
	}
	colors, err := brandColors(input.Variables)
	if err != nil {
		return "", Meta{}, err
	}
	product := ""
	if len(input.Assets) > 0 {
		product = strings.TrimSpace(input.Assets[0].WorkspacePath)
	}
	if requiresProductImage(input.TemplateKey) && product == "" {
		return "", Meta{}, fmt.Errorf("%s requires a product image", input.TemplateKey)
	}
	spec := templateLibrarySpecs[input.TemplateKey]
	data := renderData{
		Meta:         meta,
		Headline:     stringVariable(input.Variables, "headline", "保持画面节奏"),
		Caption:      stringVariable(input.Variables, "caption", "使用已确认素材生成低成本兜底视频"),
		CTA:          stringVariable(input.Variables, "cta", "了解更多"),
		Primary:      colors[0],
		Accent:       colors[1],
		Product:      product,
		Scenes:       sceneVariables(input.Variables),
		VariantLabel: spec.Label,
		VariantProof: spec.Proof,
	}
	var buf bytes.Buffer
	tmpl := staticFallbackTemplate
	if input.TemplateKey == "marketing_ad_4_scene_v1" {
		tmpl = marketingAdFourSceneTemplate
	} else if _, ok := templateLibrarySpecs[input.TemplateKey]; ok {
		tmpl = templateLibraryVariantTemplate
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", Meta{}, err
	}
	return buf.String(), meta, nil
}

type templateLibrarySpec struct {
	Label string
	Proof string
}

var templateLibrarySpecs = map[string]templateLibrarySpec{
	"product_hero_v1": {
		Label: "产品主视觉",
		Proof: "大图露出 + 品牌主张",
	},
	"benefit_cards_v1": {
		Label: "卖点卡片",
		Proof: "三段式卖点，适合信息密度更高的广告",
	},
	"comparison_v1": {
		Label: "对比呈现",
		Proof: "把出行前后的省力感做成对照",
	},
	"testimonial_v1": {
		Label: "用户评价",
		Proof: "用口碑语气降低营销感",
	},
	"price_cta_v1": {
		Label: "价格行动",
		Proof: "突出优惠和立即行动",
	},
}

func isKnownTemplate(templateKey string) bool {
	if templateKey == "static_fallback_ken_burns_v1" || templateKey == "marketing_ad_4_scene_v1" {
		return true
	}
	_, ok := templateLibrarySpecs[templateKey]
	return ok
}

func requiresProductImage(templateKey string) bool {
	if templateKey == "marketing_ad_4_scene_v1" {
		return true
	}
	_, ok := templateLibrarySpecs[templateKey]
	return ok
}

func normalizeMeta(input RenderInput) (Meta, error) {
	duration := input.DurationSec
	if duration == 0 {
		duration = 5
	}
	if !allowedInt(duration, 3, 4, 5, 6, 8, 10) {
		return Meta{}, fmt.Errorf("duration_sec %d is not supported", duration)
	}
	fps := input.FPS
	if fps == 0 {
		fps = 24
	}
	if fps != 24 && fps != 30 {
		return Meta{}, fmt.Errorf("fps %d is not supported", fps)
	}
	resolution := strings.TrimSpace(input.Resolution)
	if resolution == "" {
		resolution = "1080p"
	}
	if resolution != "720p" && resolution != "1080p" {
		return Meta{}, fmt.Errorf("resolution %q is not supported", resolution)
	}
	ratio := strings.TrimSpace(input.Ratio)
	if ratio == "" {
		ratio = "9:16"
	}
	width, height, ok := dimensions(ratio, resolution)
	if !ok {
		return Meta{}, fmt.Errorf("ratio %q is not supported", ratio)
	}
	return Meta{
		TemplateKey:    input.TemplateKey,
		DurationSec:    duration,
		Ratio:          ratio,
		Resolution:     resolution,
		FPS:            fps,
		Width:          width,
		Height:         height,
		DurationFrames: duration * fps,
	}, nil
}

func dimensions(ratio string, resolution string) (int, int, bool) {
	longSide := 1080
	if resolution == "720p" {
		longSide = 720
	}
	switch ratio {
	case "9:16":
		return longSide, longSide * 16 / 9, true
	case "16:9":
		return longSide * 16 / 9, longSide, true
	case "1:1":
		return longSide, longSide, true
	default:
		return 0, 0, false
	}
}

func brandColors(variables map[string]any) ([]string, error) {
	colors := []string{"#111827", "#F5C542"}
	raw, ok := variables["brand_colors"]
	if !ok {
		return colors, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("brand_colors must be an array")
	}
	for i, value := range values {
		color, ok := value.(string)
		if !ok || !hexColorPattern.MatchString(color) {
			return nil, fmt.Errorf("brand_colors[%d] must be #RRGGBB", i)
		}
		if i < len(colors) {
			colors[i] = color
		}
	}
	return colors, nil
}

func stringVariable(variables map[string]any, key string, fallback string) string {
	value, ok := variables[key]
	if !ok {
		return fallback
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return fallback
	}
	return strings.TrimSpace(text)
}

func sceneVariables(variables map[string]any) []sceneData {
	fallbacks := []sceneData{
		{Index: 0, Headline: stringVariable(variables, "headline", "轻装短途出发"), Caption: "一箱装下周末和通勤。", Badge: "01"},
		{Index: 1, Headline: "顺滑万向轮", Caption: "推行、转弯、过站台都更省力。", Badge: "02"},
		{Index: 2, Headline: "轻便硬壳", Caption: "好拿好推，短途出行更松弛。", Badge: "03"},
		{Index: 3, Headline: stringVariable(variables, "cta", "现在出发"), Caption: stringVariable(variables, "caption", "悦行行李箱，让旅途更省心。"), Badge: "GO"},
	}
	raw, ok := variables["scenes"].([]any)
	if !ok {
		return fallbacks
	}
	for i := 0; i < len(raw) && i < len(fallbacks); i++ {
		item, ok := raw[i].(map[string]any)
		if !ok {
			continue
		}
		fallbacks[i].Headline = stringFromMap(item, "headline", fallbacks[i].Headline)
		fallbacks[i].Caption = stringFromMap(item, "caption", fallbacks[i].Caption)
		fallbacks[i].Badge = stringFromMap(item, "badge", fallbacks[i].Badge)
	}
	return fallbacks
}

func stringFromMap(values map[string]any, key string, fallback string) string {
	value, ok := values[key]
	if !ok {
		return fallback
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return fallback
	}
	return strings.TrimSpace(text)
}

func allowedInt(value int, allowed ...int) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

var staticFallbackTemplate = htmltemplate.Must(htmltemplate.New("static_fallback_ken_burns_v1").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Headline}}</title>
  <style>
    html, body { margin: 0; width: 100%; height: 100%; background: {{.Primary}}; font-family: "Noto Sans CJK SC", "PingFang SC", sans-serif; }
    .root { position: relative; overflow: hidden; width: {{.Meta.Width}}px; height: {{.Meta.Height}}px; color: #fff; background: linear-gradient(160deg, {{.Primary}}, #0f172a 58%, {{.Accent}}); }
    .product { position: absolute; inset: 9% 11% 34%; display: grid; place-items: center; animation: kenburns {{.Meta.DurationSec}}s ease-out forwards; }
    .product img { max-width: 82%; max-height: 82%; object-fit: contain; filter: drop-shadow(0 28px 60px rgba(0,0,0,.34)); }
    .copy { position: absolute; left: 8%; right: 8%; bottom: 9%; display: grid; gap: 18px; }
    h1 { margin: 0; font-size: 86px; line-height: 1.04; letter-spacing: 0; }
    p { margin: 0; font-size: 38px; line-height: 1.32; }
    .cta { justify-self: start; padding: 20px 30px; background: #fff; color: #111827; font-size: 34px; font-weight: 700; border-radius: 8px; }
    @keyframes kenburns { from { transform: scale(1) translateY(12px); } to { transform: scale(1.08) translateY(-8px); } }
  </style>
  <script nonce="clipanvil">
    window.__timelines = window.__timelines || {};
    window.__timelines["static_fallback_ken_burns_v1"] = {
      pause() { return this; },
      seek() { return this; },
      time() { return 0; },
      duration() { return {{.Meta.DurationSec}}; },
      totalDuration() { return {{.Meta.DurationSec}}; },
      getChildren() { return []; }
    };
  </script>
</head>
<body>
  <main class="root" data-composition-id="static_fallback_ken_burns_v1" data-start="0" data-duration="{{.Meta.DurationSec}}" data-fps="{{.Meta.FPS}}" data-width="{{.Meta.Width}}" data-height="{{.Meta.Height}}">
    <section class="product">
      {{if .Product}}<img src="{{.Product}}" alt="product">{{end}}
    </section>
    <section class="copy">
      <h1>{{.Headline}}</h1>
      <p>{{.Caption}}</p>
      <div class="cta">{{.CTA}}</div>
    </section>
  </main>
</body>
</html>
`))

var marketingAdFourSceneTemplate = htmltemplate.Must(htmltemplate.New("marketing_ad_4_scene_v1").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Headline}}</title>
  <style>
    html, body { margin: 0; width: 100%; height: 100%; background: {{.Primary}}; font-family: "Noto Sans CJK SC", "PingFang SC", sans-serif; }
    .root { position: relative; overflow: hidden; width: {{.Meta.Width}}px; height: {{.Meta.Height}}px; color: #fff; background: radial-gradient(circle at 24% 14%, rgba(255,255,255,.18), transparent 26%), linear-gradient(160deg, {{.Primary}}, #14213d 54%, {{.Accent}}); }
    .grain { position: absolute; inset: 0; opacity: .16; background-image: linear-gradient(90deg, rgba(255,255,255,.08) 1px, transparent 1px), linear-gradient(0deg, rgba(255,255,255,.05) 1px, transparent 1px); background-size: 44px 44px; }
    .product { position: absolute; left: 9%; right: 9%; top: 12%; height: 44%; display: grid; place-items: center; animation: productMove {{.Meta.DurationSec}}s ease-in-out forwards; }
    .product img { max-width: 88%; max-height: 96%; object-fit: contain; filter: drop-shadow(0 34px 70px rgba(0,0,0,.42)); }
    .brand { position: absolute; left: 8%; top: 6%; font-size: 28px; letter-spacing: 0; opacity: .86; font-weight: 700; }
    .scene { position: absolute; left: 7%; right: 7%; bottom: 7%; opacity: 0; transform: translateY(34px); }
    .copy-safe-panel { padding: 24px 28px 28px; border-radius: 8px; background: linear-gradient(90deg, rgba(9, 14, 26, .84), rgba(9, 14, 26, .62) 72%, rgba(9, 14, 26, .16)); box-shadow: 0 22px 70px rgba(0,0,0,.22); }
    .scene[data-scene-index="0"] { animation: scene0 {{.Meta.DurationSec}}s linear forwards; }
    .scene[data-scene-index="1"] { animation: scene1 {{.Meta.DurationSec}}s linear forwards; }
    .scene[data-scene-index="2"] { animation: scene2 {{.Meta.DurationSec}}s linear forwards; }
    .scene[data-scene-index="3"] { animation: scene3 {{.Meta.DurationSec}}s linear forwards; }
    .badge { display: inline-grid; place-items: center; min-width: 74px; height: 54px; padding: 0 18px; border: 2px solid rgba(255,255,255,.74); border-radius: 8px; font-size: 26px; font-weight: 800; color: {{.Accent}}; background: rgba(0,0,0,.16); }
    h1 { margin: 18px 0 12px; font-size: 82px; line-height: 1.04; letter-spacing: 0; }
    p { margin: 0; max-width: 880px; font-size: 38px; line-height: 1.32; color: rgba(255,255,255,.94); text-shadow: 0 2px 16px rgba(0,0,0,.28); }
    .cta { display: inline-block; margin-top: 28px; padding: 20px 30px; background: #fff; color: #111827; font-size: 34px; font-weight: 800; border-radius: 8px; }
    @keyframes productMove { 0% { transform: scale(.94) translateY(18px); } 42% { transform: scale(1.03) translateY(-6px); } 100% { transform: scale(1.1) translateY(-18px); } }
    @keyframes scene0 { 0% { opacity: 0; transform: translateY(34px); } 8%, 21% { opacity: 1; transform: translateY(0); } 25%, 100% { opacity: 0; transform: translateY(-24px); } }
    @keyframes scene1 { 0%, 25% { opacity: 0; transform: translateY(34px); } 33%, 46% { opacity: 1; transform: translateY(0); } 50%, 100% { opacity: 0; transform: translateY(-24px); } }
    @keyframes scene2 { 0%, 50% { opacity: 0; transform: translateY(34px); } 58%, 71% { opacity: 1; transform: translateY(0); } 75%, 100% { opacity: 0; transform: translateY(-24px); } }
    @keyframes scene3 { 0%, 75% { opacity: 0; transform: translateY(34px); } 86%, 100% { opacity: 1; transform: translateY(0); } }
  </style>
  <script nonce="clipanvil">
    window.__timelines = window.__timelines || {};
    window.__timelines["marketing_ad_4_scene_v1"] = {
      pause() { return this; },
      seek() { return this; },
      time() { return 0; },
      duration() { return {{.Meta.DurationSec}}; },
      totalDuration() { return {{.Meta.DurationSec}}; },
      getChildren() { return []; }
    };
  </script>
</head>
<body>
  <main class="root" data-composition-id="marketing_ad_4_scene_v1" data-start="0" data-duration="{{.Meta.DurationSec}}" data-fps="{{.Meta.FPS}}" data-width="{{.Meta.Width}}" data-height="{{.Meta.Height}}">
    <div class="grain"></div>
    <div class="brand">{{.Headline}}</div>
    <section class="product">
      <img src="{{.Product}}" alt="product">
    </section>
    {{range .Scenes}}
    <section class="scene copy-safe-panel" data-scene-index="{{.Index}}">
      <div class="badge">{{.Badge}}</div>
      <h1>{{.Headline}}</h1>
      <p>{{.Caption}}</p>
      {{if eq .Index 3}}<div class="cta">现在出发</div>{{end}}
    </section>
    {{end}}
  </main>
</body>
</html>
`))

var templateLibraryVariantTemplate = htmltemplate.Must(htmltemplate.New("template_library_variant_v1").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Headline}}</title>
  <style>
    html, body { margin: 0; width: 100%; height: 100%; background: {{.Primary}}; font-family: "Noto Sans CJK SC", "PingFang SC", sans-serif; }
    .root { position: relative; overflow: hidden; width: {{.Meta.Width}}px; height: {{.Meta.Height}}px; color: #fff; background: linear-gradient(155deg, {{.Primary}} 0%, #0b1220 52%, {{.Accent}} 112%); }
    .root::before { content: ""; position: absolute; inset: 0; background-image: linear-gradient(90deg, rgba(255,255,255,.08) 1px, transparent 1px), linear-gradient(0deg, rgba(255,255,255,.05) 1px, transparent 1px); background-size: 42px 42px; opacity: .18; }
    .tag { position: absolute; left: 7%; top: 5%; z-index: 2; padding: 14px 20px; border: 2px solid rgba(255,255,255,.5); border-radius: 8px; font-size: 25px; font-weight: 800; color: {{.Accent}}; background: rgba(0,0,0,.18); }
    .product { position: absolute; z-index: 1; inset: 10% 7% 36%; display: grid; place-items: center; animation: productDrift {{.Meta.DurationSec}}s ease-in-out forwards; }
    .product img { max-width: 88%; max-height: 96%; object-fit: contain; filter: drop-shadow(0 34px 74px rgba(0,0,0,.42)); }
    .copy { position: absolute; z-index: 2; left: 7%; right: 7%; bottom: 7%; display: grid; gap: 18px; }
    h1 { margin: 0; font-size: 76px; line-height: 1.04; letter-spacing: 0; }
    .caption { margin: 0; font-size: 34px; line-height: 1.34; color: rgba(255,255,255,.88); }
    .proof { font-size: 26px; line-height: 1.25; color: rgba(255,255,255,.72); }
    .cards { display: grid; grid-template-columns: repeat(3, 1fr); gap: 14px; margin-top: 10px; }
    .card { min-height: 144px; padding: 18px; border-radius: 8px; background: rgba(255,255,255,.13); border: 1px solid rgba(255,255,255,.22); }
    .card strong { display: block; font-size: 26px; line-height: 1.12; color: #fff; }
    .card span { display: block; margin-top: 10px; font-size: 21px; line-height: 1.24; color: rgba(255,255,255,.76); }
    .cta { justify-self: start; margin-top: 6px; padding: 20px 30px; background: #fff; color: #111827; font-size: 34px; font-weight: 900; border-radius: 8px; }
    .root[data-template-variant="product_hero_v1"] .product { inset: 8% 5% 31%; }
    .root[data-template-variant="product_hero_v1"] h1 { font-size: 86px; }
    .root[data-template-variant="benefit_cards_v1"] .product { inset: 8% 10% 43%; }
    .root[data-template-variant="benefit_cards_v1"] .copy { bottom: 6%; }
    .root[data-template-variant="comparison_v1"] .cards { grid-template-columns: 1fr 1fr; }
    .root[data-template-variant="comparison_v1"] .card:nth-child(3) { grid-column: 1 / span 2; min-height: 92px; }
    .root[data-template-variant="testimonial_v1"] .card { font-style: italic; background: rgba(255,255,255,.18); }
    .root[data-template-variant="price_cta_v1"] .cta { background: {{.Accent}}; color: #111827; }
    @keyframes productDrift { 0% { transform: scale(.94) translateY(20px); } 48% { transform: scale(1.03) translateY(-4px); } 100% { transform: scale(1.08) translateY(-20px); } }
  </style>
  <script nonce="clipanvil">
    window.__timelines = window.__timelines || {};
    window.__timelines[{{.Meta.TemplateKey}}] = {
      pause() { return this; },
      seek() { return this; },
      time() { return 0; },
      duration() { return {{.Meta.DurationSec}}; },
      totalDuration() { return {{.Meta.DurationSec}}; },
      getChildren() { return []; }
    };
  </script>
</head>
<body>
  <main class="root" data-template-variant="{{.Meta.TemplateKey}}" data-composition-id="{{.Meta.TemplateKey}}" data-start="0" data-duration="{{.Meta.DurationSec}}" data-fps="{{.Meta.FPS}}" data-width="{{.Meta.Width}}" data-height="{{.Meta.Height}}">
    <div class="tag">{{.VariantLabel}}</div>
    <section class="product">
      <img src="{{.Product}}" alt="product">
    </section>
    <section class="copy">
      <h1>{{.Headline}}</h1>
      <p class="caption">{{.Caption}}</p>
      <div class="proof">{{.VariantProof}}</div>
      <div class="cards">
        {{range .Scenes}}
        {{if lt .Index 3}}<div class="card"><strong>{{.Headline}}</strong><span>{{.Caption}}</span></div>{{end}}
        {{end}}
      </div>
      <div class="cta">{{.CTA}}</div>
    </section>
  </main>
</body>
</html>
`))
