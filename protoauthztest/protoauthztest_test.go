// Copyright 2026 The protoauthz Authors
// SPDX-License-Identifier: MIT

package protoauthztest_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	draftv1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/draft/v1"
	examplev1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/v1"
	protoauthzv1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/v1"
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

func TestRequireAuthLevels(t *testing.T) {
	tests := []struct {
		name string
		run  func(testing.TB)
		want []string
	}{
		{
			"names the one unannotated RPC",
			func(tb testing.TB) {
				protoauthztest.RequireAuthLevels(tb, draftv1.File_protoauthz_example_draft_v1_notes_proto)
			},
			[]string{"/protoauthz.example.draft.v1.NoteService/DeleteNote has no usable (protoauthz.v1.auth_level) option"},
		},
		{
			"fails when no files are passed",
			func(tb testing.TB) { protoauthztest.RequireAuthLevels(tb) },
			[]string{"protoauthztest: the files passed contain no RPCs"},
		},
		{
			"fails when the files hold no service",
			func(tb testing.TB) {
				protoauthztest.RequireAuthLevels(tb, protoauthzv1.File_protoauthz_v1_auth_level_proto)
			},
			[]string{"protoauthztest: the files passed contain no RPCs"},
		},
		{
			"reports a nil descriptor and still checks the rest",
			func(tb testing.TB) {
				protoauthztest.RequireAuthLevels(tb, nil, draftv1.File_protoauthz_example_draft_v1_notes_proto)
			},
			[]string{
				"protoauthztest: file descriptor 0 is nil",
				"/protoauthz.example.draft.v1.NoteService/DeleteNote has no usable (protoauthz.v1.auth_level) option",
			},
		},
		{
			"passes a fully annotated service",
			func(tb testing.TB) {
				protoauthztest.RequireAuthLevels(tb, examplev1.File_protoauthz_example_v1_notes_proto)
			},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{TB: t}
			tt.run(r)
			if !slices.Equal(r.errors, tt.want) {
				t.Fatalf("errors %q, want %q", r.errors, tt.want)
			}
		})
	}
}

func TestRequireAuthLevelsFromRegistry(t *testing.T) {
	// The example and draft services are registered by their imports above.
	tests := []struct {
		name     string
		prefixes []string
		want     []string
	}{
		{
			"names the draft service's one unannotated RPC",
			[]string{"/protoauthz.example.draft.v1."},
			[]string{"/protoauthz.example.draft.v1.NoteService/DeleteNote has no usable (protoauthz.v1.auth_level) option"},
		},
		{
			"passes a fully annotated service",
			[]string{"/protoauthz.example.v1."},
			nil,
		},
		{
			"matches a prefix without its leading slash, as the interceptors do",
			[]string{"protoauthz.example.v1."},
			nil,
		},
		{
			"with no prefixes checks every registered RPC",
			nil,
			[]string{"/protoauthz.example.draft.v1.NoteService/DeleteNote has no usable (protoauthz.v1.auth_level) option"},
		},
		{
			"fails on a prefix no registered RPC is under",
			[]string{"/no.such.v1."},
			[]string{`protoauthztest: no registered RPC is under prefix "/no.such.v1.": import its generated package, or fix the prefix`},
		},
		{
			"names a prefix as it was passed, before normalising",
			[]string{"no.such.v1."},
			[]string{`protoauthztest: no registered RPC is under prefix "no.such.v1.": import its generated package, or fix the prefix`},
		},
		{
			"a prefix matches only the start of a method name",
			[]string{"/DeleteNote"},
			[]string{`protoauthztest: no registered RPC is under prefix "/DeleteNote": import its generated package, or fix the prefix`},
		},
		{
			"a prefix with two leading slashes gets exactly one, as the interceptors do",
			[]string{"//protoauthz.example.v1."},
			nil,
		},
		{
			"fails on the one unmatched prefix among several",
			[]string{"/protoauthz.example.v1.", "/no.such.v1."},
			[]string{`protoauthztest: no registered RPC is under prefix "/no.such.v1.": import its generated package, or fix the prefix`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{TB: t}
			protoauthztest.RequireAuthLevelsFromRegistry(r, tt.prefixes...)
			if !slices.Equal(r.errors, tt.want) {
				t.Fatalf("errors %q, want %q", r.errors, tt.want)
			}
		})
	}
}

// A prefix that can match no method name panics, as it does for the
// interceptors, before any check runs.
func TestRequireAuthLevelsFromRegistryPanicsOnInvalidPrefix(t *testing.T) {
	for _, bad := range []string{"/pkg-x.", "/x.v1.S/M/", "/pkg. "} {
		t.Run(bad, func(t *testing.T) {
			r := &recorder{TB: t}
			defer func() {
				got := recover()
				want := fmt.Sprintf("protoauthz: invalid prefix %q", bad)
				if msg, ok := got.(string); !ok || !strings.HasPrefix(msg, want) {
					t.Fatalf("panic %v, want one starting %q", got, want)
				}
				if len(r.errors) != 0 {
					t.Fatalf("errors before the panic: %q", r.errors)
				}
			}()
			protoauthztest.RequireAuthLevelsFromRegistry(r, bad)
		})
	}
}

// The check a service's own test suite runs.
func TestExampleServiceIsAnnotated(t *testing.T) {
	protoauthztest.RequireAuthLevels(t, examplev1.File_protoauthz_example_v1_notes_proto)
}
