// Copyright 2026 The protoauthz Authors
// SPDX-License-Identifier: MIT

// Package protoauthz enforces a per-RPC access level that each gRPC method
// declares in its proto file with the protoauthz.v1.auth_level option.
//
// Load the levels with LoadAuthLevelsFromRegistry and install
// NewAuthLevelInterceptor, and NewAuthLevelStreamInterceptor for streaming
// RPCs, after the interceptor that authenticates the caller. That interceptor
// records the caller with WithUserID, WithRole and WithEmail. A method with no
// level is denied, or only those under the given prefixes when prefixes are
// passed. MissingAuthLevels and MissingAuthLevelsFromRegistry list the methods
// the interceptors would deny for their level, and protoauthztest fails a test
// on each. NewLadder ranks roles of your own.
package protoauthz
