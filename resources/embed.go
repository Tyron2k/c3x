// Package resources holds the declarative TOML catalog as an
// embedded filesystem.
//
// The catalog is data, not code — authoring a new resource is a TOML
// file under `resources/<provider>/<kind>.toml` plus a fixture entry in
// the verifier harness. The embed directive ships every catalog file
// inside the released binary so c3x is a single static artifact with
// no runtime file dependency.
package resources

import "embed"

//go:embed aws/*.toml azure/*.toml gcp/*.toml
var FS embed.FS
