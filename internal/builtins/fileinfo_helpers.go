package builtins

import (
	stdfs "io/fs"
	"reflect"
)

type fileInfoIdentityKey struct {
	device uint64
	inode  uint64
}

func fileInfoIdentity(info stdfs.FileInfo) (fileInfoIdentityKey, bool) {
	if info == nil {
		return fileInfoIdentityKey{}, false
	}
	if dev, ino, ok := testDeviceAndInode(info); ok {
		return fileInfoIdentityKey{device: dev, inode: ino}, true
	}
	sys := reflect.ValueOf(info.Sys())
	if !sys.IsValid() {
		return fileInfoIdentityKey{}, false
	}
	if sys.Kind() == reflect.Pointer {
		if sys.IsNil() {
			return fileInfoIdentityKey{}, false
		}
		sys = sys.Elem()
	}
	if sys.Kind() != reflect.Struct {
		return fileInfoIdentityKey{}, false
	}
	nodeID := sys.FieldByName("NodeID")
	if !nodeID.IsValid() {
		return fileInfoIdentityKey{}, false
	}
	return fileInfoIdentityKey{inode: fileInfoUintField(nodeID)}, true
}

func fileInfoDevice(info stdfs.FileInfo) (uint64, bool) {
	identity, ok := fileInfoIdentity(info)
	if !ok {
		return 0, false
	}
	return identity.device, true
}

func fileInfoAllocatedBytes(info stdfs.FileInfo) int64 {
	if info == nil {
		return 0
	}
	sys := reflect.ValueOf(info.Sys())
	if sys.IsValid() {
		if sys.Kind() == reflect.Pointer {
			if !sys.IsNil() {
				sys = sys.Elem()
			}
		}
		if sys.IsValid() && sys.Kind() == reflect.Struct {
			if blocks := sys.FieldByName("Blocks"); blocks.IsValid() {
				return int64(fileInfoUintField(blocks)) * 512
			}
		}
	}
	if info.IsDir() {
		return 0
	}
	return max(info.Size(), 0)
}

func fileInfoLinkCount(info stdfs.FileInfo) (uint64, bool) {
	if info == nil {
		return 0, false
	}
	sys := reflect.ValueOf(info.Sys())
	if !sys.IsValid() {
		return 0, false
	}
	if sys.Kind() == reflect.Pointer {
		if sys.IsNil() {
			return 0, false
		}
		sys = sys.Elem()
	}
	if sys.Kind() != reflect.Struct {
		return 0, false
	}
	for _, field := range []string{"Nlink", "NLink"} {
		if value := sys.FieldByName(field); value.IsValid() {
			return fileInfoUintField(value), true
		}
	}
	return 0, false
}

func fileInfoUintField(value reflect.Value) uint64 {
	switch value.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return value.Uint()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return uint64(value.Int())
	default:
		return 0
	}
}
