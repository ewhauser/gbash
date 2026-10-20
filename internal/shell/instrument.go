package shell

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ewhauser/gbash/policy"
	"github.com/ewhauser/gbash/shell/syntax"
)

const loopIterCommandName = "__jb_loop_iter"

func instrumentLoopBudgets(program *syntax.File, pol policy.Policy, budget *executionBudget, loopNamespace string) error {
	if program == nil || pol == nil || budget == nil || pol.Limits().MaxLoopIterations <= 0 {
		return nil
	}

	var nextLoopID int
	for node := range syntax.All(program) {
		switch node := node.(type) {
		case *syntax.WhileClause:
			if budget.hasLoopGuard(node.Do) {
				continue
			}
			guard, err := budget.newLoopGuardStmt(loopKind(node), loopGuardID(loopNamespace, nextLoopID))
			if err != nil {
				return err
			}
			nextLoopID++
			node.Do = append([]*syntax.Stmt{guard}, node.Do...)
		case *syntax.ForClause:
			if budget.hasLoopGuard(node.Do) {
				continue
			}
			guard, err := budget.newLoopGuardStmt("for", loopGuardID(loopNamespace, nextLoopID))
			if err != nil {
				return err
			}
			nextLoopID++
			node.Do = append([]*syntax.Stmt{guard}, node.Do...)
		}
	}

	return nil
}

func loopGuardID(namespace string, id int) string {
	idText := strconv.Itoa(id)
	if strings.TrimSpace(namespace) == "" {
		return idText
	}
	return namespace + "-" + idText
}

func loopKind(clause *syntax.WhileClause) string {
	if clause != nil && clause.Until {
		return "until"
	}
	return "while"
}

// hasLoopGuard reports whether stmts already starts with a guard this budget
// inserted. Guards are recognized by node identity, so a user-written call to
// the helper name never counts as one.
func (b *executionBudget) hasLoopGuard(stmts []*syntax.Stmt) bool {
	if len(stmts) == 0 || stmts[0] == nil {
		return false
	}
	call := loopGuardCall(stmts[0])
	if call == nil {
		return false
	}
	_, ok := b.loopGuards.Load(call)
	return ok
}

func (b *executionBudget) newLoopGuardStmt(kind, id string) (*syntax.Stmt, error) {
	file, err := syntax.NewParser().Parse(strings.NewReader(fmt.Sprintf("%s %s %s || exit $?\n", loopIterCommandName, kind, id)), "loop-guard")
	if err != nil {
		return nil, err
	}
	if len(file.Stmts) != 1 {
		return nil, fmt.Errorf("unexpected loop guard statement count: %d", len(file.Stmts))
	}
	call := loopGuardCall(file.Stmts[0])
	if call == nil {
		return nil, fmt.Errorf("unexpected loop guard statement shape")
	}
	b.loopGuards.Store(call, loopGuard{kind: kind, id: id})
	return file.Stmts[0], nil
}

type loopGuard struct {
	kind string
	id   string
}

// loopGuardCall returns the guard call at the head of a guard statement.
func loopGuardCall(stmt *syntax.Stmt) *syntax.CallExpr {
	if stmt == nil {
		return nil
	}
	switch cmd := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		return cmd
	case *syntax.BinaryCmd:
		return loopGuardCall(cmd.X)
	default:
		return nil
	}
}

// reservedHelperPrefix marks names the runtime reserves for the commands it
// inserts. User code may not call them.
const reservedHelperPrefix = "__jb_"

func wordLiteral(word *syntax.Word) string {
	if word == nil {
		return ""
	}
	return word.Lit()
}
