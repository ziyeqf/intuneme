# Container Lifecycle

Detailed flows for each lifecycle command. All commands share the `--root` flag (default: `~/.local/share/intuneme`) and use the `runner.Runner` interface for command execution.

## `intuneme init`

One-time provisioning that creates the container from scratch.

**Flags:** `--force` (reinit), `--password-file` (read password from file), `--insiders` (use insiders channel)

**Steps:**

1. **Check prerequisites** — Verify `systemd-nspawn` and `machinectl` are available (`prereq.Check()`)
2. **Create home bind mount** — `mkdir ~/Intune`
3. **Pull OCI image** — Auto-detect puller (podman → skopeo+umoci → docker), pull from `ghcr.io/frostyard/ubuntu-intune:<tag>`
4. **Extract rootfs** — Unpack image to `~/.local/share/intuneme/rootfs/`
5. **Configure GPU access** — Detect host render group GID, create matching group in container, add user to it
6. **Create container user** — Match host UID/GID. Handles three cases: rename existing user with same UID, create new user, or update existing user's groups
7. **Set password** — Validate locally (12+ chars, mixed case, digit, special char, no username substring), then pass via bind-mounted read-only temp file to `chpasswd`
8. **Write fixups** — hostname, `/etc/hosts`, `profile.d/intuneme.sh`, `fix-home-ownership.service`, PAM sudoers
9. **Install polkit rule** — `50-intuneme.rules` to `/etc/polkit-1/rules.d/` (allows sudo group to use machinectl)
10. **Install sudoers rule** — `/etc/sudoers.d/intuneme-exec` for passwordless nsenter (validated with `visudo -c`)
11. **SELinux** (if detected) — Install custom policy module, relabel rootfs as `container_file_t`
12. **Save config** — Write `config.toml`

## `intuneme start`

Boots the container and sets up runtime environment.

**Flow:**

1. **Load config** — Read `config.toml`, verify rootfs exists
2. **Detect host sockets** — Auto-detect Wayland, PipeWire, PulseAudio, X11 auth (`nspawn.DetectHostSockets()`)
3. **Prepare broker proxy** (if enabled) — Create runtime directory, add bind mount
4. **Validate sudo** — Prompt for password if needed (`nspawn.ValidateSudo()`)
5. **Write display marker** — Write host `$DISPLAY` to `rootfs/etc/intuneme-host-display` (read by profile.d script on container login)
6. **Boot container** — `systemd-nspawn` with all bind mounts, DRI device cgroup rules, `--boot` flag
7. **Wait for registration** — Poll `machinectl` up to 30 seconds until container is listed
8. **Install udev rules** — YubiKey (`70-intuneme-yubikey.rules`) and video (`70-intuneme-video.rules`) hotplug rules
9. **Ensure sudoers** — Reinstall sudoers rule if missing (handles upgrades from older versions)
10. **Forward existing devices** — Detect already-plugged YubiKeys and video devices, forward into container
11. **Start broker proxy** (if enabled):
    - Enable systemd linger for container user
    - Create login session via `machinectl`
    - Wait for session bus socket to appear
    - Launch `intuneme broker-proxy` as background process with PID file

## `intuneme stop`

Graceful shutdown shared between `stop` and `recreate` commands.

**Flow:**

1. **Stop broker proxy** (if enabled) — Kill process by PID file, remove PID file
2. **Remove udev rules** — Delete rules files and helper script, reload udev (idempotent)
3. **Power off container** — `machinectl poweroff <machine>`
4. **Wait for deregistration** — Poll `machinectl` up to 30 seconds until container is no longer listed

## `intuneme destroy`

Removes container and enrollment state, preserves user files.

**Flow:**

1. **Stop container** if running (uses same `runStop()`)
2. **Remove sudoers rule** — Delete `/etc/sudoers.d/intuneme-exec`
3. **Delete rootfs** — `sudo rm -rf` the rootfs directory
4. **Remove config** — Delete `config.toml`
5. **Clean enrollment state** from `~/Intune`:
   - `.config/intune/` (enrollment database)
   - `.local/share/intune/`, `.local/share/intune-portal/` (app state)
   - `.local/share/keyrings/` (gnome-keyring)
   - `.local/state/microsoft-identity-broker/` (broker state)
   - `.cache/intune-portal/` (cache)
6. **Preserve user files** — Downloads, documents, etc. remain in `~/Intune`

## `intuneme recreate`

Updates the container image while preserving enrollment. Can switch channels.

**Flags:** `--insiders` (switch to insiders channel), `--password-file`

**Flow:**

1. **Early validation** — Check sudoers rule exists, validate sudo access
2. **Stop container** if running
3. **Backup state:**
   - Password hash from container's `/etc/shadow` (`provision.BackupShadowEntry()`)
   - Device broker state from `~/Intune/.local/state/microsoft-identity-broker/` (`provision.BackupDeviceBrokerState()`)
4. **Delete old rootfs** — `sudo rm -rf`
5. **Pull and extract new image** — Same as init step
6. **Re-provision** — GPU, user, hostname, fixups, polkit rule (same steps as init)
7. **Restore state:**
   - Write backed-up password hash into new shadow file
   - Copy backed-up broker state back to `~/Intune`
8. **Update config** — Save with potentially new insiders flag

## `intuneme status`

Reports container state without modifying anything.

**Output fields:** initialized, rootfs_path, machine_name, state (running/stopped), channel (stable/insiders), broker_proxy (enabled/disabled, running/stopped)

Supports `--json` for machine-readable output via `clix.OutputJSON()`.

## `intuneme shell`

Opens an interactive bash login shell inside the container.

Uses `machinectl shell <user>@<machine> /bin/bash --login`. The login shell sources `/etc/profile.d/intuneme.sh` which sets up DISPLAY, audio, and keyring.

## `intuneme open edge` / `intuneme open portal`

Launches GUI apps via `nspawn.Exec()`. Uses the nsenter pattern described in [OVERVIEW.md](OVERVIEW.md#command-execution-inside-the-container). Both are built from a shared `makeOpenAppCmd()` factory.
