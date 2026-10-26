//go:build !linux && !darwin

package analyze

// statfs is unavailable on platforms outside the project's support matrix
// (Windows and others). The tool targets Termux/Android and Linux; this
// fallback keeps `go build ./...` working everywhere without pretending
// to know the volume's free space.
func statfs(path string) Volume {
	return Volume{Path: path, Error: "disk usage reporting not supported on this platform"}
}
