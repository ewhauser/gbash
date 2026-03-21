//go:build !linux

package shell

func currentProcessGroup() int {
	return 0
}
