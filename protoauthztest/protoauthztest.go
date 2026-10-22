// Copyright 2026 The protoauthz Authors
// SPDX-License-Identifier: MIT

// Package protoauthztest checks, in a test, that every RPC carries an
// auth_level the protoauthz interceptors can enforce.
package protoauthztest

import (
	"fmt"
	"slices"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/tetriz-ai/protoauthz"
	"github.com/tetriz-ai/protoauthz/internal/prefix"
)

// RequireAuthLevels fails t, naming each one, for every RPC in files that
// has no auth_level, has AUTH_LEVEL_UNSPECIFIED, or has a value protoauthz
// does not know. Pass the generated File_... descriptors of your services.
// It also fails when an argument is nil or when the files hold no RPC.
func RequireAuthLevels(t testing.TB, files ...protoreflect.FileDescriptor) {
	t.Helper()
	rpcs := 0
	for i, fd := range files {
		if fd == nil {
			t.Errorf("protoauthztest: file descriptor %d is nil", i)
			continue
		}
		for j := range fd.Services().Len() {
			rpcs += fd.Services().Get(j).Methods().Len()
		}
	}
	if rpcs == 0 {
		t.Errorf("protoauthztest: the files passed contain no RPCs")
	}
	for _, method := range protoauthz.MissingAuthLevels(files...) {
		t.Errorf("%s has no usable (protoauthz.v1.auth_level) option", method)
	}
}

// RequireAuthLevelsFromRegistry is RequireAuthLevels over every RPC in the
// global proto registry, so there is no list of File_... descriptors to keep.
// It fails for each method protoauthz.MissingAuthLevelsFromRegistry reports:
// an RPC with no level under one of the prefixes, or anywhere when there are
// none, and an RPC set to an unenforceable level wherever it is. It sees only
// the services linked into the test binary, so put the test in the package
// that registers your services on the grpc.Server, which links every service
// it serves. Pass the prefixes your interceptors take ("/your.pkg.v1."), so
// gRPC's own services stay out.
//
// It also fails for each prefix no registered RPC is under, as when the
// generated package is not imported or the prefix is mistyped, and, with no
// prefixes, when the registry holds no RPC. It panics on a prefix that can
// match no method name, as the interceptors do.
func RequireAuthLevelsFromRegistry(t testing.TB, prefixes ...string) {
	t.Helper()
	missing := protoauthz.MissingAuthLevelsFromRegistry(prefixes...)
	methods := registeredMethods()
	if len(prefixes) == 0 && len(methods) == 0 {
		t.Errorf("protoauthztest: the registry holds no RPC: import your generated packages")
	}
	// The same normalisation and matching as the interceptors.
	for i, normalised := range prefix.Normalise(prefixes) {
		under := []string{normalised}
		if !slices.ContainsFunc(methods, func(m string) bool { return prefix.Under(m, under) }) {
			t.Errorf("protoauthztest: no registered RPC is under prefix %q: import its generated package, or fix the prefix", prefixes[i])
		}
	}
	for _, method := range missing {
		t.Errorf("%s has no usable (protoauthz.v1.auth_level) option", method)
	}
}

// registeredMethods returns the full name of every RPC in the global registry.
func registeredMethods() []string {
	var methods []string
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		for i := range fd.Services().Len() {
			sd := fd.Services().Get(i)
			for j := range sd.Methods().Len() {
				methods = append(methods, fmt.Sprintf("/%s/%s", sd.FullName(), sd.Methods().Get(j).Name()))
			}
		}
		return true
	})
	return methods
}
