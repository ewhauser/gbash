package builtins

import (
	"context"
	"testing"
)

func TestParseLSTimeStylePosixPrefixRespectsLCAllPrecedence(t *testing.T) {
	t.Parallel()

	spec := NewLS().Spec()

	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "lc all overrides lc time",
			env: map[string]string{
				"LC_ALL":  "C.UTF-8",
				"LC_TIME": "POSIX",
			},
			want: "long-iso",
		},
		{
			name: "posix effective locale uses locale style",
			env: map[string]string{
				"LC_ALL":  "POSIX",
				"LC_TIME": "C.UTF-8",
			},
			want: "locale",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			inv := &Invocation{
				Args: []string{"-l", "--time-style=posix-long-iso"},
				Env:  tt.env,
			}
			matches, _, err := ParseCommandSpec(inv, &spec)
			if err != nil {
				t.Fatalf("ParseCommandSpec() error = %v", err)
			}

			got, err := parseLSTimeStyle(inv, matches)
			if err != nil {
				t.Fatalf("parseLSTimeStyle() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseLSTimeStyle() = %q, want %q", got, tt.want)
			}
		})
	}
}

//nolint:paralleltest // Mutates the package-global identity DB loader.
func TestPrimeLSIdentityDBCachesPerInvocation(t *testing.T) {
	t.Parallel()

	calls := 0

	opts := &lsOptions{
		longFormat: true,
		showOwner:  true,
		showGroup:  true,
		identityDBLoader: func(context.Context, *Invocation) *permissionIdentityDB {
			calls++
			return &permissionIdentityDB{}
		},
	}

	primeLSIdentityDB(context.Background(), &Invocation{}, opts)
	primeLSIdentityDB(context.Background(), &Invocation{}, opts)

	if calls != 1 {
		t.Fatalf("loader calls = %d, want 1", calls)
	}
	if opts.identityDB == nil {
		t.Fatal("identityDB = nil, want cached DB")
	}
}

func TestPrimeLSIdentityDBSkipsNumericIDs(t *testing.T) {
	t.Parallel()

	calls := 0

	opts := &lsOptions{
		longFormat: true,
		showOwner:  true,
		showGroup:  true,
		numericIDs: true,
		identityDBLoader: func(context.Context, *Invocation) *permissionIdentityDB {
			calls++
			return &permissionIdentityDB{}
		},
	}

	primeLSIdentityDB(context.Background(), &Invocation{}, opts)

	if calls != 0 {
		t.Fatalf("loader calls = %d, want 0", calls)
	}
	if opts.identityDB != nil {
		t.Fatalf("identityDB = %#v, want nil", opts.identityDB)
	}
}
