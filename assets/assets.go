// Package assets embeds the installer's images.
package assets

import _ "embed"

//go:embed logo.png
var Logo []byte

//go:embed logo-small.png
var LogoSmall []byte
