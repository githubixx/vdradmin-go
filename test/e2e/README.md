# UI regression tests

These tests preserve the reviewed vdradmin-go appearance after Chroma UI
integration, using baselines established before extraction plus explicitly
approved visual fixes. They use real HTTP handlers, templates, CSS, and JavaScript with an
isolated Go fixture server. They do not connect to a real VDR or modify your
configuration or recordings.

## Reproducible comparison

Install Docker, then run from the repository root:

```sh
CHROMA_UI_DIR= bash scripts/test-e2e.sh
```

The default fixture builder uses the Chroma revision pinned in `go.mod`, not a
local checkout. It and UI CI first run the non-publishing standalone dependency
preflight with workspaces disabled. The current pin is the published Chroma UI
release `v0.1.0`; a separate Chroma checkout is not required.

For coordinated local changes, optionally run:

```sh
CHROMA_UI_DIR=/path/to/chroma-ui bash scripts/test-e2e.sh
```

This explicitly supplies Chroma as a Docker named build context. The fixture
builder creates a container-local workspace; no sibling path or local
replacement is committed. A successful workspace run does not substitute for
verifying the pinned dependency through the standalone builder.

The runner builds an isolated fixture server and uses the pinned Playwright
1.64.1 Ubuntu amd64 image, including its pinned browser binaries and system fonts.
Chromium uses **exact pixel comparisons**, with the two exceptions documented
below; Firefox runs
behavior tests. Do not generate authoritative screenshots on the host OS:
font rendering can differ.

Chromium uses software rendering (`--disable-gpu`) to reduce compositing
variance. Two reviewed exceptions permit at most 100 differing pixels each in
`aurora-glass-1-mobile-timers.png` and `luxury-1-mobile-timers.png`. Repeated
captures showed 52-97 and 25-52 changing backdrop-filter edge pixels,
respectively, around translucent buttons. The limit is roughly 0.012% of the
Aurora image. Color thresholds remain zero, and dimension changes still fail.
All other screenshots permit zero differing pixels. Do not broaden these
exceptions to accommodate migration changes.

All 37 themes cover channels and timers at desktop and mobile widths.
Representative themes additionally cover recordings, configuration, the Watch
TV shell, and event details. Interaction tests cover menus, theme options,
htmx refresh, explicit theme selection, mobile navigation, and native controls.
The suite also covers both OS color schemes in system mode. The completed
integration and follow-up fixes were verified with 193 browser tests:
programme-column alignment checks exercise all 37 themes on Channels and
Playing at 1440px, 769px, and 390px in Chromium and Firefox, including varied
time digits and long-title wrapping. Metal-specific checks verify orange
collisions and red critical conflicts on timer cards and timeline blocks while
retaining metallic buttons and recording styling.
An additional Chromium/Firefox check verifies Aurora's centered navigation
badge/menu and aligned archive-job actions, bringing the suite to 195 tests.

Reports, actual/expected/diff images, and failure traces are written to ignored
`test-results/` and `playwright-report/` directories. CI runs comparisons, never
automatic snapshot updates.

## Known pre-existing layout issues

The `*-mobile-channels.png` baselines retain an existing responsive-layout bug:
the Channel label/dropdown and Day label/date dropdown do not fit within the
mobile viewport. Preserve these expectations until a separately approved fix;
this is a deferred application bug, not an accepted new regression or a reason
to regenerate the baselines. Fix it separately later.

Some `*-mobile-timers.png` baselines also retain a theme-specific layout bug:
the 0-24 hour timeline below the date dropdown does not fit within the mobile
viewport. The user confirmed that `crt-starfield-1-mobile-timers.png` renders
the timeline correctly; the full set of affected themes has not been checked.
Leave the layout and expectations unchanged in unrelated work and address
affected themes separately later.

## Intentional baseline changes

Only after reviewing a deliberate visual change:

```sh
# Replace the selector with the exact approved set of tests.
bash scripts/test-e2e.sh --project=chromium --grep 'approved test selector' --update-snapshots
bash scripts/test-e2e.sh
bash scripts/test-e2e.sh
```

Review the changed PNG files before accepting them. A migration failure is not
permission to replace the original expectations.

The 216 reference images retain the original appearance except for these
explicitly approved updates:

- `chromium/behavior.spec.js/metal-theme-dropdown-open.png`: opaque Metal
  dropdown, readable selected text, and scrolling the current selection into
  view. Changes were confined to the dropdown rectangle.
- `chromium/themes.spec.js/metal-1-{desktop,mobile}-timers.png`: orange collision
  and red critical-conflict indicators on timer cards and timeline blocks.
- All 37 `chromium/themes.spec.js/*-desktop-channels.png` images, the two
  `system-{light,dark}-channels.png` images, and
  `chromium/behavior.spec.js/metal-menu-open.png`: consistent desktop time-column
  widths so programme titles align. Mobile stacking and Information popups
  remain unchanged.
- `chromium/themes.spec.js/aurora-glass-1-{desktop,mobile}-{channels,timers}.png`:
  the later navigation-centering fix changes the desktop header height and
  mobile menu-button placement. These references retain that fix rather than
  reverting it to match the earlier screenshots.

The last alignment update changed exactly 40 images and preserved the other
176 reference hashes, including the earlier approved Metal fixes. The complete
193-test run passed afterward. These approvals do not authorize broad snapshot
regeneration for future changes.

## Local debugging

Node.js 22 or newer and Go matching the main module are needed for host runs:

```sh
npm ci
npm run test:e2e:resources
npx playwright install chromium firefox
npm run test:e2e -- --project=firefox
```

The fixture process is owned by Playwright and is stopped when the run ends.
Port 18763 must be free; the runner refuses to reuse an existing server.

## External resources

Production Google Fonts imports and htmx references are unchanged. Browser
tests fulfill those requests from a test-only cache verified against SHA-256
hashes in `resources.lock.json`. The same applies to Watch TV's pinned hls.js.
This keeps the actual font files and JavaScript
implementation stable without redistributing them in the repository.

The reviewed Google Fonts CSS responses are stored in `test/e2e/resources/`
under their SHA-256 names. Google Fonts CSS endpoints can return different
bytes across requests or locations, so cold builds use these test-only fixtures
rather than refetching mutable stylesheets. Font binaries and version-pinned
JavaScript are still downloaded into the ignored cache and strictly verified;
they are not committed. Missing or modified CSS fixtures fail explicitly, even
when a warm cache exists.

`npm run test:e2e:resources` verifies the CSS fixtures, downloads other missing
cached resources, and rejects any hash mismatch. To intentionally update the
resource lock and CSS fixtures, first download the pinned Chroma module with
`GOWORK=off go mod download github.com/githubixx/chroma-ui`, then run
`npm run test:e2e:resources:update`, review URL/hash changes, and review the
resulting screenshot differences. Discovery includes the pinned Chroma shared
and theme stylesheets plus VDR templates/styles and any local overrides.
A cold cache requires network access during
setup; screenshot rendering does not depend on live external responses.

Run `npm run test:e2e:resources:test` for the offline resource-bootstrap
regressions. The Docker build also runs them before preparing its cold cache.

Do not silently substitute system fonts or a fake htmx implementation.

## Dynamic content normalization

The server uses fixed EPG/recording data, an all-day recording timer, and
tomorrow-relative overlaps/conflicts. Tests read the timer URL from readiness
metadata so the timeline remains populated on later dates. Restart the fixture
if a run crosses its reported startup-day midnight expiry.

Only screenshot captures normalize timer calendar text, the footer year, and
random temporary storage prefixes in configuration inputs/profile cells.
Markup, links, time-of-day labels, state indicators, panel styling, and actions
are preserved. Normalized configuration values are never submitted.

The Watch TV fixture supplies a deterministic image and current programme
through the real application handlers. Screenshots wait for those responses,
fonts, and images rather than capturing a transient loading state. Real video
streaming, transcoding, and archive execution are outside this visual suite.

Watch TV captures use snapshot mode, so its toolbar differs from streaming
mode. Both share the screen/remote/channel layout. Before capturing, geometry
assertions verify three separate, equal-height desktop panels (stacked on
mobile), no Watch TV panel/grid overflow, and programme information below
the panels. Behavior tests exercise every snapshot size on desktop and mobile.
The original oversized-screen expectations were corrected after a reference
image exposed snapshot sizing that covered the neighboring panels.
Spaceship 2's desktop-only column and margin overrides are also restricted
to the desktop breakpoint so its mobile panels stack correctly.
