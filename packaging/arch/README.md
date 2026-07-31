# Arch Linux Packaging

`PKGBUILD` is the source-built package definition for `vdradmin-go`. The AUR Git repository should contain this file and the generated `.SRCINFO`.

## Local package build

Use these commands after a tagged release includes the package support in this directory. The checked-in `0.10.0` metadata is a release template and cannot package that earlier archive because it predates the renamed executable and staged install target.

Install build prerequisites and create a clean package build:

```bash
sudo pacman -S --needed base-devel go make namcap vdr
cd packaging/arch
makepkg -sC
sudo pacman -U vdradmin-go-*.pkg.tar.zst
```

The package installs:

- `/usr/bin/vdradmin-go`
- `/usr/share/vdradmin-go/web`
- `/usr/lib/systemd/system/vdradmin-go.service`
- `/etc/vdradmin-go/config.example.yaml`
- `/usr/share/licenses/vdradmin-go/GPL-3.0-or-later.txt`

The systemd service runs as `vdr:vdr`, matching the VDR recording ownership model. It stores its mutable configuration in `/var/lib/vdradmin-go/config.yaml`; the example configuration under `/etc` is not used automatically.

Enable the service after installation:

```bash
sudo systemctl enable --now vdradmin-go.service
```

The `vdr` account must be able to write each configured archive destination. `ffmpeg`, `vdr-epgsearch`, and `vdr-streamdev-server` are optional package dependencies for their respective features.

## Release update

1. Set `pkgver` to the new upstream tag and reset `pkgrel=1`.
2. Refresh the upstream source archive checksum with `updpkgsums`.
3. Run `makepkg -sC` and `namcap PKGBUILD vdradmin-go-*.pkg.tar.zst`.
4. Generate metadata with `makepkg --printsrcinfo > .SRCINFO`.
5. Copy `PKGBUILD` and `.SRCINFO` into the separate AUR Git repository, commit, and push.
