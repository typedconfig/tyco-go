package tyco

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// FieldSchema describes a declared struct field.
type FieldSchema struct {
	Name         string
	TypeName     string
	IsPrimaryKey bool
	IsNullable   bool
	IsArray      bool
	DefaultValue *Value
	EnumChoices  []*Value
}

// Clone duplicates the schema entry.
func (f *FieldSchema) Clone() *FieldSchema {
	if f == nil {
		return nil
	}
	clone := *f
	if f.DefaultValue != nil {
		clone.DefaultValue = f.DefaultValue.Clone()
	}
	if len(f.EnumChoices) > 0 {
		clone.EnumChoices = make([]*Value, len(f.EnumChoices))
		for idx, choice := range f.EnumChoices {
			clone.EnumChoices[idx] = choice.Clone()
		}
	}
	return &clone
}

// TycoStruct captures schema information plus parsed instances.
type TycoStruct struct {
	name            string
	fields          []*FieldSchema
	primaryKeyField string
	instances       []*TycoInstance
	primaryIndex    map[string]*TycoInstance
}

// NewTycoStruct initialises an empty schema definition.
func NewTycoStruct(name string) *TycoStruct {
	return &TycoStruct{
		name:         name,
		fields:       make([]*FieldSchema, 0),
		instances:    make([]*TycoInstance, 0),
		primaryIndex: make(map[string]*TycoInstance),
	}
}

// Clone deep-copies the struct definition.
func (s *TycoStruct) Clone() *TycoStruct {
	if s == nil {
		return nil
	}
	clone := &TycoStruct{
		name:            s.name,
		primaryKeyField: s.primaryKeyField,
		fields:          make([]*FieldSchema, len(s.fields)),
		instances:       make([]*TycoInstance, len(s.instances)),
		primaryIndex:    make(map[string]*TycoInstance, len(s.primaryIndex)),
	}
	for i, field := range s.fields {
		clone.fields[i] = field.Clone()
	}
	for i, inst := range s.instances {
		clone.instances[i] = inst.Clone()
	}
	for key, inst := range s.primaryIndex {
		clone.primaryIndex[key] = inst.Clone()
	}
	return clone
}

// Name returns the struct identifier.
func (s *TycoStruct) Name() string {
	return s.name
}

// Fields exposes the declared fields.
func (s *TycoStruct) Fields() []*FieldSchema {
	return s.fields
}

// Instances returns all parsed instances.
func (s *TycoStruct) Instances() []*TycoInstance {
	return s.instances
}

// PrimaryKeyField returns the configured PK field, if any.
func (s *TycoStruct) PrimaryKeyField() string {
	return s.primaryKeyField
}

// HasPrimaryKey reports whether the struct defines a primary key.
func (s *TycoStruct) HasPrimaryKey() bool {
	return s.primaryKeyField != ""
}

// AddField appends a field schema entry.
func (s *TycoStruct) AddField(field *FieldSchema) {
	if field.IsPrimaryKey {
		s.primaryKeyField = field.Name
	}
	s.fields = append(s.fields, field)
}

// AddInstance records a parsed instance.
func (s *TycoStruct) AddInstance(inst *TycoInstance) {
	s.instances = append(s.instances, inst)
}

// SetDefault updates the default for the named field.
func (s *TycoStruct) SetDefault(fieldName string, value *Value) error {
	for _, field := range s.fields {
		if field.Name == fieldName {
			if value == nil {
				if len(field.EnumChoices) > 0 {
					return fmt.Errorf("field %q previously set as enum; cannot clear with empty default", fieldName)
				}
				field.DefaultValue = nil
			} else {
				field.DefaultValue = value.Clone()
				field.EnumChoices = nil
			}
			return nil
		}
	}
	return fmt.Errorf("unknown field %q", fieldName)
}

// SetEnumChoices configures allowed values for a scalar field.
func (s *TycoStruct) SetEnumChoices(fieldName string, choices []*Value) error {
	for _, field := range s.fields {
		if field.Name == fieldName {
			field.EnumChoices = make([]*Value, len(choices))
			for idx, choice := range choices {
				field.EnumChoices[idx] = choice.Clone()
			}
			field.DefaultValue = nil
			return nil
		}
	}
	return fmt.Errorf("unknown field %q", fieldName)
}

func valueMatchesEnum(value *Value, choices []*Value) bool {
	for _, choice := range choices {
		if value.Equal(choice) {
			return true
		}
	}
	return false
}

func formatEnumChoices(choices []*Value) string {
	parts := make([]string, len(choices))
	for idx, choice := range choices {
		parts[idx] = choice.ToTemplateText()
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// BuildPrimaryIndex refreshes the lookup table used by references.
func (s *TycoStruct) BuildPrimaryIndex() {
	s.primaryIndex = make(map[string]*TycoInstance)
	if s.primaryKeyField == "" {
		return
	}
	for _, inst := range s.instances {
		if value := inst.GetAttribute(s.primaryKeyField); value != nil {
			s.primaryIndex[value.ToTemplateText()] = inst.Clone()
		}
	}
}

// FindByPrimaryKey locates an instance by its PK literal.
func (s *TycoStruct) FindByPrimaryKey(key string) *TycoInstance {
	return s.primaryIndex[key]
}

// TycoContext holds globals and struct definitions.
type TycoContext struct {
	globals map[string]*Value
	structs map[string]*TycoStruct
}

// NewContext creates an empty context.
func NewContext() *TycoContext {
	return &TycoContext{
		globals: make(map[string]*Value),
		structs: make(map[string]*TycoStruct),
	}
}

// Clone deep copies the context, including instances.
func (ctx *TycoContext) Clone() *TycoContext {
	copyCtx := NewContext()
	for key, value := range ctx.globals {
		copyCtx.globals[key] = value.Clone()
	}
	for name, structDef := range ctx.structs {
		copyCtx.structs[name] = structDef.Clone()
	}
	return copyCtx
}

// SetGlobal registers a global value.
func (ctx *TycoContext) SetGlobal(name string, value *Value) {
	ctx.globals[name] = value
}

// GetGlobal fetches a global attribute.
func (ctx *TycoContext) GetGlobal(name string) *Value {
	return ctx.globals[name]
}

// Globals exposes the global map.
func (ctx *TycoContext) Globals() map[string]*Value {
	return ctx.globals
}

// EnsureStruct retrieves or creates a struct definition.
func (ctx *TycoContext) EnsureStruct(name string) *TycoStruct {
	if def, ok := ctx.structs[name]; ok {
		return def
	}
	def := NewTycoStruct(name)
	ctx.structs[name] = def
	return def
}

// GetStruct returns a struct definition.
func (ctx *TycoContext) GetStruct(name string) *TycoStruct {
	return ctx.structs[name]
}

// Structs returns all struct definitions.
func (ctx *TycoContext) Structs() map[string]*TycoStruct {
	return ctx.structs
}

// Render finalises the context (defaults, references, templates).
func (ctx *TycoContext) Render() error {
	if err := ctx.resolveInlineInstances(); err != nil {
		return err
	}
	for _, structDef := range ctx.structs {
		structDef.BuildPrimaryIndex()
	}
	if err := ctx.resolveReferences(); err != nil {
		return err
	}
	ctx.renderTemplates()
	return nil
}

func (ctx *TycoContext) resolveInlineInstances() error {
	snapshot := ctx.cloneStructMap()

	coerce := func(value *Value, schema *FieldSchema) (*Value, error) {
		if value == nil || schema == nil || schema.IsArray {
			return value, nil
		}
		if value.Type != ValueString || value.String == nil {
			return value, nil
		}
		literal := value.String.Value
		switch schema.TypeName {
		case "int":
			num, err := parseInteger(literal)
			if err != nil {
				return nil, err
			}
			return &Value{Type: ValueInt, Int: num}, nil
		case "float":
			f, parseErr := strconv.ParseFloat(strings.TrimSpace(literal), 64)
			if parseErr != nil {
				return nil, newParseErrorf("Invalid float literal %q: %v", literal, parseErr)
			}
			return &Value{Type: ValueFloat, Float: f}, nil
		case "bool":
			val := literal == "true" || literal == "True"
			return &Value{Type: ValueBool, Bool: val}, nil
		default:
			return value, nil
		}
	}

	var resolveValue func(value *Value) error
	resolveValue = func(value *Value) error {
		if value == nil {
			return nil
		}
		switch value.Type {
		case ValueArray:
			for _, item := range value.Array {
				if err := resolveValue(item); err != nil {
					return err
				}
			}
		case ValueInstance:
			if value.Instance == nil {
				return nil
			}
			schema := snapshot[value.Instance.StructName]
			if schema == nil {
				return nil
			}
			if err := applySchema(value.Instance, schema, snapshot, coerce, resolveValue); err != nil {
				return err
			}
		}
		return nil
	}

	for key, value := range ctx.globals {
		if err := resolveValue(value); err != nil {
			return err
		}
		ctx.globals[key] = value
	}

	for name, structDef := range ctx.structs {
		schema := snapshot[name]
		if schema == nil {
			schema = structDef.Clone()
		}
		for _, inst := range structDef.instances {
			if err := applySchema(inst, schema, snapshot, coerce, resolveValue); err != nil {
				return err
			}
		}
	}

	return nil
}

type coercionFn func(value *Value, schema *FieldSchema) (*Value, error)

func applySchema(
	instance *TycoInstance,
	schema *TycoStruct,
	schemas map[string]*TycoStruct,
	coerce coercionFn,
	resolveValue func(value *Value) error,
) error {
	if instance == nil || schema == nil {
		return nil
	}

	type placeholder struct {
		index int
		name  string
	}

	placeholders := make([]placeholder, 0)
	for key := range instance.fields {
		if strings.HasPrefix(key, "_arg") {
			idx, err := strconv.Atoi(strings.TrimPrefix(key, "_arg"))
			if err == nil {
				placeholders = append(placeholders, placeholder{index: idx, name: key})
			}
		}
	}
	sort.Slice(placeholders, func(i, j int) bool {
		return placeholders[i].index < placeholders[j].index
	})

	for _, holder := range placeholders {
		if holder.index < len(schema.fields) {
			fieldSchema := schema.fields[holder.index]
			if value := instance.RemoveAttribute(holder.name); value != nil {
				coerced, err := coerce(value, fieldSchema)
				if err != nil {
					return err
				}
				instance.SetAttribute(fieldSchema.Name, coerced)
			}
		}
	}

	for _, field := range schema.fields {
		if value := instance.RemoveAttribute(field.Name); value != nil {
			coerced, err := coerce(value, field)
			if err != nil {
				return err
			}
			if len(field.EnumChoices) > 0 && !valueMatchesEnum(coerced, field.EnumChoices) {
				return newParseErrorf("Field %q enum value %q not in choices %s", field.Name, coerced.ToTemplateText(), formatEnumChoices(field.EnumChoices))
			}
			instance.SetAttribute(field.Name, coerced)
		} else if len(field.EnumChoices) > 0 {
			return newParseErrorf("Field %q enum value not set for struct %s", field.Name, schema.Name())
		} else if field.DefaultValue != nil {
			instance.SetAttribute(field.Name, field.DefaultValue.Clone())
		}
	}

	instance.EnforceOrderFromSchema(schema.fields)

	for _, value := range instance.fields {
		if value == nil {
			continue
		}
		if err := resolveValue(value); err != nil {
			return err
		}
	}

	return nil
}

func (ctx *TycoContext) resolveReferences() error {
	snapshot := ctx.cloneStructMap()

	var visit func(value *Value) error
	visit = func(value *Value) error {
		if value == nil {
			return nil
		}
		switch value.Type {
		case ValueReference:
			ref := value.Reference
			if ref == nil {
				return nil
			}
			structDef := snapshot[ref.StructName]
			if structDef == nil {
				return newUnknownStructError(ref.StructName)
			}
			instance := structDef.FindByPrimaryKey(ref.PrimaryKey)
			if instance == nil {
				return newReferenceError(fmt.Sprintf("Unknown %s(%s)", ref.StructName, ref.PrimaryKey))
			}
			ref.Resolved = instance.Clone()
		case ValueArray:
			for _, item := range value.Array {
				if err := visit(item); err != nil {
					return err
				}
			}
		case ValueInstance:
			if value.Instance == nil {
				return nil
			}
			for _, item := range value.Instance.fields {
				if err := visit(item); err != nil {
					return err
				}
			}
		}
		return nil
	}

	for _, value := range ctx.globals {
		if err := visit(value); err != nil {
			return err
		}
	}
	for _, structDef := range ctx.structs {
		for _, inst := range structDef.instances {
			for _, value := range inst.fields {
				if err := visit(value); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (ctx *TycoContext) renderTemplates() {
	snapshot := ctx.Clone()
	for key, value := range ctx.globals {
		if value != nil {
			value.RenderTemplates(snapshot, nil)
			snapshot.globals[key] = value.Clone()
		}
	}

	for name, structDef := range ctx.structs {
		shadow := snapshot.structs[name]
		for idx, inst := range structDef.instances {
			instanceSnapshot := inst.Clone()
			for _, fieldName := range inst.FieldOrder() {
				if value := inst.fields[fieldName]; value != nil {
					value.RenderTemplates(snapshot, instanceSnapshot)
					instanceSnapshot = inst.Clone()
				}
			}
			if shadow != nil {
				if idx < len(shadow.instances) {
					shadow.instances[idx] = inst.Clone()
				} else {
					shadow.instances = append(shadow.instances, inst.Clone())
				}
			}
		}
	}
}

// ToJSON materialises the Go representation used by tests/consumers.
func (ctx *TycoContext) ToJSON() map[string]any {
	result := make(map[string]any, len(ctx.globals)+len(ctx.structs))
	for key, value := range ctx.globals {
		result[key] = value.ToJSONValue()
	}
	for name, structDef := range ctx.structs {
		if !structDef.HasPrimaryKey() {
			continue
		}
		items := make([]any, 0, len(structDef.instances))
		for _, inst := range structDef.instances {
			items = append(items, inst.ToJSONObject())
		}
		result[name] = items
	}
	return result
}

// ToObject exposes the same aggregated representation as the Python binding.
func (ctx *TycoContext) ToObject() map[string]any {
	return ctx.ToJSON()
}

func (ctx *TycoContext) cloneStructMap() map[string]*TycoStruct {
	snapshot := make(map[string]*TycoStruct, len(ctx.structs))
	for name, structDef := range ctx.structs {
		snapshot[name] = structDef.Clone()
	}
	return snapshot
}
