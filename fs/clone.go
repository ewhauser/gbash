package fs

import (
	"context"
	"io"
	stdfs "io/fs"
	"os"
	"path"
	"time"
)

func clonePath(ctx context.Context, src FileSystem, srcName string, dst *MemoryFS, dstName string) error {
	return clonePathWithInfo(ctx, src, srcName, nil, dst, dstName)
}

func clonePathWithInfo(ctx context.Context, src FileSystem, srcName string, info stdfs.FileInfo, dst *MemoryFS, dstName string) error {
	var err error
	if info == nil {
		info, err = src.Lstat(ctx, srcName)
		if err != nil {
			return err
		}
	}
	ownership, hasOwnership := OwnershipFromFileInfo(info)
	absDst := Clean(dstName)
	if info.Mode()&stdfs.ModeSymlink != 0 {
		target, err := src.Readlink(ctx, srcName)
		if err != nil {
			return err
		}
		if err := dst.Symlink(ctx, target, absDst); err != nil {
			return err
		}
		if !hasOwnership {
			return nil
		}
		return dst.Chown(ctx, absDst, ownership.UID, ownership.GID, false)
	}
	if info.IsDir() {
		if err := dst.MkdirAll(ctx, absDst, info.Mode().Perm()); err != nil {
			return err
		}
		if hasOwnership {
			if err := dst.Chown(ctx, absDst, ownership.UID, ownership.GID, false); err != nil {
				return err
			}
		}
		entries, err := src.ReadDir(ctx, srcName)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			childSrc := joinChildPath(Clean(srcName), entry.Name())
			childDst := joinChildPath(absDst, entry.Name())
			var childInfo stdfs.FileInfo
			if entry.Type()&stdfs.ModeSymlink == 0 {
				childInfo, _ = entry.Info()
			}
			if err := clonePathWithInfo(ctx, src, childSrc, childInfo, dst, childDst); err != nil {
				return err
			}
		}
		return nil
	}
	if info.Mode()&stdfs.ModeType != 0 {
		return cloneSpecialFile(dst, absDst, info)
	}
	return cloneFile(ctx, src, srcName, dst, absDst, info.Mode())
}

func joinChildPath(parent, child string) string {
	if parent == "/" {
		return "/" + child
	}
	return parent + "/" + child
}

func cloneFile(ctx context.Context, src FileSystem, srcName string, dst *MemoryFS, dstName string, mode stdfs.FileMode) error {
	reader, err := src.Open(ctx, srcName)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	info, err := src.Stat(ctx, srcName)
	if err != nil {
		return err
	}

	writer, err := dst.OpenFile(ctx, dstName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	defer func() { _ = writer.Close() }()

	if _, err := io.Copy(writer, reader); err != nil {
		return err
	}
	if err := dst.Chmod(ctx, dstName, mode); err != nil {
		return err
	}
	ownership, ok := OwnershipFromFileInfo(info)
	if !ok {
		return nil
	}
	return dst.Chown(ctx, dstName, ownership.UID, ownership.GID, true)
}

func cloneSpecialFile(dst *MemoryFS, dstName string, info stdfs.FileInfo) error {
	now := info.ModTime().UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}

	dst.mu.Lock()
	defer dst.mu.Unlock()

	if err := dst.mkdirAllLocked(parentDir(dstName), 0o755); err != nil {
		return err
	}

	node := &memoryNode{
		id:      dst.newNodeIDLocked(),
		mode:    info.Mode(),
		atime:   now,
		modTime: now,
		uid:     DefaultOwnerUID,
		gid:     DefaultOwnerGID,
	}
	if info.Mode()&stdfs.ModeNamedPipe != 0 {
		node.fifo = newMemoryFIFO()
	}
	if ownership, ok := OwnershipFromFileInfo(info); ok {
		node.uid = ownership.UID
		node.gid = ownership.GID
	}
	dst.nodes[dstName] = node
	dst.nodes[parentDir(dstName)].children[path.Base(dstName)] = struct{}{} //nolint:nilaway // parent guaranteed by mkdirAllLocked above
	return nil
}
