package yq

import (
	"container/list"
	"context"
	"fmt"
	"math"
	"reflect"

	"github.com/mikefarah/yq/v4/pkg/yqlib"
)

// yqMinEvalBudgetBytes is the smallest estimated size any single operator
// result may reach before evaluation is aborted.
const yqMinEvalBudgetBytes = 256 << 20

// yqEvalBudgetInputFactor scales the budget with the estimated size of the
// input documents so that large but legitimate inputs keep working.
const yqEvalBudgetInputFactor = 4

// yqNodeWeightBytes is the estimated cost of one result node. A
// CandidateNode is a couple of hundred bytes, but yqlib copies nodes freely
// while evaluating; peak heap measured about 1.3KiB per live result node.
const yqNodeWeightBytes = 1 << 10

// yqIdentityComparedOpTypes are operation types that yqlib compares by
// pointer while evaluating, so their nodes must keep the shared global type.
var yqIdentityComparedOpTypes = map[string]bool{
	"BLOCK":           true,
	"UNION":           true,
	"ASSIGN_VARIABLE": true,
	"CREATE_MAP":      true,
	"FIRST":           true,
	"TRAVERSE_ARRAY":  true,
}

type yqEvalBudgetError struct {
	limit int64
}

func (e *yqEvalBudgetError) Error() string {
	return fmt.Sprintf("evaluation exceeded the estimated %d byte size limit", e.limit)
}

// yqEvalGuard makes one yqlib evaluation cancellable and bounded. yqlib has
// no context support, so the guard wraps the handler of every operator in a
// parsed expression: each call first checks ctx, and each result is sized
// against the budget. The first failure is sticky so that operators which
// swallow errors from their operands cannot hide it.
type yqEvalGuard struct {
	ctx      context.Context //nolint:containedctx // intentional: yqlib handlers take no ctx, so wrapped handlers check it here
	maxBytes int64
	depth    int
	err      error
}

func newYQEvalGuard(ctx context.Context) *yqEvalGuard {
	return &yqEvalGuard{ctx: ctx, maxBytes: yqMinEvalBudgetBytes}
}

// parse parses expression and instruments the resulting tree. The tree must
// only be evaluated under this guard.
func (g *yqEvalGuard) parse(expression string) (*yqlib.ExpressionNode, error) {
	node, err := yqlib.ExpressionParser.ParseExpression(expression)
	if err != nil {
		return nil, err
	}
	g.instrument(node)
	return node, nil
}

func (g *yqEvalGuard) instrument(node *yqlib.ExpressionNode) {
	for ; node != nil; node = node.RHS {
		g.instrument(node.LHS)
		op := node.Operation
		if op == nil || op.OperationType == nil || op.OperationType.Handler == nil {
			continue
		}
		if yqIdentityComparedOpTypes[op.OperationType.Type] {
			continue
		}
		// Operations are allocated per parse, but their types are shared
		// globals; give this node its own copy with a guarded handler.
		opType := *op.OperationType
		handler := reflect.ValueOf(&opType.Handler).Elem()
		handler.Set(g.wrapHandler(handler))
		op.OperationType = &opType
	}
}

var yqContextType = reflect.TypeFor[yqlib.Context]()

func (g *yqEvalGuard) wrapHandler(handler reflect.Value) reflect.Value {
	original := reflect.ValueOf(handler.Interface())
	fail := func(err error) []reflect.Value {
		return []reflect.Value{reflect.Zero(yqContextType), reflect.ValueOf(&err).Elem()}
	}
	return reflect.MakeFunc(handler.Type(), func(args []reflect.Value) []reflect.Value {
		if err := g.check(); err != nil {
			return fail(err)
		}
		if g.depth == 0 {
			// A top-level call receives the input documents.
			if input, ok := args[1].Interface().(yqlib.Context); ok {
				g.maxBytes = max(yqMinEvalBudgetBytes, yqEvalBudgetInputFactor*g.size(input.MatchingNodes, math.MaxInt64))
			}
		}
		g.depth++
		results := original.Call(args)
		g.depth--
		if !results[1].IsNil() {
			return results
		}
		if result, ok := results[0].Interface().(yqlib.Context); ok {
			if g.size(result.MatchingNodes, g.maxBytes) > g.maxBytes {
				g.err = &yqEvalBudgetError{limit: g.maxBytes}
				return fail(g.err)
			}
		}
		return results
	})
}

// check reports a previous failure or the context error.
func (g *yqEvalGuard) check() error {
	if g.err != nil {
		return g.err
	}
	if err := g.ctx.Err(); err != nil {
		g.err = err
	}
	return g.err
}

// finish returns the sticky failure, if any, after an evaluation completes.
func (g *yqEvalGuard) finish(err error) error {
	if g.err != nil {
		return g.err
	}
	return err
}

// size estimates the size of an operator result, stopping once it exceeds
// limit. One result is walked as a tree, so shared subtrees count once per
// occurrence (as they would when printed); multiple results are deduplicated
// because they commonly overlap, as with recursive descent.
func (g *yqEvalGuard) size(nodes *list.List, limit int64) int64 {
	if nodes == nil || nodes.Len() == 0 {
		return 0
	}
	var seen map[*yqlib.CandidateNode]struct{}
	if nodes.Len() > 1 {
		seen = make(map[*yqlib.CandidateNode]struct{})
	}
	var total int64
	stack := make([]*yqlib.CandidateNode, 0, 16)
	for el := nodes.Front(); el != nil; el = el.Next() {
		node, ok := el.Value.(*yqlib.CandidateNode)
		if !ok || node == nil {
			continue
		}
		stack = append(stack[:0], node)
		for len(stack) > 0 {
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen != nil {
				if _, dup := seen[node]; dup {
					continue
				}
				seen[node] = struct{}{}
			}
			total += yqNodeWeightBytes + int64(len(node.Value))
			if total > limit {
				return total
			}
			stack = append(stack, node.Content...)
		}
	}
	return total
}
