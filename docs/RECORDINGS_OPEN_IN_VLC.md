# Open archived recordings in VLC/mpv (Firefox + KDE)

This project’s Archive Job UI can show a clickable output video link once encoding is finished.

For MP4 output, most browsers can play it inline.

For MKV output, browsers often **can’t** play it inline and will typically **download** the file instead. That’s expected.

This document explains how to reliably open the archived output in VLC on **Firefox + KDE (KDE Plasma 6.x)**.

## What we discovered

- **You can’t reliably force a browser to launch VLC from an HTTP link.** Modern browsers restrict “open local app” behavior.
- **MP4 inline playback is usually fine**, assuming the codec is supported by the browser.
- **MKV inline playback is not reliable** in browsers; download behavior is common.
- The best UX is:
  - provide a **direct HTTP URL** that VLC/mpv can open as a “network stream”, and
  - optionally provide **custom protocol links** (`vlc:`) for users who register URL handlers.

## What vdradmin-go provides

- A server endpoint that serves the finished output file (supports Range requests via `http.ServeFile`).
- The Archive Job page shows:
  - an **action bar** below it:
    - **Copy URL** (paste into VLC/mpv)
    - **Open in VLC** (best-effort, requires URL handler registration)

## Recommended usage (works without any KDE setup)

1. On the Archive Job page, click **Copy URL**.
2. VLC: `Media → Open Network Stream…` and paste the URL.
3. mpv: run `mpv "<PASTED_URL>"`.

This works for both MKV and MP4 output.

## Optional: enable “Open in VLC/mpv” buttons (Firefox + KDE)

These buttons use custom protocols. You must register handlers for:

- `x-scheme-handler/vlc`
- `x-scheme-handler/mpv`

### 1) Create handler script

Create `~/.local/bin/vlc-url-handler`:

```sh
#!/bin/sh
raw="$1"
raw="${raw#vlc:}"
url="$(python3 -c 'import sys,urllib.parse; print(urllib.parse.unquote(sys.argv[1]))' "$raw")"
exec vlc "$url"
```

Make it executable:

```sh
chmod +x ~/.local/bin/vlc-url-handler
```

Why URL-decoding?

Firefox/KDE can mangle nested URLs if you try `vlc://http://...`. The UI therefore emits links like:

- `vlc:<urlencoded-http-url>`

The handler script decodes that back into a normal `http(s)://...` URL before launching VLC/mpv.

### 2) Create `.desktop` files

Create `~/.local/share/applications/vlc-url-handler.desktop`:

```ini
[Desktop Entry]
Type=Application
Name=VLC URL Handler
Exec=/home/<YOUR_USER>/.local/bin/vlc-url-handler %u
NoDisplay=true
Terminal=false
MimeType=x-scheme-handler/vlc;
```

Important:

- KDE’s `kbuildsycoca6` will ignore desktop entries without `Type=Application`.
- Replace `/home/<YOUR_USER>/...` with your real path.

Rebuild KDE service cache:

```sh
kbuildsycoca6
```

### 3) Register defaults via `xdg-mime`

KDE 6.x may not expose “URL Handler” in System Settings as a separate page. Using `xdg-mime` works reliably:

```sh
xdg-mime default vlc-url-handler.desktop x-scheme-handler/vlc
```

Verify:

```sh
xdg-mime query default x-scheme-handler/vlc
```

### 4) Firefox prompt

After this, clicking **Open in VLC/mpv** should cause Firefox to prompt to open an external application.

If you don’t see a prompt, restart Firefox once after `kbuildsycoca6`.

## Notes

- MKV still won’t reliably play *inline* in the browser, but opening the **stream URL** in VLC/mpv is reliable.
- If the server requires auth/admin access, VLC/mpv may need access to the URL accordingly.

### VLC keeps asking for credentials

If VLC prompts for Basic Auth credentials every time you open a new recording, it’s usually because:

- Each archive job uses a different output URL (different job id), and VLC doesn’t reliably persist Basic Auth credentials per host/realm the way browsers do.
- On `localhost`, some clients resolve to IPv6 (`::1`) rather than IPv4 (`127.0.0.1`). If you only allow `127.0.0.1/32` as a trusted local net, IPv6 loopback won’t match.

Fix options:

- **Recommended:** run VLC on the same machine and open `http://localhost:...` URLs. vdradmin-go treats loopback (`127.0.0.1` and `::1`) as trusted, so VLC shouldn’t be prompted.
- If you want to explicitly configure it, add both loopback ranges to `auth.local_nets`:

```yaml
auth:
  local_nets:
    - "127.0.0.1/32"
    - "::1/128"
```

- For opening from another machine on your LAN without prompts, add your LAN CIDR to `auth.local_nets` (this bypasses auth for that network; treat it as a trusted network!).
