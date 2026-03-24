package main

import "testing"

func TestDownloadGenomeCommandExists(t *testing.T) {
	cmd := newRootCmd()
	found, _, err := cmd.Find([]string{"download-genome"})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if found == nil || found.Name() != "download-genome" {
		t.Fatalf("unexpected command = %v", found)
	}
}
