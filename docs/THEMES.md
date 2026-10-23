# Theme System

vdradmin-go supports a modular theme system that allows you to easily customize the application's appearance.

## Theme Modes

These are always available:

- **system** - Automatically adapts to your operating system's light/dark mode preference (default)
- **light** - Always use light mode
- **dark** - Always use dark mode

## Bundled Themes

All 37 original themes are embedded by
[Chroma UI](https://github.com/githubixx/chroma-ui); an installed `themes/`
directory is no longer required. VDR keeps its page layouts, domain-specific
theme adjustments, menu contents, authorization, and configured selection.
Shared theme assets are served through the application, not downloaded from
GitHub at runtime. Original Google Fonts imports still need network access.

An optional `themes/` directory below `--assets-dir` supplies additional themes
or complete replacements of embedded IDs. Each local directory must contain
valid `theme.yaml` metadata and readable, nonempty `theme.css`; malformed or
incomplete local themes fail startup explicitly rather than silently falling
back. A local theme replaces the whole shared theme, including its artwork:
missing files are not borrowed from the embedded original.

Existing installations may still have the old shipped theme files. Installation
does not delete them, since they may contain user changes; they become overrides
and can mask embedded theme updates. Back up that directory outside the active
assets tree, then restore only intentional complete overrides. A missing local
theme directory is valid and uses the embedded catalog.

Theme IDs are the directory names (for example: `luxury-1`).

Curated themes currently included:

- `aurora-glass-1`
- `cartoon-1`
- `crt-starfield-1`
- `fitness`
- `glas-1`
- `gold-1`
- `golden-moon`
- `lighting-1`
- `luxury-1`
- `mantle-1`
- `metal-1`
- `retro-arcade`
- `solar-system-1`
- `space-night-1`
- `spaceship-2`

Spaceship themes currently included:

- `spaceship-blue-(dark|light)`
- `spaceship-cyan-(dark|light)`
- `spaceship-green-(dark|light)`
- `spaceship-grey-(dark|light)`
- `spaceship-magenta-(dark|light)`
- `spaceship-mono-(dark|light)`
- `spaceship-orange-(dark|light)`
- `spaceship-purple-(dark|light)`
- `spaceship-red-(dark|light)`
- `spaceship-yellow-(dark|light)`

The Configurations page theme dropdown shows the display name from each theme's `theme.yaml` (`name:`).

Shared assets use `/ui/css/` and `/ui/js/`. The existing
`/themes/<id>/theme.css` route remains available with revalidation, and relative
theme artwork resolves under `/themes/<id>/assets/`. Legacy
`/static/img/themes/<id>/...` URLs remain supported; application-local files
take precedence on those legacy URLs. Theme routes retain application
middleware; local static assets retain their existing serving policy.

Stylesheet ordering is shared foundation, VDR layout, selected theme, then
VDR compatibility rules. Existing domain class names remain available alongside
generic component hooks for custom themes. Built-in-theme-specific compatibility
rules apply only when `data-theme-source="bundled"` is rendered; complete local
replacements with the same ID retain control of their theme styling.
Generic items follow Chroma UI's
[item state color contract](https://github.com/githubixx/chroma-ui#item-state-color-contract):
active borders are green, warnings yellow/orange, and errors red. Default
items use an ordinary theme border, not the active-state accent. These meanings
also apply to SVG frames. VDR-specific recording/tuner treatments remain in
the application compatibility layer rather than redefining generic statuses.
For bundled Metal 1, timer collisions use orange borders and critical conflicts
use red, including their timeline blocks. Its neutral button palette and
recording treatment remain separate from these problem indicators.
Desktop programme lists reserve a consistent font-relative width for time
ranges, so titles and subtitles align across cards and channel groups.
Mobile stacking and Information-popup date layouts are unchanged.
Playing Today's TV Guide reuses the themed channel headings, `ui-item` programme
cards, enhanced selects and buttons. Its channel pager uses Chroma UI's
`ui-pager` composition; channel ranges, selection and navigation remain
application-owned. Guide-only layout rules stack time,
programme content and actions within each card, and responsive channel paging
keeps the grid within the available width without nested scrolling. Compact
guide-only card padding, row gaps and slot separation reduce the minimum heights
that drive the global timeline. Full titles and subtitle paragraphs remain
visible in guide cards; subtitles use compact margins and line spacing without
clipping or line clamping. List View and Information keep their existing layout.
Programme
positions and heights follow the same clipped time window in every channel.
Shared chronological boundary positions are measured from all channels,
including off-page ones. Each card imposes its own minimum readable height;
quiet intervals are compressed instead of letting a short clipped programme
set the scale for the whole day. Heights are therefore adaptive, not strictly
proportional to duration, and paging does not change boundary positions. Channel
headings share a measured height; themed gaps preserve times with missing data.
Overlapping EPG entries use parallel lanes within their channel.
The filter toolbar stays above the pager so
open dropdowns remain clickable, including in themes with stacked glass panels.
Bundled Spaceship 2 retains its shared responsive header wrapping on the guide;
the existing List View and other pages keep their compatibility layout.
Editing local CSS takes effect on
refresh; adding themes or changing metadata requires rediscovery/restart.

### Verifying Chroma UI is in use

Inspect an actual executable, not the repository directory. From the repository
root, build and inspect a standalone binary:

```sh
GOWORK=off make build-web
go version -m ./build/vdradmin-go | grep github.com/githubixx/chroma-ui
```

This verifies the newly built binary; it does not identify an already-running
process. `make run` uses `go run`, which creates a temporary executable.

To check the running application's assets, select an explicit theme and run
`document.documentElement.dataset` in the browser developer console.
`themeSource: "bundled"` identifies a Chroma-embedded theme; `"custom"` identifies
a local replacement managed by Chroma, and `"system"` identifies System mode.
In the Network tab, disable caching and reload: `/ui/css/base.css` and the
`/ui/js/` helpers should load successfully. Explicit themes also load
`/themes/<id>/theme.css`; this retained URL does not imply the old implementation.
The additional `/static/css/base.css` and `/static/css/theme-compat.css` are
expected VDR-owned layouts and compatibility rules.

### CRT Starfield artwork

`crt-starfield-1` uses artwork derived from the original
`web/static/img/themes/crt-starfield-1/crt-hero.png` across every page.
The optimized `crt-hero-fade.webp` dissolves the television scene into
`crt-sky.webp`, a dark photographic starfield derived from the sky in the same
image. Both runtime WebPs are embedded by Chroma UI. Page content starts
immediately below the navigation and overlays the television; no hero spacer
is inserted. The background scrolls with the page; occasional CSS comets stop
moving when reduced motion is preferred. No reference-site images or logos
are bundled.

To customize the WebPs after replacing `crt-hero.png`, first create a complete
local `crt-starfield-1` override using that theme's CSS and metadata from the
Chroma UI checkout. Then use ImageMagick from the VDR repository root (the
dimensions below match the supplied 1672x941 image). Keep the original source
and attribution details:

```bash
mkdir -p web/themes/crt-starfield-1/assets
magick web/static/img/themes/crt-starfield-1/crt-hero.png -alpha on \
    \( -size 1x941 gradient:white-black -level 0%,32% -resize 1672x941\! \) \
    -compose CopyOpacity -composite -quality 88 \
    web/themes/crt-starfield-1/assets/crt-hero-fade.webp
magick web/static/img/themes/crt-starfield-1/crt-hero.png \
    -crop 1672x300+0+0 +repage \( +clone -flip \) -append \
    -fill '#08101d' -colorize 17 -quality 87 \
    web/themes/crt-starfield-1/assets/crt-sky.webp
```

The image-model prompt used for the scene was:

> Original editorial product photograph for a dark astronomy-themed television scheduling dashboard. Wide 16:9 composition: a late-1970s portable CRT television with physical dials, curved glass and gently worn graphite casing stands on dark ground under a detailed midnight sky. On its screen is an original abstract space-program telemetry graphic with orbital arcs, a planet silhouette, and dim amber and cyan indicator lights. A thin golden glow marks the distant horizon; small sharp stars vary naturally in brightness, with a few faint meteor traces. Cinematic but crisp, dimensional lighting, restrained ink/navy/charcoal and amber palette. Keep the entire CRT in the middle third, with generous sky above and room to crop at the sides; fade the bottom into near-black. No typography, labels, watermarks, logos, people, additional devices, recognizable mission insignia, broadcast footage, glowing blobs, or exaggerated nebulae. Render at approximately 2400x1350. Also create a separately composed portrait 4:5 mobile version with the whole CRT visible and extra sky above.

## Using Themes

### Configuration

Set your preferred theme in `config.yaml`:

```yaml
ui:
    theme: "system"  # Options: system, light, dark, any bundled theme ID (e.g. "luxury-1"), or any custom theme ID
```

### Theme Selection

- **system**: Respects OS preference (auto-switches between light and dark)
- **light**: Always uses light theme regardless of OS setting  
- **dark**: Always uses dark theme regardless of OS setting
- **spaceship-*-light**: Spaceship theme with lighter accents on a dark page
- **spaceship-*-dark**: Spaceship theme with darker accents on a dark page
- **custom-name**: Uses your custom theme from `web/themes/custom-name/`

## Creating Custom Themes

Start from the CSS, metadata, and any referenced artwork in
`assets/themes/<id>/` of the Chroma UI revision pinned in `go.mod`, not the
removed VDR bundled-theme directories. Copy the complete theme into your local
override directory, then modify its variables and rules. When assigning a new
ID, update the CSS `data-theme` selectors to match it. Keep artwork in the
theme's `assets/` subdirectory and use relative URLs.

### Step 1: Create Theme Directory

Create a new directory under the `themes/` directory of your configured
`--assets-dir` (the source checkout uses `web/themes/`):

```bash
mkdir -p web/themes/my-theme
```

### Step 2: Create theme.yaml

Create `web/themes/my-theme/theme.yaml` with metadata:

```yaml
name: My Custom Theme
author: Your Name
description: A beautiful custom theme
version: 1.0.0
```

### Step 3: Create theme.css

Create `web/themes/my-theme/theme.css` with your color variables:

```css
/* My Custom Theme */

:root[data-theme="my-theme"] {
    --font-sans: 'Inter', system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif;
    --font-display: 'Space Grotesk', 'Inter', system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif;

    /* Primary colors */
    --primary-color: #your-color;
    --primary-hover: #your-hover-color;
    --secondary-color: #64748b;
    --success-color: #10b981;
    --danger-color: #ef4444;
    --warning-color: #f59e0b;

    /* Background */
    --bg-color: #your-bg-color;
    --bg-image: radial-gradient(...);

    /* Surfaces (cards, panels) */
    --surface-color: #your-surface-color;
    --surface-hover: rgba(...);

    /* Text colors */
    --text-color: #your-text-color;
    --text-muted: rgba(...);
    --border-color: rgba(...);

    /* Shadows */
    --shadow: 0 1px 2px rgba(...), 0 12px 24px rgba(...);
    --shadow-lg: 0 2px 4px rgba(...), 0 18px 44px rgba(...);

    /* Active navigation highlights */
    --nav-active-bg: linear-gradient(...);
    --nav-active-border: rgba(...);
    --nav-active-shadow: 0 10px 26px rgba(...);

    /* Menu overlay */
    --menu-scrim-color: rgba(...);
    --menu-scrim-padding: 20px;
    
    /* Layout */
    --radius: 0.75rem;
    --spacing: 1rem;
    
    /* Browser color scheme hint */
    color-scheme: light; /* or dark */
}
```

### Step 4: Configure and Restart

Update `config.yaml`:

```yaml
ui:
  theme: "my-theme"
```

Restart vdradmin-go.

If you only edit an existing theme's `theme.css`, a browser reload is usually enough because theme CSS is served with `Cache-Control: no-cache`.
You still need to restart vdradmin-go when adding a new theme directory, renaming a theme, or changing `theme.yaml` metadata because theme discovery happens at startup.

## Theme Variables Reference

### Colors

- `--primary-color` - Main accent color (buttons, links, highlights)
- `--primary-hover` - Hover state for primary elements
- `--secondary-color` - Secondary UI elements
- `--success-color` - Success indicators (green)
- `--danger-color` - Error/delete actions (red)
- `--warning-color` - Warning indicators (orange/yellow)

### Backgrounds

- `--bg-color` - Main page background color
- `--bg-image` - Optional gradient overlays on background
- `--surface-color` - Cards, panels, modals (can be gradient)
- `--surface-hover` - Hover state for surfaces

### Typography

- `--text-color` - Main text color
- `--text-muted` - Secondary/muted text (timestamps, descriptions)
- `--font-sans` - Main body font stack
- `--font-display` - Display/heading font stack

### Borders & Dividers

- `--border-color` - Border color for cards, inputs, dividers

### Shadows

- `--shadow` - Standard shadow for cards and buttons
- `--shadow-lg` - Larger shadow for hover states and modals

### Navigation

- `--nav-active-bg` - Background for active navigation items
- `--nav-active-border` - Border for active navigation items
- `--nav-active-shadow` - Shadow for active navigation items

### Layout

- `--radius` - Border radius for rounded corners
- `--spacing` - Base spacing unit (padding, margins)

## Tips

1. **Start from existing theme**: Copy `light` or `dark` and modify colors
2. **Use CSS variables**: All layout uses CSS variables - only change colors
3. **Test both modes**: Check your theme in different lighting conditions
4. **Preserve contrast**: Ensure text remains readable
5. **Restart selectively**: CSS edits to an existing theme usually only need a reload; adding a new theme or changing theme metadata requires a vdradmin-go restart

## Example: Creating a "Blue" Theme

```bash
# Copy a complete theme from your Chroma UI checkout, including any artwork
CHROMA_UI_DIR=/path/to/chroma-ui
mkdir -p web/themes
cp -r "$CHROMA_UI_DIR/assets/themes/light" web/themes/blue
sed -i 's/data-theme="light"/data-theme="blue"/g' web/themes/blue/theme.css

# Edit metadata
cat > web/themes/blue/theme.yaml << EOF
name: Blue
author: vdradmin-go
description: Cool blue theme
version: 1.0.0
EOF

# Edit --primary-color and --primary-hover in theme.css
nano web/themes/blue/theme.css

# Configure
sed -i 's/theme: "system"/theme: "blue"/' config.yaml
```

Restart vdradmin-go.

## Troubleshooting

### Theme not found

Check that:

1. For a custom theme, its directory exists in `themes/` below `--assets-dir`
2. `theme.yaml` exists and is valid YAML
3. `theme.css` exists
4. Theme name in config matches directory name exactly (case-sensitive)

### Theme looks wrong

1. Check browser console for CSS errors
2. Verify all CSS variables are defined
3. Compare with working theme (light/dark)
4. Clear browser cache (Ctrl+F5)

### Changes not showing

1. Reload the page first; theme CSS is served with `no-cache`
2. If you added a new theme or changed `theme.yaml`, restart vdradmin-go
3. Hard-refresh the browser (`Ctrl+F5`) if the old stylesheet still appears
4. Check file permissions

## Advanced: Conditional Styles

You can add media queries or conditional styles within your theme.css:

```css
:root[data-theme="my-theme"] {
    --primary-color: #2563eb;
    /* ... other variables ... */
}

/* Adjust for smaller screens */
@media (max-width: 768px) {
    :root[data-theme="my-theme"] {
        --spacing: 0.75rem;
    }
}

/* High contrast mode */
@media (prefers-contrast: high) {
    :root[data-theme="my-theme"] {
        --border-color: #000000;
    }
}
```

## Notes

- Theme CSS updates are picked up on reload because `/themes/<id>/theme.css` is served with `Cache-Control: no-cache`.
- The Configurations page theme dropdown shows `System (auto)` plus all discovered themes, using the display name from each `theme.yaml` when available.
- Embedded themes are always available. Optional local directories supplement
  or replace them, but the available theme list is built at startup, so adding
  or renaming local themes still requires a restart.
