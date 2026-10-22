// This program checks three callers against DeleteNote, an admin-only
// RPC in the example's proto file.
package main

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/tetriz-ai/protoauthz"
	_ "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/v1" // registers the example service
)

func main() {
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
}
