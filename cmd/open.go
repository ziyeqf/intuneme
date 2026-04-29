package cmd

import (
	"fmt"
	"os"

	"github.com/frostyard/intuneme/internal/config"
	"github.com/frostyard/intuneme/internal/nspawn"
	"github.com/frostyard/intuneme/internal/runner"
	"github.com/spf13/cobra"
)

var openCmd = &cobra.Command{
	Use:   "open",
	Short: "Launch an application inside the running container",
}

func runOpenApp(command string) error {
	r := &runner.SystemRunner{}
	root := rootDir
	if root == "" {
		var err error
		root, err = config.DefaultRoot()
		if err != nil {
			return err
		}
	}

	cfg, err := config.Load(root)
	if err != nil {
		return err
	}

	if _, err := os.Stat(cfg.RootfsPath); err != nil {
		return fmt.Errorf("not initialized — run 'intuneme init' first")
	}

	if !nspawn.IsRunning(r, cfg.MachineName) {
		return fmt.Errorf("container is not running — run 'intuneme start' first")
	}

	return nspawn.Exec(r, cfg.MachineName, cfg.HostUser, cfg.HostUID, command)
}

func makeOpenAppCmd(use, short, command string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE:  func(cmd *cobra.Command, args []string) error { return runOpenApp(command) },
	}
}

func edgeLaunchCommand(x11 bool, waylandTextInputVersion string) string {
	if !x11 {
		if waylandTextInputVersion != "" {
			return "env INTUNEME_EDGE_WAYLAND_TEXT_INPUT_VERSION=" + waylandTextInputVersion + " microsoft-edge"
		}
		return "microsoft-edge"
	}
	return "env INTUNEME_EDGE_OZONE=x11 XMODIFIERS=@im=fcitx GTK_IM_MODULE=xim QT_IM_MODULE=xim LC_CTYPE=zh_CN.UTF-8 microsoft-edge"
}

func makeOpenEdgeCmd() *cobra.Command {
	var x11 bool
	var waylandTextInputVersion string
	c := &cobra.Command{
		Use:   "edge",
		Short: "Launch Microsoft Edge inside the container",
		RunE: func(cmd *cobra.Command, args []string) error {
			if waylandTextInputVersion != "" && waylandTextInputVersion != "1" && waylandTextInputVersion != "3" {
				return fmt.Errorf("--wayland-text-input-version must be 1 or 3")
			}
			return runOpenApp(edgeLaunchCommand(x11, waylandTextInputVersion))
		},
	}
	c.Flags().BoolVar(&x11, "x11", false, "launch Edge through X11/XIM for host Fcitx input methods")
	c.Flags().StringVar(&waylandTextInputVersion, "wayland-text-input-version", "", "force Wayland text-input protocol version for Edge (1 or 3)")
	return c
}

func init() {
	openCmd.AddCommand(makeOpenEdgeCmd())
	openCmd.AddCommand(makeOpenAppCmd(
		"portal",
		"Launch Intune Portal inside the container",
		"intune-portal",
	))
	rootCmd.AddCommand(openCmd)
}
