package tyco

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestCanonicalSuite(t *testing.T) {
	root := ".."
	inputsDir := filepath.Join(root, "tyco-test-suite", "inputs")
	expectedDir := filepath.Join(root, "tyco-test-suite", "expected")

	entries, err := os.ReadDir(inputsDir)
	if err != nil {
		t.Fatalf("unable to read shared inputs: %v", err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".tyco" {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		inputPath := filepath.Join(inputsDir, name)
		expectedPath := filepath.Join(expectedDir, strings.TrimSuffix(name, filepath.Ext(name))+".json")
		if _, err := os.Stat(expectedPath); err != nil {
			t.Logf("skip %s (missing expected)", name)
			continue
		}

		ctx, err := Load(inputPath)
		if err != nil {
			t.Fatalf("parse %s failed: %v", name, err)
		}
		actual := canonicaliseJSON(ctx.ToJSON())
		expectedBytes, err := os.ReadFile(expectedPath)
		if err != nil {
			t.Fatalf("read expected %s failed: %v", name, err)
		}
		expected := decodeJSON(expectedBytes)

		if !reflect.DeepEqual(expected, actual) {
			t.Fatalf("mismatch for %s:\nexpected: %v\nactual: %v", name, expected, actual)
		}
	}
}

func canonicaliseJSON(data map[string]any) any {
	return cloneInterfaces(data)
}

func cloneInterfaces(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, val := range typed {
			out[key] = cloneInterfaces(val)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for idx, item := range typed {
			out[idx] = cloneInterfaces(item)
		}
		return out
	default:
		return value
	}
}

func decodeJSON(data []byte) any {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		panic(err)
	}
	return normalizeNumbers(out)
}

func normalizeNumbers(value any) any {
	switch typed := value.(type) {
	case json.Number:
		str := typed.String()
		if strings.ContainsAny(str, ".eE") {
			f, _ := typed.Float64()
			return f
		}
		if i, err := typed.Int64(); err == nil {
			return i
		}
		f, _ := typed.Float64()
		return f
	case []any:
		for idx, item := range typed {
			typed[idx] = normalizeNumbers(item)
		}
		return typed
	case map[string]any:
		for key, val := range typed {
			typed[key] = normalizeNumbers(val)
		}
		return typed
	default:
		return value
	}
}
