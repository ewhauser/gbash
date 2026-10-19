package sqlite3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	stdfs "io/fs"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"

	gosqlite "github.com/ncruces/go-sqlite3"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/ncruces/go-sqlite3/ext/serdes"

	"github.com/ewhauser/gbash/commands"
	gbfs "github.com/ewhauser/gbash/fs"
)

type sqliteOutputMode string

const (
	sqliteModeList     sqliteOutputMode = "list"
	sqliteModeCSV      sqliteOutputMode = "csv"
	sqliteModeJSON     sqliteOutputMode = "json"
	sqliteModeLine     sqliteOutputMode = "line"
	sqliteModeColumn   sqliteOutputMode = "column"
	sqliteModeTable    sqliteOutputMode = "table"
	sqliteModeMarkdown sqliteOutputMode = "markdown"
	sqliteModeTabs     sqliteOutputMode = "tabs"
	sqliteModeBox      sqliteOutputMode = "box"
	sqliteModeQuote    sqliteOutputMode = "quote"
	sqliteModeHTML     sqliteOutputMode = "html"
	sqliteModeASCII    sqliteOutputMode = "ascii"
)

type sqliteOptions struct {
	mode      sqliteOutputMode
	header    bool
	separator string
	newline   string
	nullValue string
	readonly  bool
	bail      bool
	echo      bool
	cmd       string
}

type sqliteParsedArgs struct {
	options     sqliteOptions
	database    string
	sqlText     string
	showVersion bool
	showHelp    bool
}

type sqliteStatementResult struct {
	Columns []string
	Rows    [][]any
}

type sqliteSQLContinueError struct {
	Err error
}

func (e *sqliteSQLContinueError) Error() string {
	if e == nil || e.Err == nil {
		return "sqlite3 statement error"
	}
	return e.Err.Error()
}

func (e *sqliteSQLContinueError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type SQLite3 struct {
	// maxDuration overrides sqliteDefaultMaxDuration; tests set it.
	maxDuration time.Duration
}

// sqliteDefaultMaxDuration bounds SQL execution when the request has no
// deadline. SQLite never consults the shell's loop or command budgets, so an
// unbounded recursive CTE would otherwise run forever.
const sqliteDefaultMaxDuration = 60 * time.Second

var errSQLiteTimeLimit = errors.New("sqlite3: execution exceeded the time limit")

// sqliteExecContext adds the fallback duration to ctx when it has no
// deadline. Stdin is read before this context is derived, so time spent
// waiting on a slow producer does not count.
func sqliteExecContext(ctx context.Context, maxDuration time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	if maxDuration <= 0 {
		maxDuration = sqliteDefaultMaxDuration
	}
	return context.WithTimeoutCause(ctx, maxDuration, errSQLiteTimeLimit)
}

// sqliteContextError reports why ctx stopped: the fallback time limit as a
// command failure, or the request's own cancellation or timeout as is.
func sqliteContextError(ctx context.Context, inv *commands.Invocation) error {
	if errors.Is(context.Cause(ctx), errSQLiteTimeLimit) {
		return exitf(inv, "sqlite3: execution exceeded the %s time limit", sqliteDefaultMaxDuration)
	}
	return ctx.Err()
}

func NewSQLite3() *SQLite3 {
	return &SQLite3{}
}

func Register(registry commands.CommandRegistry) error {
	if registry == nil {
		return nil
	}
	return registry.Register(NewSQLite3())
}

func (c *SQLite3) Name() string {
	return "sqlite3"
}

func (c *SQLite3) Run(ctx context.Context, inv *commands.Invocation) error {
	parsed, err := parseSQLiteArgs(inv)
	if err != nil {
		return err
	}

	switch {
	case parsed.showHelp:
		_, _ = io.WriteString(inv.Stdout, sqliteHelpText)
		return nil
	case parsed.showVersion:
		version, err := sqliteVersion(ctx)
		if err != nil {
			return &commands.ExitError{Code: 1, Err: err}
		}
		_, _ = fmt.Fprintf(inv.Stdout, "sqlite3 (gbash) backed by ncruces/go-sqlite3 v0.31.1 / SQLite %s\n", version)
		return nil
	case parsed.database == "":
		return exitf(inv, "sqlite3: missing database argument")
	}

	if parsed.sqlText == "" {
		data, err := commands.ReadAllStdin(ctx, inv)
		if err != nil {
			return err
		}
		parsed.sqlText = string(data)
	}
	if strings.TrimSpace(parsed.sqlText) == "" {
		return exitf(inv, "sqlite3: missing SQL input")
	}

	dbPath, exists, err := resolveSQLiteDatabasePath(ctx, inv, parsed.database)
	if err != nil {
		return err
	}
	if parsed.options.readonly && parsed.database != ":memory:" && !exists {
		return exitf(inv, "sqlite3: %s: No such file or directory", parsed.database)
	}
	execCtx, cancel := sqliteExecContext(ctx, c.maxDuration)
	defer cancel()
	return runSQLiteDatabase(execCtx, inv, &parsed, dbPath, exists)
}

// sqliteOOMMessage is the panic value ncruces/go-sqlite3 raises when SQLite
// reports SQLITE_NOMEM, for example once the hard heap limit is reached.
const sqliteOOMMessage = "sqlite3: out of memory"

func runSQLiteDatabase(ctx context.Context, inv *commands.Invocation, parsed *sqliteParsedArgs, dbPath string, exists bool) (err error) {
	// Registered before the connection's Close so that it runs after Close
	// while a NOMEM panic unwinds.
	defer func() {
		if recovered := recover(); recovered != nil {
			if oom, ok := recovered.(error); ok && oom.Error() == sqliteOOMMessage {
				err = exitf(inv, "%s", sqliteOOMMessage)
				return
			}
			panic(recovered)
		}
	}()

	conn, err := gosqlite.OpenContext(ctx, sqliteSandboxDSN)
	if err != nil {
		return &commands.ExitError{Code: 1, Err: err}
	}
	defer func() { _ = conn.Close() }()
	// OpenContext only uses ctx while opening; keep it attached so the
	// request timeout and cancellation interrupt running statements.
	conn.SetInterrupt(ctx)

	if err := configureSQLiteConn(conn, parsed.options.readonly); err != nil {
		return &commands.ExitError{Code: 1, Err: err}
	}
	if parsed.database != ":memory:" && exists {
		data, err := readSQLiteDatabase(ctx, inv, dbPath)
		if err != nil {
			return err
		}
		if len(data) > 0 {
			if err := serdes.Deserialize(conn, "main", data); err != nil {
				return exitf(inv, "sqlite3: %v", err)
			}
		}
	}

	var wrote bool
	if parsed.options.cmd != "" {
		cmdWrote, err := runSQLiteBatch(ctx, inv, conn, parsed.options.cmd, &parsed.options)
		wrote = wrote || cmdWrote
		if err != nil {
			return err
		}
	}

	sqlWrote, err := runSQLiteBatch(ctx, inv, conn, parsed.sqlText, &parsed.options)
	wrote = wrote || sqlWrote
	if err != nil {
		return err
	}

	if parsed.database != ":memory:" && !parsed.options.readonly && wrote {
		data, err := serdes.Serialize(conn, "main")
		if err != nil {
			return exitf(inv, "sqlite3: %v", err)
		}
		if err := writeFileContents(ctx, inv, dbPath, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func parseSQLiteArgs(inv *commands.Invocation) (parsed sqliteParsedArgs, err error) {
	parsed.options = sqliteOptions{
		mode:      sqliteModeList,
		header:    false,
		separator: "|",
		newline:   "\n",
		nullValue: "",
	}

	args := inv.Args
	endOfOptions := false
	for len(args) > 0 {
		arg := args[0]
		args = args[1:]

		if endOfOptions {
			switch {
			case parsed.database == "":
				parsed.database = arg
			case parsed.sqlText == "":
				parsed.sqlText = arg
			default:
				return sqliteParsedArgs{}, exitf(inv, "sqlite3: unexpected extra argument %q", arg)
			}
			continue
		}

		switch arg {
		case "--":
			endOfOptions = true
		case "--help", "-help":
			parsed.showHelp = true
		case "-version":
			parsed.showVersion = true
		case "-list":
			parsed.options.mode = sqliteModeList
		case "-csv":
			parsed.options.mode = sqliteModeCSV
		case "-json":
			parsed.options.mode = sqliteModeJSON
		case "-line":
			parsed.options.mode = sqliteModeLine
		case "-column":
			parsed.options.mode = sqliteModeColumn
		case "-table":
			parsed.options.mode = sqliteModeTable
		case "-markdown":
			parsed.options.mode = sqliteModeMarkdown
		case "-tabs":
			parsed.options.mode = sqliteModeTabs
		case "-box":
			parsed.options.mode = sqliteModeBox
		case "-quote":
			parsed.options.mode = sqliteModeQuote
		case "-html":
			parsed.options.mode = sqliteModeHTML
		case "-ascii":
			parsed.options.mode = sqliteModeASCII
		case "-header":
			parsed.options.header = true
		case "-noheader":
			parsed.options.header = false
		case "-readonly":
			parsed.options.readonly = true
		case "-bail":
			parsed.options.bail = true
		case "-echo":
			parsed.options.echo = true
		case "-separator":
			parsed.options.separator, args, err = sqliteNextArg(inv, arg, args)
			if err != nil {
				return sqliteParsedArgs{}, err
			}
		case "-newline":
			parsed.options.newline, args, err = sqliteNextArg(inv, arg, args)
			if err != nil {
				return sqliteParsedArgs{}, err
			}
		case "-nullvalue":
			parsed.options.nullValue, args, err = sqliteNextArg(inv, arg, args)
			if err != nil {
				return sqliteParsedArgs{}, err
			}
		case "-cmd":
			parsed.options.cmd, args, err = sqliteNextArg(inv, arg, args)
			if err != nil {
				return sqliteParsedArgs{}, err
			}
		default:
			if strings.HasPrefix(arg, "-") {
				optName := arg
				if strings.HasPrefix(arg, "--") {
					optName = arg[1:]
				}
				return sqliteParsedArgs{}, exitf(inv, "sqlite3: unknown option: %s\nUse -help for a list of options.", optName)
			}
			switch {
			case parsed.database == "":
				parsed.database = arg
			case parsed.sqlText == "":
				parsed.sqlText = arg
			default:
				return sqliteParsedArgs{}, exitf(inv, "sqlite3: unexpected extra argument %q", arg)
			}
		}
	}

	return parsed, nil
}

func sqliteNextArg(inv *commands.Invocation, flag string, args []string) (value string, rest []string, err error) {
	if len(args) == 0 {
		return "", nil, exitf(inv, "sqlite3: missing argument to %s", flag)
	}
	return args[0], args[1:], nil
}

func resolveSQLiteDatabasePath(ctx context.Context, inv *commands.Invocation, database string) (absPath string, exists bool, err error) {
	if database == ":memory:" {
		return database, false, nil
	}
	info, abs, exists, err := statMaybe(ctx, inv, database)
	if err != nil {
		return "", false, err
	}
	if exists && info.IsDir() { //nolint:nilaway // exists guards info non-nil
		return "", false, exitf(inv, "sqlite3: %s: Is a directory", database)
	}
	return abs, exists, nil
}

func readSQLiteDatabase(ctx context.Context, inv *commands.Invocation, name string) ([]byte, error) {
	file, _, err := openRead(ctx, inv, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	data, err := commands.ReadAll(ctx, inv, file)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func exitf(inv *commands.Invocation, format string, args ...any) error {
	return commands.Exitf(inv, 1, format, args...)
}

func statMaybe(ctx context.Context, inv *commands.Invocation, name string) (info stdfs.FileInfo, abs string, exists bool, err error) {
	abs = name
	if inv != nil && inv.FS != nil {
		abs = inv.FS.Resolve(name)
	}
	info, err = inv.FS.Stat(ctx, abs)
	if err != nil {
		if errors.Is(err, stdfs.ErrNotExist) {
			return nil, abs, false, nil
		}
		return nil, "", false, err
	}
	return info, abs, true, nil
}

func openRead(ctx context.Context, inv *commands.Invocation, name string) (gbfs.File, string, error) {
	abs := name
	if inv != nil && inv.FS != nil {
		abs = inv.FS.Resolve(name)
	}
	file, err := inv.FS.Open(ctx, abs)
	if err != nil {
		return nil, "", err
	}
	return file, abs, nil
}

func ensureParentDirExists(ctx context.Context, inv *commands.Invocation, targetAbs string) error {
	parent := path.Dir(targetAbs)
	info, _, exists, err := statMaybe(ctx, inv, parent)
	if err != nil {
		return err
	}
	if !exists {
		return &commands.ExitError{
			Code: 1,
			Err:  fmt.Errorf("%s: No such file or directory", parent),
		}
	}
	if !info.IsDir() {
		return &commands.ExitError{
			Code: 1,
			Err:  fmt.Errorf("%s: Not a directory", parent),
		}
	}
	return nil
}

func writeFileContents(ctx context.Context, inv *commands.Invocation, targetAbs string, data []byte, perm stdfs.FileMode) error {
	if err := ensureParentDirExists(ctx, inv, targetAbs); err != nil {
		return err
	}

	file, err := inv.FS.OpenFile(ctx, targetAbs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return &commands.ExitError{Code: 1, Err: err}
	}
	defer func() { _ = file.Close() }()

	if _, err := file.Write(data); err != nil {
		return &commands.ExitError{Code: 1, Err: err}
	}
	return nil
}

func configureSQLiteConn(conn *gosqlite.Conn, readonly bool) error {
	if _, err := conn.Config(gosqlite.DBCONFIG_ENABLE_LOAD_EXTENSION, false); err != nil {
		return err
	}
	if _, err := conn.Config(gosqlite.DBCONFIG_DEFENSIVE, true); err != nil {
		return err
	}
	if _, err := conn.Config(gosqlite.DBCONFIG_TRUSTED_SCHEMA, false); err != nil {
		return err
	}
	conn.Limit(gosqlite.LIMIT_ATTACHED, 0)
	conn.HardHeapLimit(sqliteHardHeapLimitBytes)
	// Keep temp tables, indices, and sorter spills in the bounded SQLite heap;
	// the sandbox VFS refuses any file that would land on the host.
	if err := conn.Exec(`PRAGMA temp_store=MEMORY`); err != nil {
		return err
	}

	if err := conn.SetAuthorizer(func(action gosqlite.AuthorizerActionCode, name3rd, name4th, _, _ string) gosqlite.AuthorizerReturnCode {
		switch action {
		case gosqlite.AUTH_ATTACH, gosqlite.AUTH_DETACH, gosqlite.AUTH_CREATE_VTABLE, gosqlite.AUTH_DROP_VTABLE:
			return gosqlite.AUTH_DENY
		case gosqlite.AUTH_FUNCTION:
			if strings.EqualFold(name4th, "load_extension") {
				return gosqlite.AUTH_DENY
			}
		case gosqlite.AUTH_PRAGMA:
			if sqliteBlockedPragma(name3rd) {
				return gosqlite.AUTH_DENY
			}
			if strings.EqualFold(name3rd, "temp_store") && name4th != "" {
				return gosqlite.AUTH_DENY
			}
		}
		if readonly && sqliteWriteAction(action) {
			return gosqlite.AUTH_DENY
		}
		return gosqlite.AUTH_OK
	}); err != nil {
		return err
	}

	if readonly {
		if err := conn.Exec(`PRAGMA query_only=on`); err != nil {
			return err
		}
	}
	return nil
}

func sqliteBlockedPragma(name string) bool {
	switch strings.ToLower(name) {
	case "writable_schema", "temp_store_directory", "data_store_directory":
		return true
	default:
		return false
	}
}

func sqliteWriteAction(action gosqlite.AuthorizerActionCode) bool {
	switch action {
	case gosqlite.AUTH_CREATE_INDEX,
		gosqlite.AUTH_CREATE_TABLE,
		gosqlite.AUTH_CREATE_TEMP_INDEX,
		gosqlite.AUTH_CREATE_TEMP_TABLE,
		gosqlite.AUTH_CREATE_TEMP_TRIGGER,
		gosqlite.AUTH_CREATE_TEMP_VIEW,
		gosqlite.AUTH_CREATE_TRIGGER,
		gosqlite.AUTH_CREATE_VIEW,
		gosqlite.AUTH_DELETE,
		gosqlite.AUTH_DROP_INDEX,
		gosqlite.AUTH_DROP_TABLE,
		gosqlite.AUTH_DROP_TEMP_INDEX,
		gosqlite.AUTH_DROP_TEMP_TABLE,
		gosqlite.AUTH_DROP_TEMP_TRIGGER,
		gosqlite.AUTH_DROP_TEMP_VIEW,
		gosqlite.AUTH_DROP_TRIGGER,
		gosqlite.AUTH_DROP_VIEW,
		gosqlite.AUTH_INSERT,
		gosqlite.AUTH_UPDATE,
		gosqlite.AUTH_ALTER_TABLE,
		gosqlite.AUTH_REINDEX,
		gosqlite.AUTH_ANALYZE:
		return true
	default:
		return false
	}
}

func runSQLiteBatch(ctx context.Context, inv *commands.Invocation, conn *gosqlite.Conn, sqlText string, opts *sqliteOptions) (bool, error) {
	remaining := sqlText
	var wrote bool
	var hadSQLError bool
	for {
		if strings.TrimSpace(remaining) == "" {
			if hadSQLError {
				return wrote, &commands.ExitError{Code: 1, Err: errors.New("sqlite3: one or more SQL statements failed")}
			}
			return wrote, nil
		}

		currentStmtText := sqliteFirstStatementText(remaining)
		if reason := sqliteForbiddenStatement(currentStmtText); reason != "" {
			return wrote, exitf(inv, "sqlite3: %s", reason)
		}

		stmt, tail, err := conn.Prepare(remaining)
		if err != nil {
			return wrote, exitf(inv, "sqlite3: %v", err)
		}
		if stmt == nil {
			if tail == remaining {
				return wrote, nil
			}
			remaining = tail
			continue
		}

		sqlForStmt := strings.TrimSpace(stmt.SQL())
		if reason := sqliteForbiddenStatement(sqlForStmt); reason != "" {
			_ = stmt.Close()
			return wrote, exitf(inv, "sqlite3: %s", reason)
		}
		if opts.echo {
			_, _ = io.WriteString(inv.Stdout, sqlForStmt)
			if !strings.HasSuffix(sqlForStmt, "\n") {
				_, _ = io.WriteString(inv.Stdout, "\n")
			}
		}

		writeStmt, err := executeSQLiteStatement(ctx, inv, stmt, opts)
		_ = stmt.Close()
		if writeStmt {
			wrote = true
		}
		if err != nil {
			var sqlErr *sqliteSQLContinueError
			if errors.As(err, &sqlErr) {
				hadSQLError = true
				if opts.bail {
					return wrote, &commands.ExitError{Code: 1, Err: sqlErr.Err}
				}
				remaining = tail
				continue
			}
			return wrote, err
		}
		remaining = tail
	}
}

func executeSQLiteStatement(ctx context.Context, inv *commands.Invocation, stmt *gosqlite.Stmt, opts *sqliteOptions) (bool, error) {
	columns := stmt.ColumnCount()
	var out sqliteRowWriter
	if columns > 0 {
		names := make([]string, columns)
		for i := range columns {
			names[i] = stmt.ColumnName(i)
		}
		out = newSQLiteRowWriter(inv, opts, names)
	}

	for stmt.Step() {
		if out == nil {
			continue
		}
		row := make([]any, columns)
		for i := range columns {
			row[i] = sqliteColumnValue(stmt, i)
		}
		if err := out.Row(row); err != nil {
			return false, sqliteOutputError(inv, err)
		}
	}
	if err := stmt.Err(); err != nil {
		if out != nil {
			_ = out.Abort()
		}
		if ctx.Err() != nil {
			return false, sqliteContextError(ctx, inv)
		}
		sqlErr := fmt.Errorf("sqlite3: %w", err)
		if inv.Stderr != nil {
			_, _ = fmt.Fprintln(inv.Stderr, sqlErr.Error())
		}
		return false, &sqliteSQLContinueError{Err: sqlErr}
	}
	if out != nil {
		if err := out.Close(); err != nil {
			return false, sqliteOutputError(inv, err)
		}
	}

	return !stmt.ReadOnly(), nil
}

func sqliteColumnValue(stmt *gosqlite.Stmt, col int) any {
	switch stmt.ColumnType(col) {
	case gosqlite.NULL:
		return nil
	case gosqlite.INTEGER:
		return stmt.ColumnInt64(col)
	case gosqlite.FLOAT:
		return stmt.ColumnFloat(col)
	case gosqlite.BLOB:
		return string(stmt.ColumnBlob(col, nil))
	default:
		return stmt.ColumnText(col)
	}
}

func formatSQLiteColumn(opts *sqliteOptions, result *sqliteStatementResult, table bool) []byte {
	if len(result.Rows) == 0 && (!opts.header || len(result.Columns) == 0) {
		return nil
	}

	showHeader := opts.header
	widths := make([]int, len(result.Columns))
	for i, name := range result.Columns {
		widths[i] = len(name)
	}
	for _, row := range result.Rows {
		for i, value := range row {
			text := sqliteStringValue(value, opts.nullValue)
			if len(text) > widths[i] {
				widths[i] = len(text)
			}
		}
	}

	var buf bytes.Buffer
	if table {
		writeSQLiteBorder(&buf, widths, '-')
	}
	if showHeader {
		writeSQLiteRow(&buf, result.Columns, widths, table)
		if table {
			writeSQLiteBorder(&buf, widths, '=')
		} else {
			for i, width := range widths {
				if i > 0 {
					buf.WriteByte(' ')
				}
				buf.WriteString(strings.Repeat("-", width))
			}
			buf.WriteByte('\n')
		}
	}
	for _, row := range result.Rows {
		record := make([]string, len(row))
		for i, value := range row {
			record[i] = sqliteStringValue(value, opts.nullValue)
		}
		writeSQLiteRow(&buf, record, widths, table)
	}
	if table {
		writeSQLiteBorder(&buf, widths, '-')
	}
	return buf.Bytes()
}

func formatSQLiteBox(opts *sqliteOptions, result *sqliteStatementResult) []byte {
	if len(result.Columns) == 0 {
		return nil
	}

	widths := make([]int, len(result.Columns))
	for i, name := range result.Columns {
		widths[i] = len(name)
	}
	for _, row := range result.Rows {
		for i, value := range row {
			text := sqliteStringValue(value, opts.nullValue)
			if len(text) > widths[i] {
				widths[i] = len(text)
			}
		}
	}

	var buf bytes.Buffer
	writeSQLiteUnicodeBorder(&buf, widths, "┌", "┬", "┐")
	writeSQLiteUnicodeRow(&buf, result.Columns, widths)
	writeSQLiteUnicodeBorder(&buf, widths, "├", "┼", "┤")
	for _, row := range result.Rows {
		record := make([]string, len(row))
		for i, value := range row {
			record[i] = sqliteStringValue(value, opts.nullValue)
		}
		writeSQLiteUnicodeRow(&buf, record, widths)
	}
	writeSQLiteUnicodeBorder(&buf, widths, "└", "┴", "┘")
	return buf.Bytes()
}

func writeSQLiteRow(buf *bytes.Buffer, values []string, widths []int, table bool) {
	if table {
		buf.WriteString("| ")
	}
	for i, value := range values {
		if i > 0 {
			if table {
				buf.WriteString(" | ")
			} else {
				buf.WriteByte(' ')
			}
		}
		buf.WriteString(value)
		if widths[i] > len(value) {
			buf.WriteString(strings.Repeat(" ", widths[i]-len(value)))
		}
	}
	if table {
		buf.WriteString(" |")
	}
	buf.WriteByte('\n')
}

func writeSQLiteBorder(buf *bytes.Buffer, widths []int, fill byte) {
	buf.WriteByte('+')
	for _, width := range widths {
		buf.WriteString(strings.Repeat(string(fill), width+2))
		buf.WriteByte('+')
	}
	buf.WriteByte('\n')
}

func writeSQLiteUnicodeRow(buf *bytes.Buffer, values []string, widths []int) {
	buf.WriteString("│ ")
	for i, value := range values {
		if i > 0 {
			buf.WriteString(" │ ")
		}
		buf.WriteString(value)
		if widths[i] > len(value) {
			buf.WriteString(strings.Repeat(" ", widths[i]-len(value)))
		}
	}
	buf.WriteString(" │\n")
}

func writeSQLiteUnicodeBorder(buf *bytes.Buffer, widths []int, left, middle, right string) {
	buf.WriteString(left)
	for i, width := range widths {
		if i > 0 {
			buf.WriteString(middle)
		}
		buf.WriteString(strings.Repeat("─", width+2))
	}
	buf.WriteString(right)
	buf.WriteByte('\n')
}

func sqliteStringValue(value any, nullValue string) string {
	switch v := value.(type) {
	case nil:
		return nullValue
	case string:
		return v
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return sqliteFloatString(v)
	default:
		return fmt.Sprint(v)
	}
}

func sqliteQuoteString(value string) string {
	return "'" + value + "'"
}

func sqliteQuoteValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "NULL"
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return sqliteFloatString(v)
	default:
		return sqliteQuoteString(fmt.Sprint(v))
	}
}

func sqliteFloatString(value float64) string {
	return strconv.FormatFloat(value, 'g', 17, 64)
}

func sqliteEscapeHTML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return replacer.Replace(value)
}

func sqliteForbiddenStatement(sql string) string {
	first := sqliteFirstKeyword(sql)
	switch first {
	case "attach":
		return "ATTACH is disabled inside the sqlite3 sandbox"
	case "detach":
		return "DETACH is disabled inside the sqlite3 sandbox"
	case "vacuum":
		return "VACUUM is disabled inside the sqlite3 sandbox"
	}
	if sqliteContainsKeyword(sql, "load_extension") {
		return "load_extension() is disabled inside the sqlite3 sandbox"
	}
	return ""
}

func sqliteFirstKeyword(sql string) string {
	i := 0
	for i < len(sql) {
		switch {
		case unicode.IsSpace(rune(sql[i])):
			i++
		case strings.HasPrefix(sql[i:], "--"):
			i += 2
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
		case strings.HasPrefix(sql[i:], "/*"):
			i += 2
			for i+1 < len(sql) && sql[i:i+2] != "*/" {
				i++
			}
			if i+1 < len(sql) {
				i += 2
			}
		default:
			start := i
			for i < len(sql) {
				r := rune(sql[i])
				if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
					i++
					continue
				}
				break
			}
			if start == i {
				return ""
			}
			return strings.ToLower(sql[start:i])
		}
	}
	return ""
}

func sqliteFirstStatementText(sql string) string {
	i := 0
	for i < len(sql) {
		switch {
		case strings.HasPrefix(sql[i:], "--"):
			i += 2
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
		case strings.HasPrefix(sql[i:], "/*"):
			i += 2
			for i+1 < len(sql) && sql[i:i+2] != "*/" {
				i++
			}
			if i+1 < len(sql) {
				i += 2
			}
		case sql[i] == '\'' || sql[i] == '"' || sql[i] == '`':
			quote := sql[i]
			i++
			for i < len(sql) {
				if sql[i] == quote {
					if quote == '\'' && i+1 < len(sql) && sql[i+1] == quote {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
		case sql[i] == '[':
			i++
			for i < len(sql) && sql[i] != ']' {
				i++
			}
			if i < len(sql) {
				i++
			}
		case sql[i] == ';':
			return sql[:i]
		default:
			i++
		}
	}
	return sql
}

func sqliteContainsKeyword(sql, want string) bool {
	want = strings.ToLower(want)
	i := 0
	for i < len(sql) {
		switch {
		case unicode.IsSpace(rune(sql[i])):
			i++
		case strings.HasPrefix(sql[i:], "--"):
			i += 2
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
		case strings.HasPrefix(sql[i:], "/*"):
			i += 2
			for i+1 < len(sql) && sql[i:i+2] != "*/" {
				i++
			}
			if i+1 < len(sql) {
				i += 2
			}
		case sql[i] == '\'' || sql[i] == '"' || sql[i] == '`':
			quote := sql[i]
			i++
			for i < len(sql) {
				if sql[i] == quote {
					if quote == '\'' && i+1 < len(sql) && sql[i+1] == quote {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
		case sql[i] == '[':
			i++
			for i < len(sql) && sql[i] != ']' {
				i++
			}
			if i < len(sql) {
				i++
			}
		default:
			start := i
			for i < len(sql) {
				r := rune(sql[i])
				if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
					i++
					continue
				}
				break
			}
			if start == i {
				i++
				continue
			}
			if strings.EqualFold(sql[start:i], want) {
				return true
			}
		}
	}
	return false
}

func sqliteVersion(ctx context.Context) (string, error) {
	conn, err := gosqlite.OpenContext(ctx, ":memory:")
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()

	stmt, _, err := conn.Prepare(`SELECT sqlite_version()`)
	if err != nil {
		return "", err
	}
	defer func() { _ = stmt.Close() }()

	if !stmt.Step() {
		if err := stmt.Err(); err != nil {
			return "", err
		}
		return "", errors.New("sqlite3: unable to determine SQLite version")
	}
	return stmt.ColumnText(0), nil
}

const sqliteHelpText = `sqlite3 - SQLite database CLI inside the gbash sandbox

Usage:
  sqlite3 [OPTIONS] DATABASE [SQL]

Supported options:
  -list                 output in list mode (default)
  -csv                  output in CSV mode
  -json                 output in JSON mode
  -line                 output in line mode
  -column               output in aligned columns
  -table                output as an ASCII table
  -header               show column headers
  -noheader             hide column headers
  -separator SEP        set the list-mode field separator (default: |)
  -newline SEP          set the list-mode row separator (default: \n)
  -nullvalue TEXT       text for NULL values
  -readonly             reject writes and skip writeback
  -bail                 stop on the first SQL error
  -echo                 print each SQL statement before execution
  -cmd SQL              run SQL before the main SQL input
  -version              show SQLite version
  -help, --help         show this help
  --                    end option parsing

Notes:
  - DATABASE may be :memory: or a sandbox filesystem path.
  - If SQL is omitted, sqlite3 reads SQL from stdin.
  - Database files are loaded from and written back to the sandbox filesystem.
  - ATTACH, DETACH, VACUUM, virtual-table creation, and load_extension() are disabled.
`

var _ commands.Command = (*SQLite3)(nil)
