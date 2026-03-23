# Container Image (`ubuntu-intune/`)

The container image defines everything that runs inside the nspawn container. It's based on Ubuntu 24.04 LTS and built as a multi-stage OCI image.

## Build Process

The `Containerfile` uses a multi-stage build:

1. **scratch AS ctx** — Copies `build_files/` and `system_files/` into a minimal stage for mount access
2. **ubuntu:24.04 AS builder** — Base image with system files copied in, build script runs with cache mounts
3. **chunkah** — Rechunks image to max 64 layers for efficient OCI distribution
4. **final** — Outputs the rechunked OCI archive

Cache mounts are used for `/var/cache/apt` (persistent across builds) and `/tmp` (tmpfs).

## Packages Installed

The build script (`build_files/build`) adds Microsoft repos and installs:

| Package | Purpose |
|---------|---------|
| `ms-identity-broker` | Microsoft identity broker (device enrollment, token management) |
| `ms-edge-stable` | Microsoft Edge browser |
| `intune-portal` | Intune Portal app (patched — removes problematic polkit restart in postinst) |
| `pipewire`, `libpulse0` | Audio support (PipeWire forwarded from host) |
| `mesa-utils`, `libgl1-mesa-dri` | GPU/OpenGL support |
| `pcscd`, `yubikey-manager` | Smart card / YubiKey support |
| `libsecret-1-0`, `gnome-keyring` | Credential storage |
| `sudo`, `cracklib-runtime` | User management, password validation |
| `unattended-upgrades` | Automatic security updates |

## System Configuration

Static config files in `system_files/` are copied into the image:

### Environment
- `/etc/environment` — Disables accessibility bridge (`NO_AT_BRIDGE=1`, `GTK_A11Y=none`)

### PAM
- `/etc/pam.d/machine-shell` — PAM stack for `machinectl shell` sessions
- Password quality: `minlen=12`, requires digit, uppercase, lowercase, special character

### Systemd Overrides
- `microsoft-identity-device-broker.service` — System-level broker override
- `microsoft-identity-broker.service` (user) — Sets `DISPLAY` from `/etc/intuneme-host-display` marker
- `intune-agent.timer` (user) — Timer override for compliance check schedule

### Edge Wrapper
- `/usr/local/bin/microsoft-edge` — Wrapper script that launches Edge with appropriate flags for container environment

### Polkit
- `50-pcscd.rules` — Allows smart card daemon access

### Apt
- Logging disabled, package retention configured

## Image Distribution

Images are published to `ghcr.io/frostyard/ubuntu-intune` with tags:
- `:v1.2.3` — Stable release matching CLI version
- `:insiders` — Insiders channel (newer packages, less tested)
- `:latest` — Development builds

Images are signed with cosign. The build is automated via `.github/workflows/build-container.yml`.

## Profile Script

The container image works in concert with `internal/provision/intuneme-profile.sh` (embedded in the Go binary, written to rootfs during init). This script runs on every login shell session and:

1. Reads host DISPLAY from `/etc/intuneme-host-display`
2. Sets `XAUTHORITY=/run/host-xauthority` for X11 auth
3. Imports Wayland, PipeWire, PulseAudio sockets into systemd user session
4. On first login per boot (marker in `/tmp`):
   - Initializes `gnome-keyring-daemon` with `--replace --unlock`
   - Stores a test secret to force default keyring collection creation
   - Restarts identity brokers to pick up the initialized keyring
5. Starts the `intune-agent` timer for compliance checks
