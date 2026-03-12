//go:build !windows

package fs

import "syscall"

func MetadataFromSys(sys any) FileMetadata {
	if stat, ok := sys.(*syscall.Stat_t); ok && stat != nil {
		return FileMetadata{
			UID:        stat.Uid,
			GID:        stat.Gid,
			Underlying: stat,
		}
	}
	return FileMetadata{Underlying: sys}
}
