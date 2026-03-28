# Theme System

vdradmin-go supports a modular theme system that allows you to easily customize the application's appearance.

## Theme Modes

These are always available:

- **system** - Automatically adapts to your operating system's light/dark mode preference (default)
- **light** - Always use light mode
- **dark** - Always use dark mode

## Bundled Themes

This repository ships multiple theme directories under `web/themes/`. All themes are discovered dynamically at startup from subdirectories that contain a `theme.yaml` file.

Theme IDs are the directory names (for example: `luxury-1`).

Curated themes currently included:

- `cartoon-1`
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
- **spaceship-*-light**: Spaceship theme (light variant)
- **spaceship-*-dark**: Spaceship theme (dark variant)
- **custom-name**: Uses your custom theme from `web/themes/custom-name/`

## Creating Custom Themes

Custom themes are easy to create - just copy an existing theme and modify the CSS variables.

### Step 1: Create Theme Directory

Create a new directory under `web/themes/`:

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
# Copy existing theme
cp -r web/themes/light web/themes/blue

# Edit metadata
cat > web/themes/blue/theme.yaml << EOF
name: Blue
author: vdradmin-go
description: Cool blue theme
version: 1.0.0
EOF

# Edit colors in theme.css
sed -i 's/#7c3aed/#2563eb/g' web/themes/blue/theme.css  # Change purple to blue
sed -i 's/#6d28d9/#1d4ed8/g' web/themes/blue/theme.css  # Change hover to dark blue

# Configure
sed -i 's/theme: "system"/theme: "blue"/' config.yaml
```

Restart vdradmin-go.

## Troubleshooting

### Theme not found

Check that:

1. Theme directory exists in `web/themes/`
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
- Theme discovery is dynamic across directories under `web/themes/`, but the available theme list is built at startup, so adding or renaming themes still requires a restart.
