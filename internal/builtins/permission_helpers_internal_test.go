package builtins

import (
	"context"
	stdfs "io/fs"
	"testing"
	"time"

	gbfs "github.com/ewhauser/gbash/fs"
)

type permissionDeniedIdentityFS struct{}

func (permissionDeniedIdentityFS) Open(context.Context, string) (gbfs.File, error) {
	return nil, stdfs.ErrPermission
}

func (permissionDeniedIdentityFS) OpenFile(context.Context, string, int, stdfs.FileMode) (gbfs.File, error) {
	return nil, stdfs.ErrPermission
}

func (permissionDeniedIdentityFS) Stat(context.Context, string) (stdfs.FileInfo, error) {
	return nil, stdfs.ErrPermission
}

func (permissionDeniedIdentityFS) Lstat(context.Context, string) (stdfs.FileInfo, error) {
	return nil, stdfs.ErrPermission
}

func (permissionDeniedIdentityFS) ReadDir(context.Context, string) ([]stdfs.DirEntry, error) {
	return nil, stdfs.ErrPermission
}

func (permissionDeniedIdentityFS) Readlink(context.Context, string) (string, error) {
	return "", stdfs.ErrPermission
}

func (permissionDeniedIdentityFS) Realpath(context.Context, string) (string, error) {
	return "", stdfs.ErrPermission
}

func (permissionDeniedIdentityFS) Symlink(context.Context, string, string) error {
	return stdfs.ErrPermission
}

func (permissionDeniedIdentityFS) Link(context.Context, string, string) error {
	return stdfs.ErrPermission
}

func (permissionDeniedIdentityFS) Chown(context.Context, string, uint32, uint32, bool) error {
	return stdfs.ErrPermission
}

func (permissionDeniedIdentityFS) Chmod(context.Context, string, stdfs.FileMode) error {
	return stdfs.ErrPermission
}

func (permissionDeniedIdentityFS) Chtimes(context.Context, string, time.Time, time.Time) error {
	return stdfs.ErrPermission
}

func (permissionDeniedIdentityFS) MkdirAll(context.Context, string, stdfs.FileMode) error {
	return stdfs.ErrPermission
}

func (permissionDeniedIdentityFS) Remove(context.Context, string, bool) error {
	return stdfs.ErrPermission
}

func (permissionDeniedIdentityFS) Rename(context.Context, string, string) error {
	return stdfs.ErrPermission
}

func (permissionDeniedIdentityFS) Getwd() string {
	return "/"
}

func (permissionDeniedIdentityFS) Chdir(string) error {
	return stdfs.ErrPermission
}

func TestLoadPermissionIdentityDBDoesNotFallbackToHostFiles(t *testing.T) {
	inv := NewInvocation(&InvocationOptions{
		Cwd:        "/",
		FileSystem: permissionDeniedIdentityFS{},
	})
	db := &permissionIdentityDB{
		usersByName:  make(map[string]uint32),
		usersByID:    make(map[uint32]string),
		groupsByName: make(map[string]uint32),
		groupsByID:   make(map[uint32]string),
	}

	loadPermissionPasswd(context.Background(), inv, db)
	loadPermissionGroup(context.Background(), inv, db)

	if len(db.usersByName) != 0 || len(db.usersByID) != 0 || len(db.groupsByName) != 0 || len(db.groupsByID) != 0 {
		t.Fatalf("identity DB = %#v, want no host fallback entries", db)
	}
}
