package builtins

import (
	"context"
	"encoding/base32"
	"strings"
)

type Base32 struct{}

func NewBase32() *Base32 {
	return &Base32{}
}

func (c *Base32) Name() string {
	return "base32"
}

func (c *Base32) Run(ctx context.Context, inv *Invocation) error {
	return RunCommand(ctx, c, inv)
}

func (c *Base32) Spec() CommandSpec {
	return baseEncodingCommandSpec(
		"base32",
		"encode/decode data and print to standard output\nWith no FILE, or when FILE is -, read standard input.\n\nThe data are encoded as described for the base32 alphabet in RFC 4648.\nWhen decoding, the input may contain newlines in addition\nto the bytes of the formal base32 alphabet. Use --ignore-garbage\nto attempt to recover from any other non-alphabet bytes in the\nencoded stream.",
	)
}

func (c *Base32) RunParsed(ctx context.Context, inv *Invocation, matches *ParsedCommand) error {
	decode := matches.Has("decode")
	ignoreGarbage := matches.Has("ignore-garbage")
	wrap := 76
	if matches.Has("wrap") {
		value, err := parseBaseEncWrap(c.Name(), matches.Value("wrap"), inv)
		if err != nil {
			return err
		}
		wrap = value
	}

	data, err := readSingleBaseEncInput(ctx, inv, c.Name(), matches.Positionals())
	if err != nil {
		return err
	}

	if decode {
		decoded, err := decodeBase32Data(data, ignoreGarbage)
		if err != nil {
			return exitf(inv, 1, "base32: invalid input")
		}
		if _, err := inv.Stdout.Write(decoded); err != nil {
			return &ExitError{Code: 1, Err: err}
		}
		return nil
	}

	encoded := base32.StdEncoding.EncodeToString(data)
	if err := writeBaseEncOutput(inv.Stdout, encoded, wrap); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	return nil
}

func decodeBase32Data(data []byte, ignoreGarbage bool) ([]byte, error) {
	normalized := normalizeBase32Input(string(data), ignoreGarbage)
	if rem := len(normalized) % 8; rem != 0 {
		normalized += strings.Repeat("=", 8-rem)
	}
	return base32.StdEncoding.DecodeString(normalized)
}

func normalizeBase32Input(input string, ignoreGarbage bool) string {
	var b strings.Builder
	for _, r := range input {
		switch {
		case r == ' ' || r == '\n' || r == '\r' || r == '\t':
			continue
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - 'a' + 'A')
		case (r >= 'A' && r <= 'Z') || (r >= '2' && r <= '7') || r == '=':
			b.WriteRune(r)
		case ignoreGarbage:
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

var _ Command = (*Base32)(nil)
var _ SpecProvider = (*Base32)(nil)
var _ ParsedRunner = (*Base32)(nil)
