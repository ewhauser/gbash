package shellstate

import "context"

type ShellVarLookup func(name string) (string, bool)

type shellVarLookupKey struct{}

func WithShellVarLookup(ctx context.Context, lookup ShellVarLookup) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if lookup == nil {
		return ctx
	}
	return context.WithValue(ctx, shellVarLookupKey{}, lookup)
}

func ShellVarLookupFromContext(ctx context.Context) ShellVarLookup {
	if ctx == nil {
		return nil
	}
	lookup, _ := ctx.Value(shellVarLookupKey{}).(ShellVarLookup)
	return lookup
}
