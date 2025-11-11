package tyco

import (
	"errors"
	"fmt"
	"strings"
)

// ErrorKind enumerates the categories of parser errors.
type ErrorKind int

const (
	ErrKindParse ErrorKind = iota
	ErrKindIO
	ErrKindReference
	ErrKindUnknownStruct
)

// SourceSpan conveys file/line context for diagnostics.
type SourceSpan struct {
	Path     string
	Line     int
	Column   int
	LineText string
}

// Display renders a human-friendly pointer similar to Python tracebacks.
func (s SourceSpan) Display() string {
	var location string
	if s.Path != "" {
		location = fmt.Sprintf("File %q, line %d, column %d:", s.Path, s.Line, s.Column)
	} else {
		location = fmt.Sprintf("Line %d, column %d:", s.Line, s.Column)
	}

	line := s.LineText
	if line == "" {
		return location
	}

	var builder strings.Builder
	visualCol := 0
	for i := 0; i < len(line) && i < s.Column-1; i++ {
		ch := line[i]
		if ch == '\t' {
			nextTab := ((visualCol / 8) + 1) * 8
			for visualCol < nextTab {
				builder.WriteByte(' ')
				visualCol++
			}
		} else {
			builder.WriteByte(' ')
			visualCol++
		}
	}
	builder.WriteByte('^')

	return fmt.Sprintf("%s\n%s\n%s", location, line, builder.String())
}

// TycoError is the shared error type used across the Go binding.
type TycoError struct {
	Kind    ErrorKind
	Message string
	Err     error
	Span    *SourceSpan
}

func (e *TycoError) Error() string {
	if e == nil {
		return "<nil>"
	}
	var builder strings.Builder
	switch e.Kind {
	case ErrKindIO:
		builder.WriteString("I/O error")
	case ErrKindReference:
		builder.WriteString("Reference error")
	case ErrKindUnknownStruct:
		builder.WriteString("Unknown struct")
	default:
		builder.WriteString("Parse error")
	}
	if e.Message != "" {
		builder.WriteString(": ")
		builder.WriteString(e.Message)
	}
	if e.Span != nil {
		builder.WriteString("\n")
		builder.WriteString(e.Span.Display())
	}
	return builder.String()
}

// Unwrap allows errors.Is/As to see any wrapped cause.
func (e *TycoError) Unwrap() error {
	return e.Err
}

// WithSpan produces a copy of the error annotated with the provided span.
func (e *TycoError) WithSpan(span SourceSpan) *TycoError {
	if e == nil {
		return nil
	}
	clone := *e
	clone.Span = &span
	return &clone
}

func newParseError(msg string) *TycoError {
	return &TycoError{Kind: ErrKindParse, Message: msg}
}

func newParseErrorf(format string, args ...any) *TycoError {
	return newParseError(fmt.Sprintf(format, args...))
}

func newIOError(err error) *TycoError {
	return &TycoError{Kind: ErrKindIO, Err: err, Message: err.Error()}
}

func newUnknownStructError(name string) *TycoError {
	return &TycoError{Kind: ErrKindUnknownStruct, Message: fmt.Sprintf("Unknown struct %q", name)}
}

func newReferenceError(msg string) *TycoError {
	return &TycoError{Kind: ErrKindReference, Message: msg}
}

func isParseError(err error) bool {
	var tycoErr *TycoError
	if errors.As(err, &tycoErr) {
		return tycoErr.Kind == ErrKindParse
	}
	return false
}
