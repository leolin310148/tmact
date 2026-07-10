package workflow

import (
	"fmt"
	"reflect"
)

func EvaluateCondition(c *Condition, state State) (bool, error) {
	if c == nil {
		return true, nil
	}
	if len(c.All) > 0 {
		for i := range c.All {
			ok, err := EvaluateCondition(&c.All[i], state)
			if err != nil || !ok {
				return ok, err
			}
		}
		return true, nil
	}
	if len(c.Any) > 0 {
		for i := range c.Any {
			ok, err := EvaluateCondition(&c.Any[i], state)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	if c.Not != nil {
		ok, err := EvaluateCondition(c.Not, state)
		return !ok, err
	}
	if c.Stage != nil {
		s, ok := state.Stages[c.Stage.ID]
		if !ok {
			return false, fmt.Errorf("stage %q not found", c.Stage.ID)
		}
		if c.Stage.Status != "" && s.Status != c.Stage.Status {
			return false, nil
		}
		if c.Stage.Outcome != "" && s.Outcome != c.Stage.Outcome {
			return false, nil
		}
		return true, nil
	}
	if c.Revision != nil {
		current := state.Revisions[c.Revision.Name]
		if c.Revision.Stage != "" {
			return current == state.Stages[c.Revision.Stage].BoundRevisions[c.Revision.Name], nil
		}
		return current == c.Revision.Equals, nil
	}
	if c.Evidence != nil {
		s := state.Stages[c.Evidence.Stage]
		return s.Evidence != nil && s.Evidence.Result == c.Evidence.Result, nil
	}
	if c.Variable != nil {
		actual := state.Variables[c.Variable.Name]
		return compare(actual, c.Variable.Value, c.Variable.Op), nil
	}
	return false, nil
}
func compare(a, b any, op string) bool {
	if op == "" {
		op = "eq"
	}
	if op == "eq" {
		return reflect.DeepEqual(normalizeNumber(a), normalizeNumber(b)) || fmt.Sprint(a) == fmt.Sprint(b)
	}
	if op == "ne" {
		return !compare(a, b, "eq")
	}
	af, aok := number(a)
	bf, bok := number(b)
	if !aok || !bok {
		return false
	}
	switch op {
	case "lt":
		return af < bf
	case "lte":
		return af <= bf
	case "gt":
		return af > bf
	case "gte":
		return af >= bf
	}
	return false
}
func number(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	case float32:
		return float64(x), true
	}
	return 0, false
}
func normalizeNumber(v any) any {
	if n, ok := number(v); ok {
		return n
	}
	return v
}
