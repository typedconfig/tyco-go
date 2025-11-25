package tyco

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var escapedNewlineRe = regexp.MustCompile(`\\\s*\r?\n\s*`)

func stripInlineComment(line string) string {
	inQuotes := false
	var quote rune
	escape := false
	var builder strings.Builder

	for _, ch := range line {
		if escape {
			builder.WriteRune(ch)
			escape = false
			continue
		}
		if inQuotes {
			if ch == '\\' {
				builder.WriteRune(ch)
				escape = true
				continue
			}
			builder.WriteRune(ch)
			if ch == quote {
				inQuotes = false
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuotes = true
			quote = ch
			builder.WriteRune(ch)
			continue
		}
		if ch == '#' {
			break
		}
		builder.WriteRune(ch)
	}

	return strings.TrimRightFunc(builder.String(), unicode.IsSpace)
}

func hasUnclosedDelimiter(line, delimiter string) bool {
	start := strings.Index(line, delimiter)
	if start == -1 {
		return false
	}
	return strings.Index(line[start+len(delimiter):], delimiter) == -1
}

func hasUnclosedParentheses(text string) bool {
	depth := 0
	inQuotes := false
	var quote rune
	escape := false
	for _, ch := range text {
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
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
	}
	return depth > 0
}

func splitTopLevel(input string, delimiter rune) []string {
	var parts []string
	var current strings.Builder
	depth := 0
	inQuotes := false
	var quote rune
	escape := false

	for _, ch := range input {
		if escape {
			current.WriteRune(ch)
			escape = false
			continue
		}
		if inQuotes {
			current.WriteRune(ch)
			if ch == '\\' {
				escape = true
				continue
			}
			if ch == quote {
				inQuotes = false
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'':
			inQuotes = true
			quote = ch
			current.WriteRune(ch)
		case '\\':
			current.WriteRune(ch)
			escape = true
		case '[', '{', '(':
			depth++
			current.WriteRune(ch)
		case ']', '}', ')':
			if depth > 0 {
				depth--
			}
			current.WriteRune(ch)
		default:
			if ch == delimiter && depth == 0 {
				part := strings.TrimSpace(current.String())
				if part != "" {
					parts = append(parts, part)
				}
				current.Reset()
			} else {
				current.WriteRune(ch)
			}
		}
	}
	if tail := strings.TrimSpace(current.String()); tail != "" {
		parts = append(parts, tail)
	}
	return parts
}

func parseInteger(token string) (int64, *TycoError) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return 0, newParseError("Empty integer literal")
	}
	negative := strings.HasPrefix(trimmed, "-")
	body := trimmed
	if negative {
		body = strings.TrimSpace(trimmed[1:])
	}
	base := 10
	switch {
	case strings.HasPrefix(body, "0x") || strings.HasPrefix(body, "0X"):
		base = 16
		body = body[2:]
	case strings.HasPrefix(body, "0o") || strings.HasPrefix(body, "0O"):
		base = 8
		body = body[2:]
	case strings.HasPrefix(body, "0b") || strings.HasPrefix(body, "0B"):
		base = 2
		body = body[2:]
	}
	if body == "" {
		return 0, newParseErrorf("Failed to parse integer %q", token)
	}
	value, err := strconv.ParseInt(body, base, 64)
	if err != nil {
		return 0, newParseErrorf("Failed to parse integer %q: %v", token, err)
	}
	if negative {
		value = -value
	}
	return value, nil
}

func normalizeTime(value string) string {
	idx := strings.Index(value, ".")
	if idx == -1 {
		return value
	}
	head := value[:idx+1]
	rest := value[idx+1:]
	digits := 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}
	fraction := rest[:digits]
	for len(fraction) < 6 {
		fraction += "0"
	}
	if len(fraction) > 6 {
		fraction = fraction[:6]
	}
	return head + fraction + rest[digits:]
}

func normalizeDateTime(value string) string {
	result := strings.ReplaceAll(value, " ", "T")
	if strings.HasSuffix(result, "Z") {
		result = strings.TrimSuffix(result, "Z") + "+00:00"
	}
	idx := strings.Index(result, ".")
	if idx == -1 {
		return result
	}
	tzStart := len(result)
	for i := idx; i < len(result); i++ {
		if result[i] == '+' || result[i] == '-' {
			tzStart = i
			break
		}
	}
	fraction := normalizeTime(result[idx:tzStart])
	return result[:idx] + fraction + result[tzStart:]
}

func unescapeBasicString(value string) (string, *TycoError) {
	value = escapedNewlineRe.ReplaceAllString(value, "")
	var builder strings.Builder
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if ch != '\\' {
			builder.WriteByte(ch)
			continue
		}
		if i+1 >= len(value) {
			builder.WriteByte('\\')
			break
		}
		next := value[i+1]
		i++
		switch next {
		case 'n':
			builder.WriteByte('\n')
		case 't':
			builder.WriteByte('\t')
		case 'r':
			builder.WriteByte('\r')
		case 'b':
			builder.WriteByte('\b')
		case 'f':
			builder.WriteByte('\f')
		case '"':
			builder.WriteByte('"')
		case '\\':
			builder.WriteByte('\\')
		case 'u', 'U':
			required := 4
			if next == 'U' {
				required = 8
			}
			if i+required >= len(value)+1 {
				return "", newParseError("Incomplete unicode escape")
			}
			hex := value[i+1 : i+1+required]
			i += required
			code, err := strconv.ParseInt(hex, 16, 32)
			if err != nil {
				return "", newParseErrorf("Invalid unicode escape: \\%c%s", next, hex)
			}
			builder.WriteRune(rune(code))
		default:
			builder.WriteByte('\\')
			builder.WriteByte(next)
		}
	}
	return builder.String(), nil
}

func stripLeadingNewline(value string) string {
	if strings.HasPrefix(value, "\n") {
		return value[1:]
	}
	return value
}
