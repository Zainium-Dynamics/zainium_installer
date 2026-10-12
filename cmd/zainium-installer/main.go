// zainium-installer is the graphical installer of the Zainium OS live session.
package main

import (
	"log"
	"os"
	"path/filepath"

	"gioui.org/app"
	"gioui.org/unit"

	"github.com/Zainium-Dynamics/zainium-installer/internal/ui"
)

const composeDir = "/overlayer/syshub/share/X11/locale"

// Gio's Wayland backend refuses to start without an XKB compose table; Zainium may ship none.
func ensureCompose() {
	if os.Getenv("XCOMPOSEFILE") != "" {
		return
	}
	if _, err := os.Stat(filepath.Join(composeDir, "compose.dir")); err == nil {
		return
	}
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	f := filepath.Join(dir, "zainium-installer.XCompose")
	if err := os.WriteFile(f, []byte("# no compose sequences\n"), 0o600); err == nil {
		os.Setenv("XCOMPOSEFILE", f)
	}
}

func main() {
	ensureCompose()
	app.ID = "tech.zainiumdynamics.Installer"
	go func() {
		w := new(app.Window)
		w.Option(
			app.Title("Install Zainium OS"),
			app.Size(unit.Dp(1024), unit.Dp(720)),
			app.MinSize(unit.Dp(760), unit.Dp(560)),
			app.Decorated(false), // the installer draws its own title bar
		)
		if err := ui.Run(w); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}
