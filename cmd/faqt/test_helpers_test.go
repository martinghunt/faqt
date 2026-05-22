package main

import (
	"os"
	"testing"
)

func runWithCapturedStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()

	out, err := os.CreateTemp(t.TempDir(), "stdout-*")
	if err != nil {
		t.Fatalf("CreateTemp(stdout) error = %v", err)
	}

	oldStdout := os.Stdout
	os.Stdout = out
	runErr := run()
	os.Stdout = oldStdout

	if err := out.Close(); err != nil {
		t.Fatalf("Close(stdout) error = %v", err)
	}
	data, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatalf("ReadFile(stdout) error = %v", err)
	}
	return string(data), runErr
}

func runWithCapturedStdinStdout(t *testing.T, input string, run func() error) (string, error) {
	t.Helper()

	in, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatalf("CreateTemp(stdin) error = %v", err)
	}
	if _, err := in.WriteString(input); err != nil {
		t.Fatalf("WriteString(stdin) error = %v", err)
	}
	if _, err := in.Seek(0, 0); err != nil {
		t.Fatalf("Seek(stdin) error = %v", err)
	}
	defer func() {
		if err := in.Close(); err != nil {
			t.Fatalf("Close(stdin) error = %v", err)
		}
	}()

	oldStdin := os.Stdin
	os.Stdin = in
	defer func() { os.Stdin = oldStdin }()

	return runWithCapturedStdout(t, run)
}
