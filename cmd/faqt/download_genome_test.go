package main

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadGenomeCommandExists(t *testing.T) {
	cmd := newRootCmd()
	found, _, err := cmd.Find([]string{"download-genome"})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if found == nil || found.Name() != "download-genome" {
		t.Fatalf("unexpected command = %v", found)
	}
	if found.Flags().Lookup("fasta") == nil {
		t.Fatal("download-genome command missing --fasta flag")
	}
	if found.Flags().Lookup("format") != nil {
		t.Fatal("download-genome command still has --format flag")
	}
}

func TestDownloadGenomeCommandRejectsRemovedFormatFlag(t *testing.T) {
	cmd := newDownloadGenomeCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"NC_000001.1",
		"-o", filepath.Join(t.TempDir(), "genome.fa"),
		"--format", "bad",
	})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --format") {
		t.Fatalf("Execute() error = %v, want unknown --format flag", err)
	}
}
