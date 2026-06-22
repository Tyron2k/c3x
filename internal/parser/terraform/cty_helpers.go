package terraform

import (
	"fmt"
	"math/big"

	"github.com/zclconf/go-cty/cty"
)

// ctyToAny converts a cty.Value into a generic Go value compatible
// with domain.Resource.Attributes. The conversion is recursive so
// nested objects and lists round-trip cleanly.
//
// Numbers come back as float64 because the calculator's expression
// layer (expr-lang) works in float64 internally; precision is
// preserved later by re-parsing as Decimal at line-item time.
//
// cty.NilType / cty.DynamicVal map to nil so the catalog's
// `default(missing, fallback)` idiom resolves cleanly downstream.
func ctyToAny(v cty.Value) any {
	if !v.IsKnown() || v.IsNull() {
		return nil
	}
	ty := v.Type()
	switch {
	case ty == cty.String:
		return v.AsString()
	case ty == cty.Bool:
		return v.True()
	case ty == cty.Number:
		f, _ := v.AsBigFloat().Float64()
		return f
	case ty.IsListType(), ty.IsSetType(), ty.IsTupleType():
		var out []any
		it := v.ElementIterator()
		for it.Next() {
			_, elem := it.Element()
			out = append(out, ctyToAny(elem))
		}
		return out
	case ty.IsMapType(), ty.IsObjectType():
		out := map[string]any{}
		it := v.ElementIterator()
		for it.Next() {
			k, elem := it.Element()
			out[k.AsString()] = ctyToAny(elem)
		}
		return out
	}
	return v.GoString()
}

// anyToCty converts a Go value (typically from a tfvars JSON file or a
// CLI --var literal) back into cty.Value. Used when populating the
// `var` scope from non-HCL sources.
func anyToCty(v any) cty.Value {
	switch t := v.(type) {
	case nil:
		return cty.NullVal(cty.DynamicPseudoType)
	case string:
		return cty.StringVal(t)
	case bool:
		return cty.BoolVal(t)
	case int:
		return cty.NumberIntVal(int64(t))
	case int64:
		return cty.NumberIntVal(t)
	case float64:
		return cty.NumberFloatVal(t)
	case *big.Float:
		return cty.NumberVal(t)
	case []any:
		if len(t) == 0 {
			return cty.ListValEmpty(cty.DynamicPseudoType)
		}
		elems := make([]cty.Value, 0, len(t))
		for _, e := range t {
			elems = append(elems, anyToCty(e))
		}
		// Use a tuple for heterogeneous lists so we don't have to unify
		// element types here.
		return cty.TupleVal(elems)
	case map[string]any:
		if len(t) == 0 {
			return cty.EmptyObjectVal
		}
		fields := make(map[string]cty.Value, len(t))
		for k, e := range t {
			fields[k] = anyToCty(e)
		}
		return cty.ObjectVal(fields)
	}
	return cty.StringVal(fmt.Sprint(v))
}
