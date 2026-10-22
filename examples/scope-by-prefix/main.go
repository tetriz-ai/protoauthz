// Command scope-by-prefix shows how prefixes narrow which unannotated methods
// the interceptor denies. With the prefix "/protoauthz.example.v1.", an
// unannotated method under it is denied and one outside it, such as a health
// check registered by another package, passes. With no prefix, both are denied.
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
	levels := protoauthz.LoadAuthLevelsFromRegistry()
	handler := func(context.Context, any) (any, error) { return "done", nil }
	caller := protoauthz.WithRole(protoauthz.WithUserID(context.Background(), "u-1"), protoauthz.RoleMember)

	methods := []string{
		"/protoauthz.example.v1.NoteService/ListNotes",   // annotated: AUTH_LEVEL_AUTHENTICATED
		"/protoauthz.example.v1.NoteService/ArchiveNote", // under the prefix, no auth_level
		"/grpc.health.v1.Health/Check",                   // outside the prefix, no auth_level
	}
	for _, scope := range []struct {
		name     string
		prefixes []string
	}{
		{"prefix /protoauthz.example.v1.", []string{"/protoauthz.example.v1."}},
		{"no prefix", nil},
	} {
		authz := protoauthz.NewAuthLevelInterceptor(levels, nil, scope.prefixes...)
		for _, method := range methods {
			_, err := authz(caller, nil, &grpc.UnaryServerInfo{FullMethod: method}, handler)
			fmt.Printf("%s: %s: %v\n", scope.name, method, status.Code(err))
		}
	}
}
