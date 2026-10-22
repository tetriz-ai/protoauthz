// Copyright 2026 The protoauthz Authors
// SPDX-License-Identifier: MIT

package protoauthztest_test

import (
	"testing"

	examplev1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/v1"
	"github.com/tetriz-ai/protoauthz/protoauthztest"
)

// In your service's test suite, pass the generated descriptor of each proto
// file that defines a service. The test fails naming every RPC without a
// usable auth_level.
//
// This example only shows the call. It has no output, so go test compiles it
// but does not run it.
func ExampleRequireAuthLevels() {
	_ = func(t *testing.T) {
		protoauthztest.RequireAuthLevels(t, examplev1.File_protoauthz_example_v1_notes_proto)
	}
}

// Put the test in the package that registers your services on the
// grpc.Server, so every service you serve is linked, and pass the prefixes
// your interceptors take. There is then no list of proto files to keep. The
// test fails naming every RPC the interceptors would deny for its level, and
// on a prefix with no linked RPC under it.
//
// This example only shows the call. It has no output, so go test compiles it
// but does not run it.
func ExampleRequireAuthLevelsFromRegistry() {
	_ = func(t *testing.T) {
		protoauthztest.RequireAuthLevelsFromRegistry(t, "/protoauthz.example.v1.")
	}
}
