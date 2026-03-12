package udev

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/frostyard/intuneme/internal/runner"
)

const (
	RulesFile  = "70-intuneme-yubikey.rules"
	RulesDir   = "/etc/udev/rules.d"
	ScriptDir  = "/usr/local/lib/intuneme"
	ScriptName = "usb-hotplug"
	StateDir   = "/run/intuneme/devices"

	YubicoVendorID = "1050"
)

// rulesTemplate is the udev rule content. The script path is fixed.
const rulesTemplate = `# intuneme: forward Yubico USB security keys to systemd-nspawn container.
# Matches vendor ID 1050 (Yubico) on any USB port.
# Managed by intuneme — do not edit manually.

# USB device nodes (/dev/bus/usb/BBB/DDD)
ACTION=="add", SUBSYSTEM=="usb", ATTR{idVendor}=="1050", RUN+="` + ScriptDir + `/` + ScriptName + ` add %E{DEVNAME}"
ACTION=="remove", SUBSYSTEM=="usb", ENV{ID_VENDOR_ID}=="1050", ENV{DEVNAME}=="/dev/bus/usb/*", RUN+="` + ScriptDir + `/` + ScriptName + ` remove %E{DEVNAME}"

# HID raw interfaces (/dev/hidraw*) — for FIDO2/U2F
ACTION=="add", SUBSYSTEM=="hidraw", ATTRS{idVendor}=="1050", RUN+="` + ScriptDir + `/` + ScriptName + ` add /dev/%k"
ACTION=="remove", SUBSYSTEM=="hidraw", ENV{ID_VENDOR_ID}=="1050", RUN+="` + ScriptDir + `/` + ScriptName + ` remove /dev/%k"
`

// scriptTemplate is the usb-hotplug shell script. %s is replaced with the machine name.
func scriptContent(machine string) string {
	return `#!/bin/bash
# intuneme USB hotplug handler — forwards Yubico devices to nspawn container.
# Managed by intuneme — do not edit manually.
set -euo pipefail

ACTION="${1:-}"
DEVNODE="${2:-}"
MACHINE="` + machine + `"
STATE_DIR="` + StateDir + `"
LOG_TAG="intuneme-usb"

log() { logger -t "$LOG_TAG" "$@" 2>/dev/null || true; }

[ -z "$ACTION" ] || [ -z "$DEVNODE" ] && exit 0

# Check if container is running.
LEADER=$(machinectl show "$MACHINE" -p Leader --value 2>/dev/null) || exit 0
[ -z "$LEADER" ] || [ "$LEADER" = "0" ] && exit 0

state_file() { echo "$STATE_DIR/$(echo "$1" | tr / _)"; }

case "$ACTION" in
  add)
    [ ! -e "$DEVNODE" ] && { log "add: $DEVNODE does not exist, skipping"; exit 0; }

    # Get device major:minor (hex).
    MAJOR=$(stat -c '0x%t' "$DEVNODE" 2>/dev/null) || { log "add: cannot stat $DEVNODE"; exit 0; }
    MINOR=$(stat -c '0x%T' "$DEVNODE" 2>/dev/null) || exit 0

    # Allow the device in the container's cgroup. DevicePolicy=auto preserves
    # the existing nspawn device policy and adds our device on top.
    systemctl set-property "machine-${MACHINE}.scope" DevicePolicy=auto "DeviceAllow=$DEVNODE rwm" 2>/dev/null || true

    # Create device node inside container.
    nsenter -t "$LEADER" -m -- mkdir -p "$(dirname "$DEVNODE")" 2>/dev/null || true
    nsenter -t "$LEADER" -m -- rm -f "$DEVNODE" 2>/dev/null || true
    nsenter -t "$LEADER" -m -- mknod "$DEVNODE" c "$MAJOR" "$MINOR" 2>/dev/null || {
      log "add: mknod failed for $DEVNODE"; exit 0
    }
    nsenter -t "$LEADER" -m -- chmod 0666 "$DEVNODE"

    # Record forwarded device.
    mkdir -p "$STATE_DIR"
    echo "$DEVNODE" > "$(state_file "$DEVNODE")"
    log "add: forwarded $DEVNODE to $MACHINE"
    ;;

  remove)
    SF="$(state_file "$DEVNODE")"
    [ ! -f "$SF" ] && exit 0

    nsenter -t "$LEADER" -m -- rm -f "$DEVNODE" 2>/dev/null || true
    rm -f "$SF"
    log "remove: removed $DEVNODE from $MACHINE"
    ;;
esac
`
}

// YubikeyDevice represents a detected Yubico USB security key.
type YubikeyDevice struct {
	// USBDevice is the USB device node path (e.g., /dev/bus/usb/003/009).
	USBDevice string
	// HIDRawDevices are associated hidraw device paths (e.g., /dev/hidraw3).
	HIDRawDevices []string
	// Product is the product ID (e.g., "0406").
	Product string
	// Name is a human-readable product name from sysfs.
	Name string
}

// Devices returns all device node paths that should be forwarded.
func (y YubikeyDevice) Devices() []string {
	devs := []string{y.USBDevice}
	devs = append(devs, y.HIDRawDevices...)
	return devs
}

// RulesPath returns the full path to the udev rules file.
func RulesPath() string {
	return filepath.Join(RulesDir, RulesFile)
}

// ScriptPath returns the full path to the helper script.
func ScriptPath() string {
	return filepath.Join(ScriptDir, ScriptName)
}

// sudoWriteFile writes data to path via a temp file + sudo install.
func sudoWriteFile(r runner.Runner, path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp("", "intuneme-udev-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	_ = tmp.Close()
	_, err = r.Run("sudo", "install", "-m", fmt.Sprintf("%04o", perm), tmp.Name(), path)
	return err
}

// Install writes the udev rule file and helper script, then reloads udev.
func Install(r runner.Runner, machineName string) error {
	// Create script directory.
	if _, err := r.Run("sudo", "mkdir", "-p", ScriptDir); err != nil {
		return fmt.Errorf("create script dir: %w", err)
	}

	// Write helper script.
	if err := sudoWriteFile(r, ScriptPath(), []byte(scriptContent(machineName)), 0755); err != nil {
		return fmt.Errorf("install helper script: %w", err)
	}

	// Write udev rule.
	if err := sudoWriteFile(r, RulesPath(), []byte(rulesTemplate), 0644); err != nil {
		return fmt.Errorf("install udev rule: %w", err)
	}

	// Reload udev rules.
	if _, err := r.Run("sudo", "udevadm", "control", "--reload-rules"); err != nil {
		return fmt.Errorf("reload udev rules: %w", err)
	}

	return nil
}

// Remove deletes the udev rule file, helper script, and state directory.
// It is intentionally graceful: missing files and failed reloads are not errors.
func Remove(r runner.Runner) error {
	rulesExisted := fileExists(RulesPath())
	scriptExisted := fileExists(ScriptPath())

	if rulesExisted {
		_, _ = r.Run("sudo", "rm", "-f", RulesPath())
	}
	if scriptExisted {
		_, _ = r.Run("sudo", "rm", "-f", ScriptPath())
	}

	// Clean up empty script directory (ignore errors — may not be empty).
	_, _ = r.Run("sudo", "rmdir", ScriptDir)

	// Clean up state directory.
	_, _ = r.Run("sudo", "rm", "-rf", StateDir)

	// Reload udev rules if we removed the rule file.
	if rulesExisted {
		_, _ = r.Run("sudo", "udevadm", "control", "--reload-rules")
	}

	return nil
}

// IsInstalled reports whether the udev rules file is installed.
func IsInstalled() bool {
	return fileExists(RulesPath())
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DetectYubikeys scans sysfs for currently plugged Yubico USB devices.
func DetectYubikeys() []YubikeyDevice {
	var devices []YubikeyDevice

	usbDevicesDir := "/sys/bus/usb/devices"
	entries, err := os.ReadDir(usbDevicesDir)
	if err != nil {
		return nil
	}

	for _, e := range entries {
		devDir := filepath.Join(usbDevicesDir, e.Name())

		vendor, err := readSysfsAttr(devDir, "idVendor")
		if err != nil || vendor != YubicoVendorID {
			continue
		}

		product, _ := readSysfsAttr(devDir, "idProduct")
		name, _ := readSysfsAttr(devDir, "product")
		busnum, _ := readSysfsAttr(devDir, "busnum")
		devnum, _ := readSysfsAttr(devDir, "devnum")

		if busnum == "" || devnum == "" {
			continue
		}

		usbDevNode := fmt.Sprintf("/dev/bus/usb/%03s/%03s", busnum, devnum)
		// Verify the device node exists.
		if !fileExists(usbDevNode) {
			continue
		}

		dev := YubikeyDevice{
			USBDevice: usbDevNode,
			Product:   product,
			Name:      name,
		}

		// Find associated hidraw devices.
		dev.HIDRawDevices = findHIDRawDevices(devDir)

		devices = append(devices, dev)
	}

	return devices
}

// readSysfsAttr reads a single sysfs attribute file.
func readSysfsAttr(devDir, attr string) (string, error) {
	data, err := os.ReadFile(filepath.Join(devDir, attr))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// findHIDRawDevices finds hidraw device nodes associated with a USB device sysfs path.
func findHIDRawDevices(usbDevDir string) []string {
	// HID raw devices appear as subdirectories like:
	//   <usbdev>/<iface>/<hid-dev>/hidraw/hidrawN
	pattern := filepath.Join(usbDevDir, "*", "*", "hidraw", "hidraw*")
	matches, _ := filepath.Glob(pattern)

	var devices []string
	for _, m := range matches {
		name := filepath.Base(m)
		devNode := "/dev/" + name
		if fileExists(devNode) {
			devices = append(devices, devNode)
		}
	}
	return devices
}

// ForwardDevice creates a device node inside the running container using nsenter.
// For hidraw devices, it also adjusts the cgroup device allow list.
func ForwardDevice(r runner.Runner, machine, devnode string) error {
	leaderPID, err := leaderPID(r, machine)
	if err != nil {
		return err
	}

	// Get major:minor of the device.
	out, err := r.Run("stat", "-c", "0x%t 0x%T", devnode)
	if err != nil {
		return fmt.Errorf("stat %s: %w", devnode, err)
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) != 2 {
		return fmt.Errorf("unexpected stat output for %s: %q", devnode, string(out))
	}
	major, minor := parts[0], parts[1]

	// Allow the device in the container's cgroup. DevicePolicy=auto preserves
	// the existing nspawn device policy and adds our device on top.
	_, _ = r.Run("sudo", "systemctl", "set-property",
		fmt.Sprintf("machine-%s.scope", machine),
		"DevicePolicy=auto",
		fmt.Sprintf("DeviceAllow=%s rwm", devnode))

	// Create the device node inside the container.
	dir := filepath.Dir(devnode)
	_, _ = r.Run("sudo", "nsenter", "-t", leaderPID, "-m", "--",
		"mkdir", "-p", dir)
	_, _ = r.Run("sudo", "nsenter", "-t", leaderPID, "-m", "--",
		"rm", "-f", devnode)
	if _, err := r.Run("sudo", "nsenter", "-t", leaderPID, "-m", "--",
		"mknod", devnode, "c", major, minor); err != nil {
		return fmt.Errorf("mknod %s: %w", devnode, err)
	}
	if _, err := r.Run("sudo", "nsenter", "-t", leaderPID, "-m", "--",
		"chmod", "0666", devnode); err != nil {
		return fmt.Errorf("chmod %s: %w", devnode, err)
	}

	// Record in state directory.
	_, _ = r.Run("sudo", "mkdir", "-p", StateDir)
	stateFile := filepath.Join(StateDir, strings.ReplaceAll(devnode, "/", "_"))
	_, _ = r.Run("sudo", "bash", "-c", fmt.Sprintf("echo %q > %s", devnode, stateFile))

	return nil
}

// leaderPID returns the PID of the container's init process.
func leaderPID(r runner.Runner, machine string) (string, error) {
	out, err := r.Run("machinectl", "show", machine, "-p", "Leader", "--value")
	if err != nil {
		return "", fmt.Errorf("machinectl show failed: %w", err)
	}
	pid := strings.TrimSpace(string(out))
	if pid == "" || pid == "0" {
		return "", fmt.Errorf("container %s is not running", machine)
	}
	return pid, nil
}

// RulesContent returns the udev rules content (for testing).
func RulesContent() string {
	return rulesTemplate
}

// ScriptContent returns the helper script content for the given machine name (for testing).
func ScriptContent(machine string) string {
	return scriptContent(machine)
}
