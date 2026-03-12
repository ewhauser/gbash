package fs

import stdfs "io/fs"

const (
	DefaultOwnerUID uint32 = 1000
	DefaultOwnerGID uint32 = 1000
)

type FileMetadata struct {
	UID        uint32
	GID        uint32
	Underlying any
}

func MetadataFromFileInfo(info stdfs.FileInfo) FileMetadata {
	if info == nil {
		return FileMetadata{UID: DefaultOwnerUID, GID: DefaultOwnerGID}
	}
	switch meta := info.Sys().(type) {
	case FileMetadata:
		return normalizeMetadata(meta)
	case *FileMetadata:
		if meta != nil {
			return normalizeMetadata(*meta)
		}
	}
	meta := MetadataFromSys(info.Sys())
	if meta.UID == 0 && meta.GID == 0 && meta.Underlying == nil {
		meta = FileMetadata{UID: DefaultOwnerUID, GID: DefaultOwnerGID, Underlying: info.Sys()}
	}
	return normalizeMetadata(meta)
}

func normalizeMetadata(meta FileMetadata) FileMetadata {
	if meta.UID == 0 && meta.GID == 0 && meta.Underlying == nil {
		return FileMetadata{UID: DefaultOwnerUID, GID: DefaultOwnerGID}
	}
	return meta
}
