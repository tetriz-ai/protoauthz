// Command custom-role-ladder uses the roles viewer, editor and owner in place
// of member, admin and super_admin. editor is the lowest role that passes
// AUTH_LEVEL_ADMIN, and owner the lowest that passes AUTH_LEVEL_SUPER_ADMIN.
package main

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/tetriz-ai/protoauthz"
	_ "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/v1" // registers the example service
)

func main() {
	ladder, err := protoauthz.NewLadder([]string{"viewer", "editor", "owner"}, "editor", "owner")
	if err != nil {
		log.Fatal(err)
	}
	authz := ladder.UnaryInterceptor(protoauthz.LoadAuthLevelsFromRegistry(), nil)
	handler := func(context.Context, any) (any, error) { return "done", nil }

	for _, method := range []string{"DeleteNote", "PurgeNotes"} { // AUTH_LEVEL_ADMIN, AUTH_LEVEL_SUPER_ADMIN
		info := &grpc.UnaryServerInfo{FullMethod: "/protoauthz.example.v1.NoteService/" + method}
		for _, role := range []string{"viewer", "editor", "owner", "admin"} {
			ctx := protoauthz.WithRole(protoauthz.WithUserID(context.Background(), "u-1"), role)
			_, err := authz(ctx, nil, info, handler)
			fmt.Printf("%s as %s: %v\n", method, role, status.Code(err))
		}
	}

	if _, err := protoauthz.NewLadder([]string{"viewer", "editor"}, "editor", "owner"); err != nil {
		fmt.Println(err)
	}
}
