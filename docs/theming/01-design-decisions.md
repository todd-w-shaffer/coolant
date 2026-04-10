# Design Decisions — Adversarial Analysis

Every design choice challenged before commitment. Each section states the question, argues both sides, and records the resolution.

---

## D1: Should TypeColor / CategoryColor be themeable?

**Pro themeable:** Full aesthetic control. A monochrome theme where everything is one hue needs to override category colors. Users with color preferences or accessibility needs should be able to change these.

**Pro fixed:** These are semantic identifiers. "Node is green, Rust is orange" is a learned vocabulary. If a theme changes them, the user loses the ability to visually parse process types at a glance. The whole point of color-coding categories is instant recognition.

**Resolution: Themeable, but themes include a `Semantic` palette that most built-in themes share.** The default semantic colors don't change between Classic/Iron/Mono — only a hypothetical "custom" theme would override them. The Theme struct includes the fields, but the built-in palette data reuses a single shared semantic map.

**Risk if wrong:** If we make them fixed, we block accessibility use cases. If we make them themeable, theme authors might create unreadable combinations. The "shared default" approach handles both.

---

## D2: Should sparkline warn/crit thresholds be part of the theme?

**Pro theme:** An "iron" theme might want to shift the green→yellow crossover to a different percentage because the color meaning is different. In traffic-light, green=fine and yellow=warning, so crossing at 70% makes sense. In a cold→hot temperature palette, "warm" might start earlier.

**Pro config:** Thresholds are operational, not aesthetic. "CPU above 70% is concerning" is a fact about the system, not a color preference. Mixing behavior and appearance creates confusion.

**Resolution: Keep thresholds in config/tuning.go. Themes only control color mapping.** The `severityColor()` function takes a value and thresholds and returns a color — the thresholds stay fixed, the gradient anchors come from the theme. This is the clean separation: config says "70% is concerning," theme says "concerning looks like this."

**Risk if wrong:** If a theme really needs different crossover points (e.g., a theme where the visual mid-point is at 50% instead of 70%), this can be revisited. But mixing them now creates a maintenance nightmare.

---

## D3: ANSI 256-color vs truecolor support

**Current state:** Mixed. The severity gradient uses truecolor (`\033[38;2;R;G;Bm`). Threat colors and gauge dots use ANSI 256 (`lipgloss.Color("208")`). Breathing dots use truecolor via lipgloss hex.

**Question:** Should the theme system target truecolor exclusively? Or support 256-color fallback?

**Analysis:**
- Coolant's target audience runs modern terminals (Ghostty, iTerm2, kitty, WezTerm). These all support truecolor.
- The braille rendering uses raw ANSI escapes (not lipgloss) for performance. Converting everything to truecolor simplifies the escape generation.
- 256-color fallback would require a parallel set of LUTs or quantization logic.
- macOS Terminal.app supports truecolor since Sequoia.

**Resolution: Truecolor only. No 256-color fallback.** The theme system defines colors as hex triples. Conversion to ANSI escapes happens at render time. If someone files a bug about 256-color terminals, we add a quantizer then — not now.

**Risk if wrong:** A user on an ancient terminal gets garbled colors. Acceptable — they can use `--theme classic` which uses simple ANSI codes (we can keep backward compat for the default theme only if needed).

**STUB: Revisit if we get actual user reports of terminal incompatibility.**

---

## D4: Should the breathing animation base color follow the theme accent?

**Current:** Hardcoded Anthropic orange (232, 115, 74) in config/tuning.go.

**Pro theme accent:** The breathing dots are a prominent visual. In a blue theme, orange hexagons would clash. The accent color should be cohesive.

**Pro brand:** Anthropic orange is a brand element. It anchors the tool's identity.

**Resolution: Theme accent wins.** The `Accent` field in the Theme struct replaces `BreatheBaseR/G/B`. Built-in "Classic" theme uses Anthropic orange. Other themes use their own accent. Brand identity comes from the default, not from preventing customization.

---

## D5: Should the offline rainbow be themeable?

**Current:** 6-color ANSI rainbow for offline sparklines — playful, irreverent.

**Analysis:** The rainbow is a mood element. It says "nothing's happening, have fun." A monochrome theme probably wants monochrome offline dots too. An iron theme might want dim purple scatter.

**Resolution: Include `OfflineColors []colorful.Color` in the theme.** Default is the current rainbow. Monochrome themes can use single-hue variants. Themes that want to suppress the playfulness can use a dim gradient.

---

## D6: thermalGradient vs overallGradient — unify or keep separate?

**Current:** Two separate 5-level gradient arrays that look similar but differ:
- `thermalGradient` (category boxes): invisible → dim amber → orange → bright orange → red
- `overallGradient` (headline bar): invisible → green → yellow → orange → red

**Why they differ:** Category boxes show process density. "3 node processes" needs to look different from "3 build processes" — the amber tones provide a warm-but-not-alarming signal. The overall headline shows system threat level — the traffic-light green→red is appropriate for an aggregate status.

**Resolution: Keep them as separate theme fields: `CategoryGradient` and `OverallGradient`.** They serve different semantic purposes. A theme author needs independent control.

**Risk if wrong:** Theme authors might find it confusing to have two gradients. Mitigate with good field documentation in the Theme struct.

---

## D7: Session phase colors — follow threat gradient or independent?

**Current:** Five phase colors (idle, active, language, build, shell-explosion) that happen to use the same values as threat colors (245, 2, 3, 208, 196).

**Question:** Should these be a derived view of the threat gradient, or a separate palette?

**Analysis:** They're semantically different. Threat level is system state (how stressed is the machine?). Session phase is workflow state (what is this session doing right now?). They happen to share colors because the progression is similar, but they could diverge.

**Resolution: Separate `SessionPhase [5]color.Color` field.** Most themes will set it to match ThreatColor, but the schema permits independence. Cheap to include, expensive to bolt on later.

---

## D8: Help view hardcoded color references

**Current:** `helpView()` in horizontal.go hardcodes color strings like `"245"`, `"2"`, `"3"`, `"208"`, `"196"` for the session diamond legend.

**Problem:** These must match the actual SessionPhase colors. If a theme changes them, the help text shows wrong colors.

**Resolution: helpView() reads from the active theme.** The layout gets a theme reference and the help view pulls session phase colors from it. This is a dependency — layout needs theme access.

---

## D9: Performance — LUT regeneration

**Current:** Two 101-entry LUTs (`greenYellowANSILUT`, `yellowRedANSILUT`) are pre-computed at init() using HCL blending.

**With themes:** Each theme needs its own LUTs. Regeneration cost: 202 HCL blends + sprintf = ~50μs. Negligible.

**Resolution: Generate LUTs at theme load time, store in the Theme struct.** No init() globals. The active theme holds its own pre-computed LUTs. Theme switching regenerates them.

**Risk if wrong:** None — the cost is trivially small.

---

## D10: Where does the Theme live in the object graph?

**Options:**
1. **Global variable** (like current colors) — simple, but makes testing hard and theme switching requires mutex
2. **Passed through constructors** — each widget gets `*Theme` at creation — clean but lots of plumbing
3. **Stored on AppState** — natural because AppState flows through Update() — but AppState is data, not config
4. **Stored on layout** — layout already owns all widgets and passes state

**Resolution: Theme is a field on the layout (`Horizontal.theme`), passed to widget constructors.** Widgets store a `*Theme` pointer. Main creates the theme from config/flags and passes it to `NewHorizontal(theme)`. This is the narrowest change that gives every widget access.

For the sparkline (which uses raw ANSI, not widget methods), the theme's pre-computed LUTs replace the package-level vars.

**STUB: If we add vertical layout later, it also gets a theme parameter. Same pattern.**

---

## D11: How are built-in themes registered?

**Options:**
1. **Switch statement** in main.go — `case "iron": theme = IronTheme()`
2. **Registry map** — `var themes = map[string]Theme{...}` 
3. **Embedded files** — TOML/JSON loaded from `embed.FS`

**Resolution: Registry map in a new `internal/theme/` package.** Simple, extensible, testable. Built-in themes are Go functions that return `Theme` structs. The map is populated at init(). External theme files (Phase 6) add to the same map at startup.

---

## D12: Dark terminal assumption

**Current:** All background colors are dark (233, 234, 235, 52). Light terminal users would see invisible text.

**Question:** Should we support light terminal themes?

**Analysis:** Coolant's users are developers running Claude Code in terminals. The demographic is overwhelmingly dark-terminal. Supporting light themes doubles the palette design work and adds a dimension to every color decision.

**Resolution: Dark terminal only for now. Don't structurally prevent light themes, but don't design for them.** The Theme struct has background colors that a light theme could override, but we don't ship any. If demand appears, add a `"light-classic"` palette.

---

## Open questions (to be resolved during implementation)

- **Q1:** Should `--theme` be a CLI flag, env var, or config file? → Start with CLI flag, add COOLANT_THEME env var.
- **Q2:** How does the theme name flow to the bash hooks? → It doesn't need to. Bash hooks don't render anything themed.
- **Q3:** Should the idle view (offline/no-data) have its own colors or share the active theme? → Share, with OfflineColors for the sparkline rainbow.
