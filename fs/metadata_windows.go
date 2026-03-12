//go:build windows

package fs

func MetadataFromSys(sys any) FileMetadata {
	return FileMetadata{Underlying: sys}
}
