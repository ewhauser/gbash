package commands

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

type Base64 struct{}

func NewBase64() *Base64 {
	return &Base64{}
}

func (c *Base64) Name() string {
	return "base64"
}

func (c *Base64) Run(ctx context.Context, inv *Invocation) error {
	args := inv.Args
	decode := false
	wrap := 76

	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch {
		case args[0] == "-d" || args[0] == "--decode":
			decode = true
			args = args[1:]
		case args[0] == "-w":
			if len(args) < 2 {
				return exitf(inv, 1, "base64: option requires an argument -- w")
			}
			value, err := strconv.Atoi(args[1])
			if err != nil || value < 0 {
				return exitf(inv, 1, "base64: invalid wrap size %q", args[1])
			}
			wrap = value
			args = args[2:]
		case strings.HasPrefix(args[0], "-w"):
			value, err := strconv.Atoi(strings.TrimPrefix(args[0], "-w"))
			if err != nil || value < 0 {
				return exitf(inv, 1, "base64: invalid wrap size %q", strings.TrimPrefix(args[0], "-w"))
			}
			wrap = value
			args = args[1:]
		default:
			return exitf(inv, 1, "base64: unsupported flag %s", args[0])
		}
	}

	inputs, err := readNamedInputs(ctx, inv, args, true)
	if err != nil {
		return err
	}
	var data []byte
	for _, input := range inputs {
		data = append(data, input.Data...)
	}

	if decode {
		decoded, err := base64.StdEncoding.DecodeString(stripWhitespace(string(data)))
		if err != nil {
			return exitf(inv, 1, "base64: invalid input")
		}
		if _, err := inv.Stdout.Write(decoded); err != nil {
			return &ExitError{Code: 1, Err: err}
		}
		return nil
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	if wrap == 0 {
		_, err := fmt.Fprintln(inv.Stdout, encoded)
		if err != nil {
			return &ExitError{Code: 1, Err: err}
		}
		return nil
	}
	for len(encoded) > wrap {
		if _, err := fmt.Fprintln(inv.Stdout, encoded[:wrap]); err != nil {
			return &ExitError{Code: 1, Err: err}
		}
		encoded = encoded[wrap:]
	}
	if _, err := fmt.Fprintln(inv.Stdout, encoded); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	return nil
}

func stripWhitespace(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

var _ Command = (*Base64)(nil)
