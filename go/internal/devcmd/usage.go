package devcmd

import (
	"io"

	"github.com/wotjr1649/Clauduct/go/internal/app"
)

// usage is the same view clauduct --usage prints. One implementation, two ways in.
func usage(out io.Writer) int { return app.WriteUsage("", out) }
