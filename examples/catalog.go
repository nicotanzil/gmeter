// Package examples supplies the same starter configurations to the dashboard
// and the CLI documentation. The built binary does not need the source tree.
package examples

import "embed"

//go:embed *.json jmx/*.jmx
var Files embed.FS
