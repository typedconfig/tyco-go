package tyco

import (
	"strconv"
	"strings"
)

// ValueType identifies the specific Tyco value variant.
type ValueType int

const (
	ValueNull ValueType = iota
	ValueBool
	ValueInt
	ValueFloat
	ValueString
	ValueDate
	ValueTime
	ValueDateTime
	ValueArray
	ValueInstance
	ValueReference
)

// TycoString stores metadata about parsed strings (templates vs literals).
type TycoString struct {
	Value       string
	HasTemplate bool
	IsLiteral   bool
}

// TycoReference captures a primary-key reference to another struct.
type TycoReference struct {
	StructName string
	PrimaryKey string
	Resolved   *TycoInstance
}

// TycoInstance models a struct invocation with ordered attributes.
type TycoInstance struct {
	StructName string
	fields     map[string]*Value
	fieldOrder []string
}

// Value wraps all supported Tyco literal kinds.
type Value struct {
	Type      ValueType
	Bool      bool
	Int       int64
	Float     float64
	String    *TycoString
	Array     []*Value
	Instance  *TycoInstance
	Reference *TycoReference
}

// NewInstance creates an empty instance for the provided struct name.
func NewInstance(structName string) *TycoInstance {
	return &TycoInstance{
		StructName: structName,
		fields:     make(map[string]*Value),
		fieldOrder: make([]string, 0),
	}
}

// Clone performs a deep copy of the instance.
func (inst *TycoInstance) Clone() *TycoInstance {
	if inst == nil {
		return nil
	}
	clone := &TycoInstance{
		StructName: inst.StructName,
		fields:     make(map[string]*Value, len(inst.fields)),
		fieldOrder: append([]string(nil), inst.fieldOrder...),
	}
	for key, val := range inst.fields {
		clone.fields[key] = val.Clone()
	}
	return clone
}

// SetAttribute stores/updates a field while maintaining order.
func (inst *TycoInstance) SetAttribute(name string, value *Value) {
	if inst.fields == nil {
		inst.fields = make(map[string]*Value)
	}
	if _, exists := inst.fields[name]; !exists {
		inst.fieldOrder = append(inst.fieldOrder, name)
	}
	inst.fields[name] = value
}

// GetAttribute reads a field value if present.
func (inst *TycoInstance) GetAttribute(name string) *Value {
	return inst.fields[name]
}

// RemoveAttribute deletes the attribute and returns the previous value.
func (inst *TycoInstance) RemoveAttribute(name string) *Value {
	value, ok := inst.fields[name]
	if !ok {
		return nil
	}
	delete(inst.fields, name)
	for idx, key := range inst.fieldOrder {
		if key == name {
			inst.fieldOrder = append(inst.fieldOrder[:idx], inst.fieldOrder[idx+1:]...)
			break
		}
	}
	return value
}

// FieldOrder returns a copy of the stored key order.
func (inst *TycoInstance) FieldOrder() []string {
	return append([]string(nil), inst.fieldOrder...)
}

// Attributes exposes the backing map for mutation.
func (inst *TycoInstance) Attributes() map[string]*Value {
	return inst.fields
}

// EnforceOrderFromSchema reorders fields to match schema definition first.
func (inst *TycoInstance) EnforceOrderFromSchema(fields []*FieldSchema) {
	ordered := make([]string, 0, len(inst.fieldOrder))
	seen := make(map[string]struct{})
	for _, field := range fields {
		if _, ok := inst.fields[field.Name]; ok {
			ordered = append(ordered, field.Name)
			seen[field.Name] = struct{}{}
		}
	}
	for _, key := range inst.fieldOrder {
		if _, ok := seen[key]; ok {
			continue
		}
		ordered = append(ordered, key)
	}
	inst.fieldOrder = ordered
}

// ToJSONObject converts the instance to a JSON-serialisable structure.
func (inst *TycoInstance) ToJSONObject() map[string]any {
	result := make(map[string]any, len(inst.fields))
	for _, key := range inst.fieldOrder {
		if value, ok := inst.fields[key]; ok {
			result[key] = value.ToJSONValue()
		}
	}
	return result
}

// Clone duplicates the value and all nested content.
func (v *Value) Clone() *Value {
	if v == nil {
		return nil
	}
	clone := *v
	switch v.Type {
	case ValueString:
		if v.String != nil {
			sCopy := *v.String
			clone.String = &sCopy
		}
	case ValueArray:
		if v.Array != nil {
			clone.Array = make([]*Value, len(v.Array))
			for idx, item := range v.Array {
				clone.Array[idx] = item.Clone()
			}
		}
	case ValueInstance:
		clone.Instance = v.Instance.Clone()
	case ValueReference:
		if v.Reference != nil {
			refCopy := *v.Reference
			if v.Reference.Resolved != nil {
				refCopy.Resolved = v.Reference.Resolved.Clone()
			}
			clone.Reference = &refCopy
		}
	}
	return &clone
}

// Equal compares two values for semantic equality.
func (v *Value) Equal(other *Value) bool {
	if v == nil || other == nil {
		return v == other
	}
	if v.Type != other.Type {
		return false
	}
	switch v.Type {
	case ValueNull:
		return true
	case ValueBool:
		return v.Bool == other.Bool
	case ValueInt:
		return v.Int == other.Int
	case ValueFloat:
		return v.Float == other.Float
	case ValueString, ValueDate, ValueTime, ValueDateTime:
		if v.String == nil || other.String == nil {
			return v.String == other.String
		}
		return v.String.Value == other.String.Value
	default:
		return v.ToTemplateText() == other.ToTemplateText()
	}
}

// ToTemplateText renders the textual representation used during templates.
func (v *Value) ToTemplateText() string {
	switch v.Type {
	case ValueNull:
		return "null"
	case ValueBool:
		if v.Bool {
			return "true"
		}
		return "false"
	case ValueInt:
		return formatInt(v.Int)
	case ValueFloat:
		return formatFloat(v.Float)
	case ValueString:
		if v.String != nil {
			return v.String.Value
		}
	case ValueDate, ValueTime, ValueDateTime:
		if v.String != nil {
			return v.String.Value
		}
	case ValueReference:
		if v.Reference != nil {
			return v.Reference.PrimaryKey
		}
	}
	return ""
}

// RenderTemplates resolves template placeholders recursively.
func (v *Value) RenderTemplates(ctx *TycoContext, current *TycoInstance) {
	switch v.Type {
	case ValueString:
		if v.String != nil {
			v.String.Render(ctx, current)
		}
	case ValueArray:
		for _, item := range v.Array {
			item.RenderTemplates(ctx, current)
		}
	case ValueInstance:
		if v.Instance == nil {
			return
		}
		keys := v.Instance.FieldOrder()
		snapshot := v.Instance.Clone()
		for _, key := range keys {
			if field := v.Instance.fields[key]; field != nil {
				field.RenderTemplates(ctx, snapshot)
			}
			snapshot = v.Instance.Clone()
		}
	}
}

// ToJSONValue materialises a Go representation ready for encoding/json.
func (v *Value) ToJSONValue() any {
	switch v.Type {
	case ValueNull:
		return nil
	case ValueBool:
		return v.Bool
	case ValueInt:
		return v.Int
	case ValueFloat:
		return v.Float
	case ValueString, ValueDate, ValueTime, ValueDateTime:
		if v.String != nil {
			return v.String.Value
		}
		return ""
	case ValueArray:
		result := make([]any, 0, len(v.Array))
		for _, value := range v.Array {
			result = append(result, value.ToJSONValue())
		}
		return result
	case ValueInstance:
		if v.Instance == nil {
			return nil
		}
		return v.Instance.ToJSONObject()
	case ValueReference:
		if v.Reference != nil && v.Reference.Resolved != nil {
			return v.Reference.Resolved.ToJSONObject()
		}
		return nil
	default:
		return nil
	}
}

// Render performs template substitution for Tyco strings.
func (s *TycoString) Render(ctx *TycoContext, current *TycoInstance) {
	if s == nil || !s.HasTemplate || s.IsLiteral {
		return
	}
	var builder strings.Builder
	runes := []rune(s.Value)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if ch != '{' {
			builder.WriteRune(ch)
			continue
		}
		j := i + 1
		var placeholder strings.Builder
		for j < len(runes) && runes[j] != '}' {
			placeholder.WriteRune(runes[j])
			j++
		}
		if j < len(runes) && runes[j] == '}' {
			if resolved, ok := resolvePlaceholder(placeholder.String(), ctx, current); ok {
				builder.WriteString(resolved)
			} else {
				builder.WriteRune('{')
				builder.WriteString(placeholder.String())
				builder.WriteRune('}')
			}
			i = j
		} else {
			builder.WriteRune(ch)
		}
	}
	result := builder.String()
	if decoded, err := unescapeBasicString(result); err == nil {
		s.Value = decoded
	} else {
		s.Value = result
	}
	s.HasTemplate = false
}

func resolvePlaceholder(path string, ctx *TycoContext, current *TycoInstance) (string, bool) {
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	segments := strings.Split(path, ".")
	queue := append([]string{}, segments...)
	if len(queue) == 0 {
		return "", false
	}
	originalFirst := queue[0]

	var obj any
	if strings.HasPrefix(path, "global.") {
		obj = ctx.Globals()
		queue = queue[1:]
	} else if current != nil {
		obj = current
	} else {
		obj = ctx.Globals()
	}

	fetch := func(target any, attr string) (any, bool) {
		switch t := target.(type) {
		case map[string]*Value:
			val, ok := t[attr]
			if ok && val != nil {
				return val, true
			}
		case *TycoInstance:
			if val := t.GetAttribute(attr); val != nil {
				return val, true
			}
		case *TycoReference:
			if t != nil && t.Resolved != nil {
				if val := t.Resolved.GetAttribute(attr); val != nil {
					return val, true
				}
			}
		case *Value:
			switch t.Type {
			case ValueInstance:
				if t.Instance != nil {
					if val := t.Instance.GetAttribute(attr); val != nil {
						return val, true
					}
				}
			case ValueReference:
				if t.Reference != nil && t.Reference.Resolved != nil {
					if val := t.Reference.Resolved.GetAttribute(attr); val != nil {
						return val, true
					}
				}
			}
		}
		return nil, false
	}

	for len(queue) > 0 {
		attr := queue[0]
		if next, ok := fetch(obj, attr); ok {
			obj = next
			queue = queue[1:]
			continue
		}
		if len(queue) > 1 {
			merged := queue[0] + "." + queue[1]
			queue = append([]string{merged}, queue[2:]...)
			continue
		}
		if attr == "global" && originalFirst == "global" {
			obj = ctx.Globals()
			queue = queue[1:]
			continue
		}
		return "", false
	}

	val, ok := obj.(*Value)
	if !ok || val == nil {
		return "", false
	}
	return val.ToTemplateText(), true
}

// formatInt avoids pulling fmt for frequently-called template conversions.
func formatInt(value int64) string {
	return strconvFormatInt(value, 10)
}

func formatFloat(value float64) string {
	return strconvFormatFloat(value, 'f', -1, 64)
}

// Dedicated helpers so we can stub during testing if needed.
var (
	strconvFormatInt   = func(i int64, base int) string { return strconv.FormatInt(i, base) }
	strconvFormatFloat = func(f float64, fmt byte, prec, bitSize int) string {
		return strconv.FormatFloat(f, fmt, prec, bitSize)
	}
)
