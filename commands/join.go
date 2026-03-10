package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type Join struct{}

type joinOptions struct {
	field1       int
	field2       int
	delimiter    string
	include      [3]bool
	onlyUnpaired [3]bool
	empty        string
	output       []joinOutputField
	ignoreCase   bool
}

type joinRecord struct {
	line   string
	fields []string
	key    string
}

type joinOutputField struct {
	file  int
	field int
	join  bool
}

func NewJoin() *Join {
	return &Join{}
}

func (c *Join) Name() string {
	return "join"
}

func (c *Join) Run(ctx context.Context, inv *Invocation) error {
	opts, leftName, rightName, err := parseJoinArgs(inv)
	if err != nil {
		return err
	}

	leftData, rightData, err := readTwoInputs(ctx, inv, leftName, rightName)
	if err != nil {
		return err
	}
	leftRecords := parseJoinRecords(textLines(leftData), opts.field1, opts.delimiter, opts.ignoreCase)
	rightRecords := parseJoinRecords(textLines(rightData), opts.field2, opts.delimiter, opts.ignoreCase)

	rightByKey := make(map[string][]int)
	for i, record := range rightRecords {
		rightByKey[record.key] = append(rightByKey[record.key], i)
	}
	rightMatched := make([]bool, len(rightRecords))

	for _, left := range leftRecords {
		matches := rightByKey[left.key]
		if len(matches) == 0 {
			if opts.include[1] || opts.onlyUnpaired[1] {
				if err := writeJoinLine(inv, &opts, &left, nil); err != nil {
					return err
				}
			}
			continue
		}
		for _, index := range matches {
			rightMatched[index] = true
			if opts.onlyUnpaired[1] || opts.onlyUnpaired[2] {
				continue
			}
			current := rightRecords[index]
			if err := writeJoinLine(inv, &opts, &left, &current); err != nil {
				return err
			}
		}
	}

	if opts.include[2] || opts.onlyUnpaired[2] {
		for index, matched := range rightMatched {
			if matched {
				continue
			}
			current := rightRecords[index]
			if err := writeJoinLine(inv, &opts, nil, &current); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseJoinArgs(inv *Invocation) (opts joinOptions, leftName, rightName string, err error) {
	args := inv.Args
	opts = joinOptions{
		field1:    1,
		field2:    1,
		delimiter: "",
	}

	for len(args) > 0 {
		arg := args[0]
		if arg == "--" {
			args = args[1:]
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}
		switch {
		case arg == "-1":
			value, rest, err := parseJoinInt(inv, "1", args[1:])
			if err != nil {
				return joinOptions{}, "", "", err
			}
			opts.field1 = value
			args = rest
			continue
		case strings.HasPrefix(arg, "-1") && len(arg) > 2:
			value, err := strconv.Atoi(arg[2:])
			if err != nil || value <= 0 {
				return joinOptions{}, "", "", exitf(inv, 1, "join: invalid field %q", arg[2:])
			}
			opts.field1 = value
		case arg == "-2":
			value, rest, err := parseJoinInt(inv, "2", args[1:])
			if err != nil {
				return joinOptions{}, "", "", err
			}
			opts.field2 = value
			args = rest
			continue
		case strings.HasPrefix(arg, "-2") && len(arg) > 2:
			value, err := strconv.Atoi(arg[2:])
			if err != nil || value <= 0 {
				return joinOptions{}, "", "", exitf(inv, 1, "join: invalid field %q", arg[2:])
			}
			opts.field2 = value
		case arg == "-t":
			if len(args) < 2 {
				return joinOptions{}, "", "", exitf(inv, 1, "join: option requires an argument -- 't'")
			}
			opts.delimiter = args[1]
			args = args[2:]
			continue
		case strings.HasPrefix(arg, "-t") && len(arg) > 2:
			opts.delimiter = arg[2:]
		case arg == "-e":
			if len(args) < 2 {
				return joinOptions{}, "", "", exitf(inv, 1, "join: option requires an argument -- 'e'")
			}
			opts.empty = args[1]
			args = args[2:]
			continue
		case strings.HasPrefix(arg, "-e") && len(arg) > 2:
			opts.empty = arg[2:]
		case arg == "-o":
			if len(args) < 2 {
				return joinOptions{}, "", "", exitf(inv, 1, "join: option requires an argument -- 'o'")
			}
			output, err := parseJoinOutput(args[1])
			if err != nil {
				return joinOptions{}, "", "", err
			}
			opts.output = output
			args = args[2:]
			continue
		case strings.HasPrefix(arg, "-o") && len(arg) > 2:
			output, err := parseJoinOutput(arg[2:])
			if err != nil {
				return joinOptions{}, "", "", err
			}
			opts.output = output
		case arg == "-i":
			opts.ignoreCase = true
		case arg == "-a":
			value, rest, err := parseJoinInt(inv, "a", args[1:])
			if err != nil {
				return joinOptions{}, "", "", err
			}
			if value != 1 && value != 2 {
				return joinOptions{}, "", "", exitf(inv, 1, "join: invalid file number %d", value)
			}
			opts.include[value] = true
			args = rest
			continue
		case strings.HasPrefix(arg, "-a") && len(arg) > 2:
			value, err := strconv.Atoi(arg[2:])
			if err != nil || (value != 1 && value != 2) {
				return joinOptions{}, "", "", exitf(inv, 1, "join: invalid file number %q", arg[2:])
			}
			opts.include[value] = true
		case arg == "-v":
			value, rest, err := parseJoinInt(inv, "v", args[1:])
			if err != nil {
				return joinOptions{}, "", "", err
			}
			if value != 1 && value != 2 {
				return joinOptions{}, "", "", exitf(inv, 1, "join: invalid file number %d", value)
			}
			opts.onlyUnpaired[value] = true
			args = rest
			continue
		case strings.HasPrefix(arg, "-v") && len(arg) > 2:
			value, err := strconv.Atoi(arg[2:])
			if err != nil || (value != 1 && value != 2) {
				return joinOptions{}, "", "", exitf(inv, 1, "join: invalid file number %q", arg[2:])
			}
			opts.onlyUnpaired[value] = true
		default:
			return joinOptions{}, "", "", exitf(inv, 1, "join: unsupported flag %s", arg)
		}
		args = args[1:]
	}

	if len(args) != 2 {
		return joinOptions{}, "", "", exitf(inv, 1, "join: expected exactly two input files")
	}
	return opts, args[0], args[1], nil
}

func parseJoinInt(inv *Invocation, flag string, args []string) (value int, rest []string, err error) {
	if len(args) == 0 {
		return 0, nil, exitf(inv, 1, "join: option requires an argument -- '%s'", flag)
	}
	value, err = strconv.Atoi(args[0])
	if err != nil {
		return 0, nil, exitf(inv, 1, "join: invalid numeric value %q", args[0])
	}
	return value, args[1:], nil
}

func parseJoinOutput(value string) ([]joinOutputField, error) {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || unicodeSpace(r)
	})
	fields := make([]joinOutputField, 0, len(parts))
	for _, part := range parts {
		if part == "0" {
			fields = append(fields, joinOutputField{join: true})
			continue
		}
		fileText, fieldText, ok := strings.Cut(part, ".")
		if !ok {
			return nil, fmt.Errorf("join: invalid output spec %q", part)
		}
		file, err := strconv.Atoi(fileText)
		if err != nil || (file != 1 && file != 2) {
			return nil, fmt.Errorf("join: invalid output spec %q", part)
		}
		field, err := strconv.Atoi(fieldText)
		if err != nil || field <= 0 {
			return nil, fmt.Errorf("join: invalid output spec %q", part)
		}
		fields = append(fields, joinOutputField{file: file, field: field})
	}
	return fields, nil
}

func parseJoinRecords(lines []string, fieldIndex int, delimiter string, ignoreCase bool) []joinRecord {
	records := make([]joinRecord, 0, len(lines))
	for _, line := range lines {
		fields := joinSplitFields(line, delimiter)
		key := joinFieldValue(fields, fieldIndex)
		if ignoreCase {
			key = strings.ToLower(key)
		}
		records = append(records, joinRecord{line: line, fields: fields, key: key})
	}
	return records
}

func joinSplitFields(line, delimiter string) []string {
	if delimiter == "" {
		return strings.Fields(line)
	}
	return strings.Split(line, delimiter)
}

func joinFieldValue(fields []string, index int) string {
	if index <= 0 || index > len(fields) {
		return ""
	}
	return fields[index-1]
}

func writeJoinLine(inv *Invocation, opts *joinOptions, left, right *joinRecord) error {
	fields := formatJoinFields(opts, left, right)
	sep := opts.delimiter
	if sep == "" {
		sep = " "
	}
	if _, err := fmt.Fprintln(inv.Stdout, strings.Join(fields, sep)); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	return nil
}

func formatJoinFields(opts *joinOptions, left, right *joinRecord) []string {
	if len(opts.output) > 0 {
		fields := make([]string, 0, len(opts.output))
		for _, spec := range opts.output {
			switch {
			case spec.join:
				fields = append(fields, joinJoinKey(left, right, opts))
			case spec.file == 1:
				fields = append(fields, joinRecordField(left, spec.field, opts.empty))
			case spec.file == 2:
				fields = append(fields, joinRecordField(right, spec.field, opts.empty))
			}
		}
		return fields
	}

	fields := []string{joinJoinKey(left, right, opts)}
	if left != nil {
		for i := 1; i <= len(left.fields); i++ {
			if i == opts.field1 {
				continue
			}
			fields = append(fields, left.fields[i-1])
		}
	}
	if right != nil {
		for i := 1; i <= len(right.fields); i++ {
			if i == opts.field2 {
				continue
			}
			fields = append(fields, right.fields[i-1])
		}
	}
	return replaceJoinEmpty(fields, opts.empty)
}

func joinJoinKey(left, right *joinRecord, opts *joinOptions) string {
	switch {
	case left != nil:
		return joinRecordField(left, opts.field1, opts.empty)
	case right != nil:
		return joinRecordField(right, opts.field2, opts.empty)
	default:
		return opts.empty
	}
}

func joinRecordField(record *joinRecord, field int, empty string) string {
	if record == nil || field <= 0 || field > len(record.fields) {
		return empty
	}
	value := record.fields[field-1]
	if value == "" {
		return empty
	}
	return value
}

func replaceJoinEmpty(fields []string, empty string) []string {
	if empty == "" {
		return fields
	}
	out := make([]string, len(fields))
	for i, field := range fields {
		if field == "" {
			out[i] = empty
			continue
		}
		out[i] = field
	}
	return out
}

func unicodeSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

var _ Command = (*Join)(nil)
