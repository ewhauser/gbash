//go:build !unix

package host

// ProcessExists reports whether pid currently refers to a live process.
func ProcessExists(pid int) (bool, error) {
	return false, nil
}
