// Copyright 2026 The protoauthz Authors
// SPDX-License-Identifier: MIT

// Package emptyregistry_test links no generated service, so the global
// registry holds no RPC. It checks that RequireAuthLevelsFromRegistry with no
// prefixes fails then, rather than pass on nothing.
package emptyregistry_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/tetriz-ai/protoauthz/protoauthztest"
)

// recorder is a testing.TB that keeps the errors instead of failing.
type recorder struct {
	testing.TB
	errors []string
}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func TestRequireAuthLevelsFromRegistryFailsOnAnEmptyRegistry(t *testing.T) {
	r := &recorder{TB: t}
	protoauthztest.RequireAuthLevelsFromRegistry(r)
	want := []string{"protoauthztest: the registry holds no RPC: import your generated packages"}
	if !slices.Equal(r.errors, want) {
		t.Fatalf("errors %q, want %q", r.errors, want)
	}
}

// With prefixes, an empty registry fails on each prefix, and the no-RPC
// message for a call with no prefixes does not repeat it.
func TestRequireAuthLevelsFromRegistryWithPrefixesOnAnEmptyRegistry(t *testing.T) {
	r := &recorder{TB: t}
	protoauthztest.RequireAuthLevelsFromRegistry(r, "/x.v1.")
	want := []string{`protoauthztest: no registered RPC is under prefix "/x.v1.": import its generated package, or fix the prefix`}
	if !slices.Equal(r.errors, want) {
		t.Fatalf("errors %q, want %q", r.errors, want)
	}
}
