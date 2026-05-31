package installer

import (
	"context"
	"os/exec"
)

var (
	TypeHomebrew = Type{name: "Homebrew", downloadable: false}
	TypePKG      = Type{name: "pkg", downloadable: true}
)

func TypeOfInstaller(ctx context.Context) Type {
	for _, packageID := range []string{"io.anonbird.client", "io.netbird.client"} {
		cmd := exec.CommandContext(ctx, "pkgutil", "--pkg-info", packageID)
		if _, err := cmd.Output(); err == nil {
			return TypePKG
		}
	}

	// Not installed using pkg file, thus installed using Homebrew.
	return TypeHomebrew
}
