package cmd

import "testing"

func TestEdgeLaunchCommand_Default(t *testing.T) {
	if got := edgeLaunchCommand(false, ""); got != "microsoft-edge" {
		t.Fatalf("edgeLaunchCommand(false) = %q, want microsoft-edge", got)
	}
}

func TestEdgeLaunchCommand_WaylandTextInputVersion(t *testing.T) {
	got := edgeLaunchCommand(false, "3")
	want := "env INTUNEME_EDGE_WAYLAND_TEXT_INPUT_VERSION=3 microsoft-edge"
	if got != want {
		t.Fatalf("edgeLaunchCommand(false, 3) = %q, want %q", got, want)
	}
}

func TestEdgeLaunchCommand_X11Fcitx(t *testing.T) {
	got := edgeLaunchCommand(true, "")
	want := "env INTUNEME_EDGE_OZONE=x11 XMODIFIERS=@im=fcitx GTK_IM_MODULE=xim QT_IM_MODULE=xim LC_CTYPE=zh_CN.UTF-8 microsoft-edge"
	if got != want {
		t.Fatalf("edgeLaunchCommand(true) = %q, want %q", got, want)
	}
}
