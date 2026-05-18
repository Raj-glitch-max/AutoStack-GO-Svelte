package workflow

import "strings"

// Constraint is a key/operator/value predicate evaluated against a context map.
// Used by conditional nodes to select traversal paths.
type Constraint struct {
	Field    string `json:"field"`
	Operator string `json:"operator"` // "eq", "neq", "gt", "lt", "contains", "prefix"
	Value    string `json:"value"`
}

// EvaluateConstraint checks one constraint against the provided context.
// Unknown operators return false.
func EvaluateConstraint(c Constraint, ctx map[string]string) bool {
	actual, ok := ctx[c.Field]
	if !ok {
		return false
	}
	switch c.Operator {
	case "eq":
		return actual == c.Value
	case "neq":
		return actual != c.Value
	case "contains":
		return strings.Contains(actual, c.Value)
	case "prefix":
		return strings.HasPrefix(actual, c.Value)
	case "suffix":
		return strings.HasSuffix(actual, c.Value)
	case "gt":
		return actual > c.Value // lexical — callers are responsible for normalization
	case "lt":
		return actual < c.Value
	case "gte":
		return actual >= c.Value
	case "lte":
		return actual <= c.Value
	default:
		return false
	}
}

// EvaluateConstraints returns true only when ALL constraints pass (AND semantics).
// An empty constraint list always returns true.
func EvaluateConstraints(cs []Constraint, ctx map[string]string) bool {
	for _, c := range cs {
		if !EvaluateConstraint(c, ctx) {
			return false
		}
	}
	return true
}

// CanTransitionTo returns true when at least one edge of the given type exists
// from the current active node in the graph.
func CanTransitionTo(graph WorkflowGraph, exec WorkflowExecution, edgeType EdgeType) bool {
	_, ok := NextNode(graph, exec.ActiveNodeID, edgeType)
	return ok
}
