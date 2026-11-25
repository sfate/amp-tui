Release builds place `darwin-amd64` and `darwin-arm64` here before compiling
so the matching helper is embedded in each macOS binary. The Go embed directive
requires this tracked directory to have at least one file during development.
