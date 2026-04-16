package s02_tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafePath_Valid(t *testing.T) {
	dir := t.TempDir()
	got, err := SafePath(dir, "foo.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != filepath.Join(dir, "foo.txt") {
		t.Fatalf("got %q, want %q", got, filepath.Join(dir, "foo.txt"))
	}
}

func TestSafePath_Traversal(t *testing.T) {
	dir := t.TempDir()
	_, err := SafePath(dir, "../../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestSafePath_AbsoluteOutside(t *testing.T) {
	dir := t.TempDir()
	_, err := SafePath(dir, "/etc/passwd")
	if err == nil {
		t.Fatal("expected error for absolute path outside workspace")
	}
}

func TestReadFileTool_Basic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("line1\nline2\nline3\n"), 0o644)
	rt := NewReadFileTool(dir)
	result, err := rt.Execute(context.Background(), map[string]any{"path": "test.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "line1") {
		t.Fatalf("expected line1 in result: %q", result)
	}
}

func TestReadFileTool_WithLimit(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("a\nb\nc\nd\ne\n"), 0o644)
	rt := NewReadFileTool(dir)
	result, _ := rt.Execute(context.Background(), map[string]any{"path": "test.txt", "limit": float64(2)})
	if !strings.Contains(result, "more lines") {
		t.Fatalf("expected truncation marker, got %q", result)
	}
}

func TestWriteFileTool_Basic(t *testing.T) {
	dir := t.TempDir()
	wt := NewWriteFileTool(dir)
	result, err := wt.Execute(context.Background(), map[string]any{"path": "out.txt", "content": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Wrote 5 bytes to out.txt" {
		t.Fatalf("unexpected result: %q", result)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "out.txt"))
	if string(data) != "hello" {
		t.Fatalf("content mismatch: %q", string(data))
	}
}

func TestWriteFileTool_NestedDir(t *testing.T) {
	dir := t.TempDir()
	wt := NewWriteFileTool(dir)
	wt.Execute(context.Background(), map[string]any{"path": "sub/dir/f.txt", "content": "nested"})
	data, _ := os.ReadFile(filepath.Join(dir, "sub", "dir", "f.txt"))
	if string(data) != "nested" {
		t.Fatalf("nested content mismatch: %q", string(data))
	}
}

func TestEditFileTool_Basic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "edit.txt"), []byte("hello world"), 0o644)
	et := NewEditFileTool(dir)
	result, _ := et.Execute(context.Background(), map[string]any{"path": "edit.txt", "old_text": "world", "new_text": "Go"})
	if result != "Edited edit.txt" {
		t.Fatalf("unexpected result: %q", result)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "edit.txt"))
	if string(data) != "hello Go" {
		t.Fatalf("edit mismatch: %q", string(data))
	}
}

func TestEditFileTool_NotFound(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "edit.txt"), []byte("hello"), 0o644)
	et := NewEditFileTool(dir)
	result, _ := et.Execute(context.Background(), map[string]any{"path": "edit.txt", "old_text": "xyz", "new_text": "abc"})
	if !strings.Contains(result, "not found") {
		t.Fatalf("expected not-found error, got %q", result)
	}
}
