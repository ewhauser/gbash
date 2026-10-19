package sqlite3

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ewhauser/gbash/commands"
)

// sqliteDefaultBufferedResultBytes bounds the rows that the width-aligned
// output modes hold in memory when the policy sets no stdout or file limit.
const sqliteDefaultBufferedResultBytes = 64 << 20

// sqliteBufferedValueOverhead approximates the per-value bookkeeping cost of
// a buffered row so that results made of many tiny values still hit the cap.
const sqliteBufferedValueOverhead = 16

// sqliteRowWriter receives the rows of one statement that returns columns.
type sqliteRowWriter interface {
	Row(values []any) error
	// Close finishes the statement's output.
	Close() error
	// Abort finishes output after the statement failed part way through.
	Abort() error
}

type sqliteResultTooLargeError struct {
	mode  sqliteOutputMode
	limit int64
}

func (e *sqliteResultTooLargeError) Error() string {
	return fmt.Sprintf("sqlite3: result too large for %s mode (exceeds %d bytes); use a streaming mode such as -list or add a LIMIT", e.mode, e.limit)
}

func newSQLiteRowWriter(inv *commands.Invocation, opts *sqliteOptions, columns []string) sqliteRowWriter {
	switch opts.mode {
	case sqliteModeColumn, sqliteModeTable, sqliteModeBox:
		return &sqliteBufferedWriter{
			stdout: inv.Stdout,
			opts:   opts,
			result: sqliteStatementResult{Columns: columns},
			limit:  sqliteBufferedResultLimit(inv),
		}
	default:
		w := &sqliteStreamWriter{
			stdout:  inv.Stdout,
			opts:    opts,
			columns: columns,
		}
		w.writeHeader()
		return w
	}
}

func sqliteBufferedResultLimit(inv *commands.Invocation) int64 {
	limit := max(inv.Limits.MaxStdoutBytes, inv.Limits.MaxFileBytes)
	if limit <= 0 {
		return sqliteDefaultBufferedResultBytes
	}
	return limit
}

func sqliteOutputError(inv *commands.Invocation, err error) error {
	var tooLarge *sqliteResultTooLargeError
	if errors.As(err, &tooLarge) {
		return exitf(inv, "%s", tooLarge.Error())
	}
	return &commands.ExitError{Code: 1, Err: err}
}

// sqliteBufferedWriter collects rows for modes whose column widths depend on
// every row, refusing to hold more than limit bytes of values.
type sqliteBufferedWriter struct {
	stdout io.Writer
	opts   *sqliteOptions
	result sqliteStatementResult
	size   int64
	limit  int64
}

func (w *sqliteBufferedWriter) Row(values []any) error {
	for _, value := range values {
		w.size += sqliteBufferedValueOverhead
		switch v := value.(type) {
		case string:
			w.size += int64(len(v))
		default:
			w.size += 8
		}
	}
	if w.size > w.limit {
		return &sqliteResultTooLargeError{mode: w.opts.mode, limit: w.limit}
	}
	w.result.Rows = append(w.result.Rows, values)
	return nil
}

func (w *sqliteBufferedWriter) Close() error {
	var formatted []byte
	switch w.opts.mode {
	case sqliteModeTable:
		formatted = formatSQLiteColumn(w.opts, &w.result, true)
	case sqliteModeBox:
		formatted = formatSQLiteBox(w.opts, &w.result)
	default:
		formatted = formatSQLiteColumn(w.opts, &w.result, false)
	}
	if len(formatted) == 0 {
		return nil
	}
	_, err := w.stdout.Write(formatted)
	return err
}

func (w *sqliteBufferedWriter) Abort() error {
	return nil
}

// sqliteStreamWriter writes each row as soon as SQLite produces it.
type sqliteStreamWriter struct {
	stdout  io.Writer
	out     bytes.Buffer
	opts    *sqliteOptions
	columns []string
	csv     *csv.Writer
	rows    int
}

func (w *sqliteStreamWriter) showHeader() bool {
	return w.opts.header && len(w.columns) > 0
}

func (w *sqliteStreamWriter) writeHeader() {
	opts := w.opts
	switch opts.mode {
	case sqliteModeCSV:
		w.csv = csv.NewWriter(&w.out)
		if w.showHeader() {
			_ = w.csv.Write(w.columns)
		}
	case sqliteModeMarkdown:
		if w.showHeader() {
			w.out.WriteString("| ")
			w.out.WriteString(strings.Join(w.columns, " | "))
			w.out.WriteString(" |\n|")
			for i := range w.columns {
				if i > 0 {
					w.out.WriteByte('|')
				}
				w.out.WriteString("---")
			}
			w.out.WriteString("|\n")
		}
	case sqliteModeTabs:
		if w.showHeader() {
			w.out.WriteString(strings.Join(w.columns, "\t"))
			w.out.WriteString(opts.newline)
		}
	case sqliteModeQuote:
		if w.showHeader() {
			values := make([]string, len(w.columns))
			for i, name := range w.columns {
				values[i] = sqliteQuoteString(name)
			}
			w.out.WriteString(strings.Join(values, ","))
			w.out.WriteString(opts.newline)
		}
	case sqliteModeHTML:
		if w.showHeader() {
			w.out.WriteString("<TR>")
			for _, name := range w.columns {
				w.out.WriteString("<TH>")
				w.out.WriteString(sqliteEscapeHTML(name))
				w.out.WriteString("</TH>")
			}
			w.out.WriteString("\n</TR>\n")
		}
	case sqliteModeASCII:
		if w.showHeader() {
			w.out.WriteString(strings.Join(w.columns, sqliteASCIIColSep))
			w.out.WriteString(sqliteASCIIRowSep)
		}
	case sqliteModeJSON, sqliteModeLine:
	default:
		if w.showHeader() {
			w.out.WriteString(strings.Join(w.columns, opts.separator))
			w.out.WriteString(opts.newline)
		}
	}
}

const (
	sqliteASCIIColSep = "\x1f"
	sqliteASCIIRowSep = "\x1e"
)

func (w *sqliteStreamWriter) stringValues(values []any) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = sqliteStringValue(value, w.opts.nullValue)
	}
	return out
}

func (w *sqliteStreamWriter) Row(values []any) error {
	opts := w.opts
	first := w.rows == 0
	w.rows++
	switch opts.mode {
	case sqliteModeCSV:
		if err := w.csv.Write(w.stringValues(values)); err != nil {
			return err
		}
	case sqliteModeJSON:
		item := make(map[string]any, len(w.columns))
		for i, name := range w.columns {
			item[name] = values[i]
		}
		data, err := json.Marshal(item)
		if err != nil {
			return err
		}
		if first {
			w.out.WriteByte('[')
		} else {
			w.out.WriteByte(',')
		}
		w.out.Write(data)
	case sqliteModeLine:
		width := 0
		for _, name := range w.columns {
			width = max(width, len(name))
		}
		if !first {
			w.out.WriteByte('\n')
		}
		for i, name := range w.columns {
			w.out.WriteString(name)
			if width > len(name) {
				w.out.WriteString(strings.Repeat(" ", width-len(name)))
			}
			w.out.WriteString(" = ")
			w.out.WriteString(sqliteStringValue(values[i], opts.nullValue))
			w.out.WriteByte('\n')
		}
	case sqliteModeMarkdown:
		w.out.WriteString("| ")
		w.out.WriteString(strings.Join(w.stringValues(values), " | "))
		w.out.WriteString(" |\n")
	case sqliteModeTabs:
		w.out.WriteString(strings.Join(w.stringValues(values), "\t"))
		w.out.WriteString(opts.newline)
	case sqliteModeQuote:
		quoted := make([]string, len(values))
		for i, value := range values {
			quoted[i] = sqliteQuoteValue(value)
		}
		w.out.WriteString(strings.Join(quoted, ","))
		w.out.WriteString(opts.newline)
	case sqliteModeHTML:
		w.out.WriteString("<TR>")
		for _, value := range values {
			w.out.WriteString("<TD>")
			w.out.WriteString(sqliteEscapeHTML(sqliteStringValue(value, opts.nullValue)))
			w.out.WriteString("</TD>")
		}
		w.out.WriteString("\n</TR>\n")
	case sqliteModeASCII:
		w.out.WriteString(strings.Join(w.stringValues(values), sqliteASCIIColSep))
		w.out.WriteString(sqliteASCIIRowSep)
	default:
		w.out.WriteString(strings.Join(w.stringValues(values), opts.separator))
		w.out.WriteString(opts.newline)
	}
	return w.flushIfFull()
}

// sqliteStreamFlushBytes is how much formatted output accumulates before it
// is written to stdout.
const sqliteStreamFlushBytes = 4 << 10

// flushIfFull surfaces stdout write errors promptly so a closed pipe stops
// the query instead of letting it run on into a discarded buffer.
func (w *sqliteStreamWriter) flushIfFull() error {
	if w.csv != nil {
		w.csv.Flush()
		if err := w.csv.Error(); err != nil {
			return err
		}
	}
	if w.out.Len() >= sqliteStreamFlushBytes {
		return w.flush()
	}
	return nil
}

func (w *sqliteStreamWriter) flush() error {
	if w.out.Len() == 0 {
		return nil
	}
	_, err := w.stdout.Write(w.out.Bytes())
	w.out.Reset()
	return err
}

func (w *sqliteStreamWriter) Close() error {
	if w.opts.mode == sqliteModeJSON && w.rows > 0 {
		w.out.WriteString("]\n")
	}
	if w.csv != nil {
		w.csv.Flush()
		if err := w.csv.Error(); err != nil {
			return err
		}
	}
	return w.flush()
}

func (w *sqliteStreamWriter) Abort() error {
	return w.Close()
}
