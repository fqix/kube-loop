// Package embedded carries the platform helper binaries and the development
// gateway CA that the prebuild step stages into this directory.
//
// The package lives here because go:embed can only reach files beside the
// package that declares it, and this directory is the staging area the build
// scripts, Makefile and CI already populate.
package embedded

import (
	"embed"
	"io/fs"
)

//go:embed all:*
var files embed.FS

// Files exposes the staged helper files, rooted at this directory.
var Files fs.FS = files
