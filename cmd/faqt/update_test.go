package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/martinghunt/faqt/internal/buildinfo"
	"github.com/martinghunt/faqt/internal/selfupdate"
)

func TestRootHasUpdateCommand(t *testing.T) {
	cmd := newRootCmd()
	found, _, err := cmd.Find([]string{"update"})
	if err != nil {
		t.Fatalf("Find(update) error = %v", err)
	}
	if found == nil || found.Name() != "update" {
		t.Fatalf("Find(update) = %v, want update command", found)
	}
	for _, name := range []string{"check", "force"} {
		if found.Flags().Lookup(name) == nil {
			t.Fatalf("update command missing --%s flag", name)
		}
	}
}

func TestUpdateCommandCheckOnly(t *testing.T) {
	oldRun := runSelfUpdate
	oldVersion := buildinfo.Version
	defer func() {
		runSelfUpdate = oldRun
		buildinfo.Version = oldVersion
	}()
	buildinfo.Version = "v1.0.0"

	var gotOpts selfupdate.Options
	runSelfUpdate = func(ctx context.Context, opts selfupdate.Options) (selfupdate.Result, error) {
		gotOpts = opts
		return selfupdate.Result{
			CurrentVersion: opts.CurrentVersion,
			LatestVersion:  "v1.1.0",
			CheckOnly:      true,
		}, nil
	}

	cmd := newUpdateCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--check"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !gotOpts.CheckOnly || gotOpts.Force {
		t.Fatalf("update options = %+v, want check-only without force", gotOpts)
	}
	if gotOpts.CurrentVersion != "v1.0.0" {
		t.Fatalf("current version = %q, want v1.0.0", gotOpts.CurrentVersion)
	}
	if stdout.String() != "faqt 1.1.0 is available (current 1.0.0)\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestUpdateCommandForcePassesGitHubToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "token-from-env")

	oldRun := runSelfUpdate
	oldVersion := buildinfo.Version
	defer func() {
		runSelfUpdate = oldRun
		buildinfo.Version = oldVersion
	}()
	buildinfo.Version = "dev"

	var gotOpts selfupdate.Options
	runSelfUpdate = func(ctx context.Context, opts selfupdate.Options) (selfupdate.Result, error) {
		gotOpts = opts
		return selfupdate.Result{
			CurrentVersion: opts.CurrentVersion,
			LatestVersion:  "v1.1.0",
			Updated:        true,
		}, nil
	}

	cmd := newUpdateCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !gotOpts.Force || gotOpts.Token != "token-from-env" {
		t.Fatalf("update options = %+v, want force with env token", gotOpts)
	}
	if stdout.String() != "updated faqt from dev to 1.1.0\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestUpdateCommandReturnsUpdateError(t *testing.T) {
	oldRun := runSelfUpdate
	defer func() { runSelfUpdate = oldRun }()
	wantErr := errors.New("update failed")
	runSelfUpdate = func(ctx context.Context, opts selfupdate.Options) (selfupdate.Result, error) {
		return selfupdate.Result{}, wantErr
	}

	cmd := newUpdateCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}
