package sqlite3

import (
	gosqlite "github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/vfs"
)

// sqliteHardHeapLimitBytes caps the SQLite heap of one invocation, including
// the in-memory main database and temp storage. PRAGMA hard_heap_limit can
// only lower it.
const sqliteHardHeapLimitBytes = 128 << 20

const sqliteSandboxVFSName = "github.com/ewhauser/gbash/contrib/sqlite3.sandboxVFS"

// sqliteSandboxDSN opens the in-memory main database on the sandbox VFS, so
// that SQLite uses it for any temp or spill file the connection asks for.
const sqliteSandboxDSN = "file::memory:?vfs=" + sqliteSandboxVFSName

func init() {
	vfs.Register(sqliteSandboxVFSName, sqliteSandboxVFS{})
}

// sqliteSandboxVFS refuses every file. Database contents move in and out of
// the sandbox filesystem through serdes, so SQLite itself never needs a file,
// and the default VFS would otherwise put temp files on the host.
type sqliteSandboxVFS struct{}

func (sqliteSandboxVFS) Open(_ string, flags vfs.OpenFlag) (vfs.File, vfs.OpenFlag, error) {
	return nil, flags, gosqlite.CANTOPEN
}

func (sqliteSandboxVFS) Delete(string, bool) error {
	return gosqlite.IOERR_DELETE
}

func (sqliteSandboxVFS) Access(string, vfs.AccessFlag) (bool, error) {
	return false, nil
}

func (sqliteSandboxVFS) FullPathname(name string) (string, error) {
	return name, nil
}
