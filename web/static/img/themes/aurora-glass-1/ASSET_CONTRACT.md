# Aurora Glass 1 Asset Contract

These assets are decorative. Text, status, icons, focus indicators, and click targets remain HTML/CSS.

## Replaceable Assets

- `aurora-desktop.avif` and `aurora-desktop.webp`: 2560x1440, 16:9 background artwork.
- `aurora-mobile.avif` and `aurora-mobile.webp`: 1080x1920, portrait background artwork.
- `panel-frame.png`: a `120x80` transparent, stretchable panel-frame overlay.
- `button-frame.png`: a `128x64` transparent, stretchable button-frame overlay.
- `panel-gloss.svg`: a `100x60` transparent, stretchable panel overlay.
- `button-glint.svg`: a `100x40` transparent, stretchable button overlay.

The stylesheet references the SVG overlays plus the desktop and mobile backgrounds today. The PNG frame files are source artwork only and are intentionally not loaded until they have true nine-slice-safe corner and edge regions.

## Image Model Prompt Rules

For a background: no text, controls, logos, people, or objects. Keep the center and lower-middle dark and low-detail; reserve bright blue/cyan/violet gravitational-wave ribbons, flares, and sparkles for the outer edges and upper-right area.

For a panel or button overlay: output transparency; no text, icons, labels, logos, or shadows outside the canvas. Keep the center visually quiet and stretchable. Put any distinctive details only near corners and outer edges. Use exact canvas sizes above.

Do not request a sprite sheet. Individual named assets are easier to replace, test, cache, and apply to variable-width buttons.
