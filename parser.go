package tyco

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type parseState int

const (
	stateTopLevel parseState = iota
	stateInStructSchema
	stateInStructInstances
)

const attrNamePattern = `[a-z_][A-Za-z0-9_]*(?:\.[A-Za-z0-9_]+)*`

var (
	structDefRe      = regexp.MustCompile(`^([A-Z][A-Za-z0-9_]*)\s*:$`)
	fieldRe          = regexp.MustCompile(`^\s*([*?])?([A-Za-z][A-Za-z0-9_]*)(\[\])?\s+(` + attrNamePattern + `)\s*:(?:\s+(.*))?$`)
	defaultUpdateRe  = regexp.MustCompile(`^\s+(` + attrNamePattern + `)\s*:(?:\s+(.*))?$`)
	structCallRe     = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_]*)\((.*)\)$`)
	whitespacePrefix = regexp.MustCompile(`^\s`)
)

// SourceLine tracks file/line metadata for diagnostics.
type SourceLine struct {
	Text       string
	Path       string
	LineNumber int
}

func (sl SourceLine) span() SourceSpan {
	return SourceSpan{
		Path:     sl.Path,
		Line:     sl.LineNumber,
		Column:   1,
		LineText: sl.Text,
	}
}

// TycoParser is the Go implementation of the canonical parser.
type TycoParser struct {
	included map[string]struct{}
}

// NewParser builds a parser instance.
func NewParser() *TycoParser {
	return &TycoParser{
		included: make(map[string]struct{}),
	}
}

// Load parses the provided file path into a context.
func Load(path string) (*TycoContext, error) {
	return NewParser().ParseFile(path)
}

// LoadString parses Tyco content directly from a string.
func LoadString(content string) (*TycoContext, error) {
	return NewParser().ParseString(content)
}

// ParseFile loads a file, resolving #include directives recursively.
func (p *TycoParser) ParseFile(path string) (*TycoContext, error) {
	lines, err := p.readFileWithIncludes(path)
	if err != nil {
		return nil, err
	}
	return p.parseLines(lines)
}

// ParseString parses Tyco content from memory.
func (p *TycoParser) ParseString(content string) (*TycoContext, error) {
	lines := make([]SourceLine, 0)
	for idx, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		lines = append(lines, SourceLine{
			Text:       line,
			LineNumber: idx + 1,
		})
	}
	return p.parseLines(lines)
}

func (p *TycoParser) readFileWithIncludes(path string) ([]SourceLine, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, newIOError(err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		canonical = abs
	}
	if _, exists := p.included[canonical]; exists {
		return nil, nil
	}
	p.included[canonical] = struct{}{}

	content, err := os.ReadFile(canonical)
	if err != nil {
		return nil, newIOError(err)
	}
	lines := make([]SourceLine, 0)
	parent := filepath.Dir(canonical)
	for idx, rawLine := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n") {
		sourceLine := SourceLine{
			Text:       rawLine,
			Path:       canonical,
			LineNumber: idx + 1,
		}
		trimmed := strings.TrimSpace(rawLine)
		if strings.HasPrefix(trimmed, "#include") {
			include := strings.TrimSpace(strings.TrimPrefix(trimmed, "#include"))
			include = strings.Trim(include, `"'`)
			includePath := filepath.Join(parent, include)
			nested, nestedErr := p.readFileWithIncludes(includePath)
			if nestedErr != nil {
				if tyErr, ok := nestedErr.(*TycoError); ok {
					return nil, tyErr.WithSpan(sourceLine.span())
				}
				return nil, nestedErr
			}
			lines = append(lines, nested...)
			continue
		}
		lines = append(lines, sourceLine)
	}
	return lines, nil
}

func (p *TycoParser) parseLines(lines []SourceLine) (*TycoContext, error) {
	ctx := NewContext()
	state := stateTopLevel
	var currentStruct string
	instanceLines := make([]string, 0)

	for idx := 0; idx < len(lines); idx++ {
		line := lines[idx]
		trimmed := stripInlineComment(line.Text)
		trimmedWS := strings.TrimSpace(trimmed)
		if trimmedWS == "" {
			continue
		}

		if matches := structDefRe.FindStringSubmatch(trimmedWS); matches != nil {
			if currentStruct != "" && len(instanceLines) > 0 {
				if err := p.parseStructInstances(currentStruct, instanceLines, ctx); err != nil {
					return nil, err
				}
				instanceLines = instanceLines[:0]
			}
			currentStruct = matches[1]
			ctx.EnsureStruct(currentStruct)
			state = stateInStructSchema
			continue
		}

		if matches := fieldRe.FindStringSubmatch(line.Text); matches != nil {
			isPrimary := matches[1] == "*"
			isNullable := matches[1] == "?"
			typeName := matches[2]
			isArray := matches[3] != ""
			attrName := matches[4]
			valueStr := ""
			if len(matches) > 5 {
				valueStr = matches[5]
			}
			lineSpan := line.span()

			if hasUnclosedDelimiter(valueStr, `"""`) || hasUnclosedDelimiter(valueStr, `'''`) {
				delimiter := `"""`
				if strings.Contains(valueStr, `'''`) {
					delimiter = `'''`
				}
				var err error
				idx, valueStr, err = p.accumulateMultiline(idx, lines, valueStr, delimiter)
				if err != nil {
					return nil, err
				}
			}

			valueStr = stripInlineComment(valueStr)
			isGlobalLine := !whitespacePrefix.MatchString(line.Text)

			if !isGlobalLine && currentStruct == "" {
				return nil, newParseError("Struct field defined before struct header").WithSpan(lineSpan)
			}

			if !isGlobalLine {
				field := &FieldSchema{
					Name:         attrName,
					TypeName:     typeName,
					IsPrimaryKey: isPrimary,
					IsNullable:   isNullable,
					IsArray:      isArray,
				}
				if strings.TrimSpace(valueStr) != "" {
					descriptor := fieldTypeDescriptor(typeName, isArray)
					parsed, err := p.parseValue(valueStr, descriptor, ctx, lineSpan)
					if err != nil {
						return nil, err
					}
					field.DefaultValue = parsed
				}
				ctx.GetStruct(currentStruct).AddField(field)
				state = stateInStructSchema
			} else {
				descriptor := fieldTypeDescriptor(typeName, isArray)
				value, err := p.parseValue(valueStr, descriptor, ctx, lineSpan)
				if err != nil {
					return nil, err
				}
				ctx.SetGlobal(attrName, value)
				state = stateTopLevel
			}
			continue
		}

		if matches := defaultUpdateRe.FindStringSubmatch(line.Text); matches != nil && currentStruct != "" {
			fieldName := matches[1]
			valueStr := ""
			if len(matches) > 2 {
				valueStr = matches[2]
			}
			lineSpan := line.span()
			if hasUnclosedDelimiter(valueStr, `"""`) || hasUnclosedDelimiter(valueStr, `'''`) {
				delimiter := `"""`
				if strings.Contains(valueStr, `'''`) {
					delimiter = `'''`
				}
				var err error
				idx, valueStr, err = p.accumulateMultiline(idx, lines, valueStr, delimiter)
				if err != nil {
					return nil, err
				}
			}
			valueStr = stripInlineComment(valueStr)

			var parsed *Value
			if strings.TrimSpace(valueStr) != "" {
				structDef := ctx.GetStruct(currentStruct)
				if structDef == nil {
					return nil, newUnknownStructError(currentStruct).WithSpan(lineSpan)
				}
				var fieldSchema *FieldSchema
				for _, field := range structDef.Fields() {
					if field.Name == fieldName {
						fieldSchema = field
						break
					}
				}
				if fieldSchema == nil {
					return nil, newParseErrorf("Unknown field %q", fieldName).WithSpan(lineSpan)
				}
				descriptor := fieldTypeDescriptor(fieldSchema.TypeName, fieldSchema.IsArray)
				value, err := p.parseValue(valueStr, descriptor, ctx, lineSpan)
				if err != nil {
					return nil, err
				}
				parsed = value
			}
			if err := ctx.GetStruct(currentStruct).SetDefault(fieldName, parsed); err != nil {
				return nil, newParseError(err.Error()).WithSpan(lineSpan)
			}
			continue
		}

		if strings.HasPrefix(trimmedWS, "-") {
			if currentStruct == "" {
				return nil, newParseError("Instance data encountered outside of a struct block").WithSpan(line.span())
			}
			state = stateInStructInstances
			instLine := strings.TrimSpace(strings.TrimPrefix(trimmedWS, "-"))
			for strings.HasSuffix(instLine, `\`) && idx+1 < len(lines) {
				instLine = strings.TrimRight(instLine[:len(instLine)-1], " \t")
				idx++
				nextLine := stripInlineComment(lines[idx].Text)
				instLine += " " + strings.TrimSpace(nextLine)
			}
			if hasUnclosedDelimiter(instLine, `"""`) || hasUnclosedDelimiter(instLine, `'''`) {
				delimiter := `"""`
				if strings.Contains(instLine, `'''`) {
					delimiter = `'''`
				}
				var err error
				idx, instLine, err = p.accumulateMultiline(idx, lines, instLine, delimiter)
				if err != nil {
					return nil, err
				}
			}
			instanceLines = append(instanceLines, instLine)
			continue
		}

		if state == stateInStructInstances && whitespacePrefix.MatchString(line.Text) {
			if len(instanceLines) > 0 {
				instanceLines[len(instanceLines)-1] += " " + trimmedWS
			}
			continue
		}
	}

	if currentStruct != "" && len(instanceLines) > 0 {
		if err := p.parseStructInstances(currentStruct, instanceLines, ctx); err != nil {
			return nil, err
		}
	}

	if err := ctx.Render(); err != nil {
		return nil, err
	}

	return ctx, nil
}

func (p *TycoParser) accumulateMultiline(idx int, lines []SourceLine, initial, delimiter string) (int, string, error) {
	value := initial
	cursor := idx
	for cursor+1 < len(lines) && hasUnclosedDelimiter(value, delimiter) {
		cursor++
		value += "\n" + lines[cursor].Text
	}
	if hasUnclosedDelimiter(value, delimiter) {
		return cursor, value, newParseErrorf("Unterminated %s string literal", delimiter)
	}
	return cursor, value, nil
}

func (p *TycoParser) parseStructInstances(structName string, lines []string, ctx *TycoContext) error {
	if len(lines) == 0 {
		return nil
	}
	structDef := ctx.GetStruct(structName)
	if structDef == nil {
		return newUnknownStructError(structName)
	}
	fields := structDef.Fields()
	for _, raw := range lines {
		parts := splitTopLevel(raw, ',')
		instance := NewInstance(structName)
		positional := 0
		usingNamed := false
		span := SourceSpan{
			LineText: raw,
			Line:     0,
			Column:   1,
		}
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if name, value, ok := splitNamedArgument(part); ok {
				usingNamed = true
				var fieldSchema *FieldSchema
				for _, field := range fields {
					if field.Name == name {
						fieldSchema = field
						break
					}
				}
				if fieldSchema == nil {
					return newParseErrorf("Unknown field %q in %s", name, structName).WithSpan(span)
				}
				descriptor := fieldTypeDescriptor(fieldSchema.TypeName, fieldSchema.IsArray)
				typed, err := p.parseValue(value, descriptor, ctx, span)
				if err != nil {
					return err
				}
				instance.SetAttribute(name, typed)
			} else {
				if usingNamed {
					return newParseError("Positional arguments cannot follow named arguments").WithSpan(span)
				}
				if positional >= len(fields) {
					return newParseErrorf("Too many positional arguments for %s", structName).WithSpan(span)
				}
				fieldSchema := fields[positional]
				descriptor := fieldTypeDescriptor(fieldSchema.TypeName, fieldSchema.IsArray)
				typed, err := p.parseValue(part, descriptor, ctx, span)
				if err != nil {
					return err
				}
				instance.SetAttribute(fieldSchema.Name, typed)
				positional++
			}
		}
		structDef.AddInstance(instance)
	}
	return nil
}

func (p *TycoParser) parseValue(token, typeName string, ctx *TycoContext, span SourceSpan) (*Value, error) {
	trimmed := strings.TrimSpace(token)
	if strings.EqualFold(trimmed, "null") {
		return &Value{Type: ValueNull}, nil
	}
	switch typeName {
	case "bool":
		if trimmed == "true" {
			return &Value{Type: ValueBool, Bool: true}, nil
		}
		if trimmed == "false" {
			return &Value{Type: ValueBool, Bool: false}, nil
		}
		return nil, newParseErrorf("Invalid bool literal %q", trimmed).WithSpan(span)
	case "int":
		value, err := parseInteger(trimmed)
		if err != nil {
			return nil, err.WithSpan(span)
		}
		return &Value{Type: ValueInt, Int: value}, nil
	case "float":
		num, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return nil, newParseErrorf("Invalid float literal %q: %v", trimmed, err).WithSpan(span)
		}
		return &Value{Type: ValueFloat, Float: num}, nil
	case "date":
		strVal, err := parseStringValue(trimmed)
		if err != nil {
			return nil, err.WithSpan(span)
		}
		return &Value{Type: ValueDate, String: &TycoString{Value: strVal.Value, IsLiteral: true}}, nil
	case "time":
		strVal, err := parseStringValue(trimmed)
		if err != nil {
			return nil, err.WithSpan(span)
		}
		normalized := normalizeTime(strVal.Value)
		return &Value{Type: ValueTime, String: &TycoString{Value: normalized, IsLiteral: true}}, nil
	case "datetime":
		strVal, err := parseStringValue(trimmed)
		if err != nil {
			return nil, err.WithSpan(span)
		}
		normalized := normalizeDateTime(strVal.Value)
		return &Value{Type: ValueDateTime, String: &TycoString{Value: normalized, IsLiteral: true}}, nil
	case "str":
		strVal, err := parseStringValue(trimmed)
		if err != nil {
			return nil, err.WithSpan(span)
		}
		return &Value{Type: ValueString, String: strVal}, nil
	default:
		if strings.HasSuffix(typeName, "[]") {
			base := strings.TrimSuffix(typeName, "[]")
			if trimmed == "[]" {
				return &Value{Type: ValueArray, Array: []*Value{}}, nil
			}
			if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
				return nil, newParseErrorf("Array literal must be wrapped in []: %s", trimmed).WithSpan(span)
			}
			items := splitTopLevel(trimmed[1:len(trimmed)-1], ',')
			values := make([]*Value, 0, len(items))
			for _, item := range items {
				if strings.TrimSpace(item) == "" {
					continue
				}
				parsed, err := p.parseValue(item, base, ctx, span)
				if err != nil {
					return nil, err
				}
				values = append(values, parsed)
			}
			return &Value{Type: ValueArray, Array: values}, nil
		}
		return p.parseStructCall(trimmed, typeName, ctx, span)
	}
}

func (p *TycoParser) parseStructCall(token, typeName string, ctx *TycoContext, span SourceSpan) (*Value, error) {
	matches := structCallRe.FindStringSubmatch(token)
	if matches == nil {
		return nil, newParseErrorf("Cannot parse value %q as type %q", token, typeName).WithSpan(span)
	}
	structName := matches[1]
	args := strings.TrimSpace(matches[2])
	if def := ctx.GetStruct(structName); def != nil {
		if def.HasPrimaryKey() {
			parsed, err := parseStringValue(args)
			if err != nil {
				return nil, err.WithSpan(span)
			}
			return &Value{
				Type: ValueReference,
				Reference: &TycoReference{
					StructName: structName,
					PrimaryKey: parsed.Value,
				},
			}, nil
		}
		inline, err := p.parseInlineInstance(structName, args)
		if err != nil {
			return nil, err.WithSpan(span)
		}
		return &Value{Type: ValueInstance, Instance: inline}, nil
	}
	parsed, err := parseStringValue(args)
	if err != nil {
		return nil, err.WithSpan(span)
	}
	return &Value{
		Type: ValueReference,
		Reference: &TycoReference{
			StructName: structName,
			PrimaryKey: parsed.Value,
		},
	}, nil
}

func (p *TycoParser) parseInlineInstance(structName, args string) (*TycoInstance, *TycoError) {
	instance := NewInstance(structName)
	parts := splitTopLevel(args, ',')
	position := 0
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if name, value, ok := splitNamedArgument(part); ok {
			strVal, err := parseStringValue(value)
			if err != nil {
				return nil, err
			}
			instance.SetAttribute(name, &Value{Type: ValueString, String: strVal})
		} else {
			strVal, err := parseStringValue(part)
			if err != nil {
				return nil, err
			}
			instance.SetAttribute("_arg"+strconv.Itoa(position), &Value{Type: ValueString, String: strVal})
			position++
		}
	}
	return instance, nil
}

func splitNamedArgument(part string) (string, string, bool) {
	depth := 0
	inQuotes := false
	var quote rune
	escape := false
	for idx, ch := range part {
		if escape {
			escape = false
			continue
		}
		if inQuotes {
			if ch == '\\' {
				escape = true
				continue
			}
			if ch == quote {
				inQuotes = false
			}
			continue
		}
		switch ch {
		case '"', '\'':
			inQuotes = true
			quote = ch
		case '[', '{', '(':
			depth++
		case ']', '}', ')':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 {
				name := strings.TrimSpace(part[:idx])
				value := strings.TrimSpace(part[idx+1:])
				if isValidFieldName(name) && value != "" {
					return name, value, true
				}
				return "", "", false
			}
		}
	}
	return "", "", false
}

func isValidFieldName(name string) bool {
	if name == "" {
		return false
	}
	for idx, ch := range name {
		if idx == 0 {
			if !(ch == '_' || (ch >= 'a' && ch <= 'z')) {
				return false
			}
		} else if !(ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')) {
			return false
		}
	}
	return true
}

func fieldTypeDescriptor(base string, isArray bool) string {
	if isArray {
		return base + "[]"
	}
	return base
}

func parseStringValue(token string) (*TycoString, *TycoError) {
	if strings.HasPrefix(token, `"""`) {
		rest := token[3:]
		if end := strings.Index(rest, `"""`); end != -1 {
			raw := rest[:end]
			content := stripLeadingNewline(raw)
			unescaped, err := unescapeBasicString(content)
			if err != nil {
				return nil, err
			}
			return &TycoString{
				Value:       unescaped,
				HasTemplate: strings.Contains(unescaped, "{") && strings.Contains(unescaped, "}"),
			}, nil
		}
		return nil, newParseError("Unterminated multi-line string literal")
	}
	if strings.HasPrefix(token, "'''") {
		rest := token[3:]
		if end := strings.Index(rest, "'''"); end != -1 {
			content := rest[:end]
			return &TycoString{
				Value:     content,
				IsLiteral: true,
			}, nil
		}
		return nil, newParseError("Unterminated multi-line literal string")
	}
	if strings.HasPrefix(token, `"`) && strings.HasSuffix(token, `"`) {
		inner := token[1 : len(token)-1]
		unescaped, err := unescapeBasicString(inner)
		if err != nil {
			return nil, err
		}
		return &TycoString{
			Value:       unescaped,
			HasTemplate: strings.Contains(unescaped, "{") && strings.Contains(unescaped, "}"),
		}, nil
	}
	if strings.HasPrefix(token, `'`) && strings.HasSuffix(token, `'`) {
		inner := token[1 : len(token)-1]
		return &TycoString{
			Value:     inner,
			IsLiteral: true,
		}, nil
	}
	return &TycoString{
		Value:       token,
		HasTemplate: strings.Contains(token, "{") && strings.Contains(token, "}"),
	}, nil
}
