// Copyright 2026 The protoauthz Authors
// SPDX-License-Identifier: MIT

package protoauthz_test

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/tetriz-ai/protoauthz"
	draftv1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/draft/v1"
	_ "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/v1" // registers the example service
)

// DeleteNote's proto sets (protoauthz.v1.auth_level) = AUTH_LEVEL_ADMIN. This
// is the README's quick start and examples/deny-by-role.
func ExampleNewAuthLevelInterceptor() {
	authz := protoauthz.NewAuthLevelInterceptor(protoauthz.LoadAuthLevelsFromRegistry(), nil)
	info := &grpc.UnaryServerInfo{FullMethod: "/protoauthz.example.v1.NoteService/DeleteNote"}
	deleteNote := func(context.Context, any) (any, error) { return "deleted", nil }

	anonymous := context.Background()
	member := protoauthz.WithRole(protoauthz.WithUserID(anonymous, "u-1"), protoauthz.RoleMember)
	admin := protoauthz.WithRole(protoauthz.WithUserID(anonymous, "u-2"), protoauthz.RoleAdmin)
	for _, ctx := range []context.Context{anonymous, member, admin} {
		_, err := authz(ctx, nil, info, deleteNote)
		fmt.Println(status.Code(err))
	}
	// Output:
	// Unauthenticated
	// PermissionDenied
	// OK
}

// WatchNotes is a server-streaming RPC at AUTH_LEVEL_AUTHENTICATED.
func ExampleNewAuthLevelStreamInterceptor() {
	authz := protoauthz.NewAuthLevelStreamInterceptor(protoauthz.LoadAuthLevelsFromRegistry(), nil)
	info := &grpc.StreamServerInfo{FullMethod: "/protoauthz.example.v1.NoteService/WatchNotes", IsServerStream: true}
	watch := func(any, grpc.ServerStream) error { return nil }

	signedIn := protoauthz.WithUserID(context.Background(), "u-1")
	for _, ctx := range []context.Context{context.Background(), signedIn} {
		err := authz(nil, stream{ctx: ctx}, info, watch)
		fmt.Println(status.Code(err))
	}
	// Output:
	// Unauthenticated
	// OK
}

// stream is a grpc.ServerStream whose context is the only part in use.
type stream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s stream) Context() context.Context { return s.ctx }

func ExampleNewLadder() {
	ladder, err := protoauthz.NewLadder([]string{"viewer", "editor", "owner"}, "editor", "owner")
	if err != nil {
		fmt.Println(err)
		return
	}
	authz := ladder.UnaryInterceptor(protoauthz.LoadAuthLevelsFromRegistry(), nil)
	info := &grpc.UnaryServerInfo{FullMethod: "/protoauthz.example.v1.NoteService/DeleteNote"}
	for _, role := range []string{"viewer", "editor"} {
		ctx := protoauthz.WithRole(protoauthz.WithUserID(context.Background(), "u-1"), role)
		_, err := authz(ctx, nil, info, func(context.Context, any) (any, error) { return nil, nil })
		fmt.Println(role, status.Code(err))
	}
	// Output:
	// viewer PermissionDenied
	// editor OK
}

func ExampleMissingAuthLevels() {
	fmt.Println(protoauthz.MissingAuthLevels(draftv1.File_protoauthz_example_draft_v1_notes_proto))
	// Output:
	// [/protoauthz.example.draft.v1.NoteService/DeleteNote]
}

func ExampleUnauthenticatedMethods() {
	levels := protoauthz.LoadAuthLevelsFromRegistry()
	fmt.Println(protoauthz.UnauthenticatedMethods(levels))
	// Output:
	// [/protoauthz.example.v1.NoteService/GetVersion]
}

// Call it where every service you serve is linked, with the prefixes your
// interceptors take. Here the draft notes service is linked, and it leaves
// DeleteNote without a level, so the result names
// /protoauthz.example.draft.v1.NoteService/DeleteNote. Any linked RPC set to
// AUTH_LEVEL_UNSPECIFIED or an unknown value is listed too, under any prefix.
//
// This example has no output, so go test compiles it but does not run it. The
// result depends on everything linked into the binary, and this package's
// tests register services of their own.
func ExampleMissingAuthLevelsFromRegistry() {
	_ = draftv1.File_protoauthz_example_draft_v1_notes_proto
	for _, method := range protoauthz.MissingAuthLevelsFromRegistry("/protoauthz.example.draft.v1.") {
		fmt.Println(method)
	}
}
