package cloudformation

import (
	"fmt"
	"strings"
)

// scope is the lookup table passed into [resolve]. It carries the
// values needed to resolve intrinsic functions:
//
//	Parameters    user-supplied parameter values (or Defaults)
//	Mappings      the !FindInMap table of the template
//	Pseudo        AWS pseudo-parameters (AWS::Region, AWS::AccountId, …)
//
// Resources are NOT in scope: a CFN `Ref: MyBucket` evaluates to the
// physical ID at deploy time, which c3x can't know. We emit a
// placeholder string for those references.
type scope struct {
	Parameters map[string]any
	Mappings   map[string]map[string]map[string]any
	Pseudo     map[string]any
}

// resolve walks the value tree, evaluating any CloudFormation
// intrinsic function it encounters. Plain values pass through
// unchanged.
//
// Supported intrinsics:
//
//	Ref            → Parameters / Pseudo lookup; unknown refs become
//	                 the literal string `<ref:Name>`.
//	Fn::Sub        → string interpolation with ${Name} placeholders
//	Fn::Join       → list-of-strings join
//	Fn::FindInMap  → mapping table lookup
//	Fn::GetAtt     → deferred to deploy-time; placeholder
//
// Unknown intrinsics return their input verbatim so c3x degrades
// instead of crashing on templates that use features we don't yet
// model.
func resolve(v any, sc scope) any {
	switch t := v.(type) {
	case map[string]any:
		if len(t) == 1 {
			for k, inner := range t {
				if out, ok := tryIntrinsic(k, inner, sc); ok {
					return out
				}
			}
		}
		out := make(map[string]any, len(t))
		for k, v := range t {
			out[k] = resolve(v, sc)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, v := range t {
			out[i] = resolve(v, sc)
		}
		return out
	default:
		return v
	}
}

func tryIntrinsic(key string, value any, sc scope) (any, bool) {
	switch key {
	case "Ref":
		return resolveRef(stringer(value), sc), true
	case "Fn::Sub":
		return resolveSub(value, sc), true
	case "Fn::Join":
		return resolveJoin(value, sc), true
	case "Fn::FindInMap":
		return resolveFindInMap(value, sc), true
	case "Fn::GetAtt":
		// We can't know runtime values; emit a placeholder so any
		// catalog expression that reads it as a string sees something
		// rather than a typed traversal.
		return fmt.Sprintf("<get-att:%v>", value), true
	}
	return nil, false
}

func resolveRef(name string, sc scope) any {
	if v, ok := sc.Parameters[name]; ok {
		return v
	}
	if v, ok := sc.Pseudo[name]; ok {
		return v
	}
	return fmt.Sprintf("<ref:%s>", name)
}

func resolveSub(v any, sc scope) any {
	switch t := v.(type) {
	case string:
		return subString(t, sc, nil)
	case []any:
		// `[template, vars]` form: extra var map after the template.
		if len(t) == 0 {
			return ""
		}
		tmpl, _ := t[0].(string)
		extras := map[string]any{}
		if len(t) > 1 {
			if m, ok := t[1].(map[string]any); ok {
				for k, v := range m {
					extras[k] = resolve(v, sc)
				}
			}
		}
		return subString(tmpl, sc, extras)
	}
	return v
}

// subString implements Fn::Sub's `${Name}` placeholder pass. Looks
// up extras first (the local var map), then Parameters, then Pseudo,
// finally falling back to a `<ref:Name>` placeholder so the user
// sees something locatable instead of an empty string.
func subString(s string, sc scope, extras map[string]any) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '$' && s[i+1] == '{' {
			end := strings.IndexByte(s[i+2:], '}')
			if end < 0 {
				b.WriteString(s[i:])
				break
			}
			name := s[i+2 : i+2+end]
			if v, ok := extras[name]; ok {
				b.WriteString(stringer(v))
			} else if v, ok := sc.Parameters[name]; ok {
				b.WriteString(stringer(v))
			} else if v, ok := sc.Pseudo[name]; ok {
				b.WriteString(stringer(v))
			} else {
				b.WriteString("<ref:")
				b.WriteString(name)
				b.WriteString(">")
			}
			i += 3 + end
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func resolveJoin(v any, sc scope) any {
	t, ok := v.([]any)
	if !ok || len(t) != 2 {
		return v
	}
	sep, _ := t[0].(string)
	items, ok := t[1].([]any)
	if !ok {
		return v
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, stringer(resolve(item, sc)))
	}
	return strings.Join(parts, sep)
}

func resolveFindInMap(v any, sc scope) any {
	t, ok := v.([]any)
	if !ok || len(t) != 3 {
		return v
	}
	mapName := stringer(resolve(t[0], sc))
	topKey := stringer(resolve(t[1], sc))
	subKey := stringer(resolve(t[2], sc))
	m, ok := sc.Mappings[mapName]
	if !ok {
		return v
	}
	row, ok := m[topKey]
	if !ok {
		return v
	}
	return row[subKey]
}

// stringer renders an `any` value as a string for interpolation.
// Unlike fmt.Sprint, it special-cases nil and pre-existing string
// values to avoid useless wrapping.
func stringer(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
