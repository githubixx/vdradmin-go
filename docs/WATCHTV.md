# Watch TV (Snapshots + Streaming)

The **Watch TV** page (`/watch`) supports two modes:

- **Default (snapshot) mode**: periodic JPEG snapshots via **SVDRP `GRAB`**
- **Optional (streaming) mode**: in-browser live playback via either:
  - the built-in **HLS proxy** (recommended), or
  - an externally configured stream URL template

In both modes, the **remote control** uses **SVDRP `HITK`** and channel switching uses **SVDRP `CHAN`**.

## How `/watch` works in vdradmin-go

At a high level:

- `GET /watch` renders the UI (TV screen + remote + channel list).
- `POST /watch/key` sends a remote key press (server calls SVDRP `HITK`).
- `POST /watch/channel` switches to a channel (server calls SVDRP `CHAN`).
- Snapshot mode:
  - `GET /watch/snapshot?...` fetches a JPEG snapshot (server calls SVDRP `GRAB` and returns the image).
- Streaming mode (built-in HLS proxy):
  - `GET /watch/stream/{channel}/index.m3u8` serves the playlist
  - `GET /watch/stream/{channel}/{segment}` serves `.ts` segments

The channel list shown on `/watch` is restricted to the channels configured in vdradmin-go (same filtering as elsewhere in the UI).

## Snapshot prerequisites (SVDRP `GRAB`)

`GRAB` works only if the VDR instance can actually **grab a frame** on the VDR host.

Key points:

- `GRAB` is implemented inside VDR as a call to the **primary device’s** screenshot capability.
- If the primary device cannot provide a frame (common on headless recording-only VDRs), VDR returns:

  - `451 Grab image failed`

In other words: **SVDRP can be enabled and functional for timers/EPG/remote control, yet `GRAB` can still be impossible**.

### Typical setups where `GRAB` fails

- Recording-only VDR with only tuner devices and no video output/decoder device.
- Headless VDR (no GPU/HDMI/X11/DRM output device plugin).
- Output device plugin loaded, but not selected as the primary device.

## Quick checks on the VDR host

Run these on the machine where VDR is running.

### 1) Confirm SVDRP is reachable

- `svdrpsend PING`

You should see a `250 ... is alive` reply.

### 2) Check the active primary device

- `svdrpsend LSTD`
- `svdrpsend PRIM`

`LSTD` prints devices with flags like:

- `P` = Primary device
- `D` = Has decoder / can replay (device-dependent)

Example (headless tuner-only):

- `1 [-P] ...`  → primary but **no decoder**

This is a strong indicator that `GRAB` will fail.

### 3) Check which plugins are loaded

- `svdrpsend PLUG`

If you only see non-output plugins (e.g. `epgsearch`) and no output device plugin, then there is likely no device that can produce screenshots.

## Common `GRAB` behaviors and errors

### `451 Grab image failed`

This usually means:

- VDR accepted the command, but could not produce an image from the current primary device.

Typical reasons:

- Headless/recording-only VDR instance
- No output/decoder device plugin
- Output device not primary

### `550 Grabbing to file not allowed (use "GRAB -" instead)`

VDR can be configured to disallow saving grabs to disk.

vdradmin-go uses `GRAB .jpg ...` (base64 image data), not “grab to file”, so this is mostly relevant for manual testing.

### `Access denied!`

SVDRP access control can reject your client.

Fix by adjusting SVDRP host access settings (VDR’s svdrp host allowlist / configuration).

## Manual `GRAB` test

Try a base64-returning grab:

- `svdrpsend GRAB .jpg 80 960 540`

Expected success:

- One or more `216-...` lines (base64 chunks)
- Final `216 Grabbed image ...`

If you consistently get `451 Grab image failed`, the Watch TV snapshot feature cannot work with the current VDR runtime setup.

## What still works without `GRAB`

Even if snapshots don’t work, these can still work:

- `HITK` (remote key presses)
- `CHAN` (channel switching)
- timers / EPG features

That means `/watch` will still be useful as a remote-control surface, but the “TV screen” will show an error overlay.

## When you actually need streaming

If your goal is to **watch live TV in the browser** from a headless recorder:

- Snapshots via `GRAB` are typically not viable.
- You’ll want a streaming approach (plugin + player pipeline).

Snapshot mode is the default, but streaming is supported when configured (see below).

## Optional stream URL mode (headless setups)

If your VDR is headless/recording-only and `GRAB` fails (for example `451 Grab image failed`), you can enable streaming for `/watch`.

### HLS Proxy (Recommended)

**vdradmin-go includes a built-in HLS transcoding proxy** that converts a streamdev MPEG-TS HTTP stream into browser-playable HLS.

**Setup:**
1. Install ffmpeg on the vdradmin-go host: `pacman -S ffmpeg` (Arch) or `apt install ffmpeg` (Debian/Ubuntu)
2. In vdradmin-go web UI, go to **Configurations** → **VDR** section
3. Set **Streamdev backend URL (HLS proxy)** to: `http://127.0.0.1:3000/{channel}`
4. Click **Save**
5. Navigate to `/watch` — streams will automatically transcode to HLS

**How it works:**
- When `/watch` is in streaming mode, the UI calls `POST /watch/channel` for tuning.
- When the browser requests `/watch/stream/{channel}/index.m3u8`, vdradmin-go starts (or reuses) a per-channel `ffmpeg` process.
- ffmpeg outputs a live HLS playlist (`index.m3u8`) and `.ts` segments under the OS temp directory (typically `/tmp/vdradmin-hls/{channel}/`).
- vdradmin-go serves:
  - `/watch/stream/{channel}/index.m3u8`
  - `/watch/stream/{channel}/{segment}`
- On channel switch, vdradmin-go stops active HLS processes to free DVB tuners.
- Idle streams are cleaned up after ~5 minutes without access.

**Browser playback:**
- Chromium/Firefox: playback uses **hls.js** (via the `/watch` page) for reliability.
- Safari/iOS: usually relies on native HLS (hls.js may be unsupported there).

**Requirements:**
- `ffmpeg` installed on the vdradmin-go host
- `vdr-plugin-streamdev-server` configured and running on VDR host

**Notes:**
- The first playlist request may take a few seconds while the DVB device tunes and ffmpeg starts.
- If you see `non-existing PPS ...` / `no frame!` messages from ffmpeg, that usually indicates corrupt/partial input during tuning; it often stabilizes after the channel lock.

### Alternative: External HLS stream URL

If you already have an external **HLS (.m3u8)** endpoint (for example via streamdev `EXT` + externremux, tvheadend, or another transcoder), configure `vdr.stream_url_template` instead of using the built-in HLS proxy:

```yaml
vdr:
  stream_url_template: "http://your-transcoder:8080/hls/{channel}.m3u8"
```

Note: This does not implement transcoding; it only embeds the URL.

Important:
- The `/watch` page prefers **hls.js** in Chromium/Firefox, so the URL should be an **HLS manifest** (`.m3u8`).
- Raw streamdev URLs like `http://127.0.0.1:3000/{channel}` typically serve MPEG-TS and are usually not playable directly in browsers.

### Using streamdev-server

`streamdev-server` exposes an HTTP server (default port is commonly **3000**, configurable in the plugin setup). It can serve channels by number or by *unique channel id*.

Examples from the streamdev documentation:

- By channel number: `http://hostname:3000/3` or `http://hostname:3000/TS/3`
- By unique channel id: `http://hostname:3000/PES/S19.2E-0-12480-898`

For vdradmin-go configuration fields that support `{channel}`, the placeholder is replaced with the **channel number** (1, 2, 3...).

#### Using streamdev with the built-in HLS proxy (recommended)

Configure the streamdev URL as the **HLS proxy backend**:

```yaml
vdr:
  streamdev_backend_url: "http://127.0.0.1:3000/{channel}"
```

This is the intended usage: streamdev provides MPEG-TS, vdradmin-go transcodes it to HLS.

#### Using streamdev for external streaming

If you want to use `stream_url_template` instead (no built-in transcoding), the URL should generally point to an **HLS (.m3u8)** output produced by something else.

Important: streamdev typically serves **MPEG-TS** (or PES/ES). Most browsers do *not* play raw TS directly in a `<video>` tag.
To get true in-browser playback without the built-in proxy you generally need a browser-friendly format (HLS), which usually requires an external remux/transcode step (for example streamdev's `EXT` mode with `externremux.sh`, or a separate proxy/transcoder).
