// Command staff-allow-list guards ExportNotes, whose proto sets
// (protoauthz.v1.auth_level) = AUTH_LEVEL_ALLOWLIST, with a callback that
// admits a fixed set of support staff by email. The callback sees only
// authenticated callers.
package main

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/tetriz-ai/protoauthz"
	_ "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/v1" // registers the example service
)

var supportStaff = map[string]bool{"jane@example.com": true}

func onSupportRota(_ context.Context, email, _ string) (bool, error) {
	return supportStaff[email], nil
}

func main() {
	authz := protoauthz.NewAuthLevelInterceptor(protoauthz.LoadAuthLevelsFromRegistry(), onSupportRota)
	info := &grpc.UnaryServerInfo{FullMethod: "/protoauthz.example.v1.NoteService/ExportNotes"}
	export := func(context.Context, any) (any, error) { return "all notes", nil }

	callers := []struct {
		name string
		ctx  context.Context
	}{
		{"anonymous", context.Background()},
		{"john@example.com, super_admin", signedIn("u-1", protoauthz.RoleSuperAdmin, "john@example.com")},
		{"jane@example.com, member", signedIn("u-2", protoauthz.RoleMember, "jane@example.com")},
	}
	for _, c := range callers {
		_, err := authz(c.ctx, nil, info, export)
		fmt.Printf("%s: %v\n", c.name, status.Code(err))
	}

	deny := protoauthz.NewAuthLevelInterceptor(protoauthz.LoadAuthLevelsFromRegistry(), nil)
	_, err := deny(callers[2].ctx, nil, info, export)
	fmt.Printf("jane@example.com, no callback: %v\n", status.Code(err))
}

func signedIn(userID, role, email string) context.Context {
	ctx := protoauthz.WithUserID(context.Background(), userID)
	return protoauthz.WithEmail(protoauthz.WithRole(ctx, role), email)
}
