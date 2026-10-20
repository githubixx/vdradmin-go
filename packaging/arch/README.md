# Arch Linux Packaging

`PKGBUILD` is the source-built package definition for `vdradmin-go`. The AUR Git repository should contain this file and the generated `.SRCINFO`.

## Local package build

Use these commands after a tagged release includes the package support in this directory. The checked-in `0.10.0` metadata is a release template and cannot package that earlier archive because it predates the renamed executable and staged install target.

Chroma UI is pinned in the upstream Go module to the published release
`v0.1.0`. Standalone builds fetch that exact version; no sibling checkout
or local workspace is required. Do not add local module
replacements or workspace files to the package sources. The VDR release archive
being packaged must include this dependency pin and the packaging support.

Install build prerequisites and create a clean package build:

```bash
sudo pacman -S --needed base-devel go make namcap vdr
cd packaging/arch
makepkg -sC
sudo pacman -U vdradmin-go-*.pkg.tar.zst
```

The package installs:

- `/usr/bin/vdradmin-go`
- `/usr/bin/vdradmin-go-mcp`
- `/usr/share/vdradmin-go/web/templates`
- `/usr/share/vdradmin-go/web/static`
- `/usr/lib/systemd/system/vdradmin-go.service`
- `/usr/lib/systemd/system/vdradmin-go-mcp.service`
- `/etc/vdradmin-go/config.example.yaml`
- `/usr/share/licenses/vdradmin-go/GPL-3.0-or-later.txt`

The systemd service runs as `vdr:vdr`, matching the VDR recording ownership model. It stores its mutable configuration in `/var/lib/vdradmin-go/config.yaml`; the example configuration under `/etc` is not used automatically.

Bundled themes, shared UI templates, and shared UI assets are embedded from Chroma UI. The filesystem VDR `web/templates` and `web/static` directories are still required. `web/themes` is optional and is not installed or owned by the package. Existing user-provided theme overrides under `/usr/share/vdradmin-go/web/themes/<id>/` are left untouched by `make install` and package upgrades; no cleanup or regeneration of that directory is performed.

The release archive likewise includes the required VDR assets but no theme directory. Extract upgrades into a separate directory, then use `make install` from source or copy the binaries and `web/templates`/`web/static` to the existing installation without replacing its `web/themes`. When running an extracted release directly, use `-assets-dir /path/to/web` (or run from the archive root).

Enable the service after installation:

```bash
sudo systemctl enable --now vdradmin-go.service
```

The MCP service is optional and serves its unauthenticated Streamable HTTP endpoint only on `127.0.0.1:8081` by default:

```bash
sudo systemctl enable --now vdradmin-go-mcp.service
```

The `vdr` account must be able to write each configured archive destination. `ffmpeg`, `vdr-epgsearch`, and `vdr-streamdev-server` are optional package dependencies for their respective features.

## Local production container build

The production image also ships only the required VDR filesystem assets. Optional theme overrides can be mounted at `/app/web/themes`; they do not need to exist to start the application.

For a normal standalone build, run from the vdradmin-go repository root:

```bash
make docker
```

This uses the pinned Chroma module and runs the standalone dependency preflight.
For coordinated development with a local Chroma checkout, optionally use
BuildKit and an explicit named context:

```bash
# Run from the vdradmin-go repository root; supply your own Chroma checkout.
make docker CHROMA_UI_DIR=/absolute/path/to/chroma-ui
```

This selects the `builder-workspace` stage and creates a workspace only inside
the build image. It does not change `go.mod`, publish a dependency, or require
a fixed sibling checkout path. Use the default standalone build to verify the
pinned dependency independently of local changes.

## Release update

1. Set `pkgver` to the new upstream tag and reset `pkgrel=1`.
2. Refresh the upstream source archive checksum with `updpkgsums`.
3. Run `makepkg -sC` and `namcap PKGBUILD vdradmin-go-*.pkg.tar.zst`.
4. Generate metadata with `makepkg --printsrcinfo > .SRCINFO`.
5. Copy `PKGBUILD` and `.SRCINFO` into the separate AUR Git repository, commit, and push.
