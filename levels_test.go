// Copyright 2026 The protoauthz Authors
// SPDX-License-Identifier: MIT

package protoauthz

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"

	draftv1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/draft/v1"
	examplev1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	_ "google.golang.org/protobuf/types/known/emptypb" // registers google/protobuf/empty.proto

	protoauthzv1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/v1"
)

// both runs one call through the unary and the stream interceptor built from
// the same ladder and arguments, and fails the test if they disagree.
func both(ctx context.Context, t *testing.T, l Ladder, levels AuthLevelMap, allow AllowlistCheck, method string) codes.Code {
	t.Helper()
	_, uErr := l.UnaryInterceptor(levels, allow)(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method},
		func(context.Context, any) (any, error) { return nil, nil })
	sErr := l.StreamInterceptor(levels, allow)(nil, &mockServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: method},
		func(any, grpc.ServerStream) error { return nil })
	if status.Code(uErr) != status.Code(sErr) {
		t.Fatalf("unary %v and stream %v disagree", uErr, sErr)
	}
	return status.Code(uErr)
}

func TestAllowlistLevel(t *testing.T) {
	const method = "/x.v1.S/Export"
	levels := AuthLevelMap{method: protoauthzv1.AuthLevel_AUTH_LEVEL_ALLOWLIST}
	staff := WithEmail(caller("u-1", "member"), "jane@example.com")

	var gotEmail, gotUser string
	recordAndAllow := func(_ context.Context, email, userID string) (bool, error) {
		gotEmail, gotUser = email, userID
		return email == "jane@example.com", nil
	}
	failing := func(context.Context, string, string) (bool, error) { return false, errors.New("store down") }

	tests := []struct {
		name  string
		allow AllowlistCheck
		ctx   context.Context
		want  codes.Code
	}{
		{"callback accepts", recordAndAllow, staff, codes.OK},
		{"callback refuses", recordAndAllow, WithEmail(caller("u-2", "super_admin"), "john@example.com"), codes.PermissionDenied},
		{"callback error", failing, staff, codes.Internal},
		{"callback error with a yes is still an error", func(context.Context, string, string) (bool, error) {
			return true, errors.New("store down")
		}, staff, codes.Internal},
		{"nil callback denies", nil, staff, codes.PermissionDenied},
		{"DenyAllAllowlist denies", DenyAllAllowlist, staff, codes.PermissionDenied},
		{"anonymous never reaches the callback", recordAndAllow, WithEmail(context.Background(), "jane@example.com"), codes.Unauthenticated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEmail, gotUser = "", ""
			if got := both(tt.ctx, t, DefaultLadder(), levels, tt.allow, method); got != tt.want {
				t.Fatalf("code %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("callback receives the context's email and user ID", func(t *testing.T) {
		both(staff, t, DefaultLadder(), levels, recordAndAllow, method)
		if gotEmail != "jane@example.com" || gotUser != "u-1" {
			t.Fatalf("callback got (%q, %q)", gotEmail, gotUser)
		}
	})
	t.Run("error text does not carry the callback's error", func(t *testing.T) {
		_, err := DefaultLadder().UnaryInterceptor(levels, failing)(staff, nil, &grpc.UnaryServerInfo{FullMethod: method},
			func(context.Context, any) (any, error) { return nil, nil })
		if strings.Contains(err.Error(), "store down") {
			t.Fatalf("error leaks the callback's error: %v", err)
		}
	})
}

func TestNewLadder(t *testing.T) {
	tests := []struct {
		name              string
		roles             []string
		admin, superAdmin string
		wantErr           string
	}{
		{"valid", []string{"viewer", "editor", "owner"}, "editor", "owner", ""},
		{"admin and super admin the same role", []string{"viewer", "owner"}, "owner", "owner", ""},
		{"no roles", nil, "a", "b", "has no roles"},
		{"empty role name", []string{"viewer", ""}, "viewer", "viewer", "empty role name"},
		{"duplicate role", []string{"viewer", "viewer"}, "viewer", "viewer", `"viewer" twice`},
		{"admin not listed", []string{"viewer", "owner"}, "editor", "owner", `"editor" is not on the ladder`},
		{"super admin not listed", []string{"viewer", "editor"}, "editor", "owner", `"owner" is not on the ladder`},
		{"super admin below admin", []string{"viewer", "editor", "owner"}, "owner", "editor", "ranks below"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewLadder(tt.roles, tt.admin, tt.superAdmin)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLadderRanksRoles(t *testing.T) {
	custom, err := NewLadder([]string{"viewer", "editor", "owner"}, "editor", "owner")
	if err != nil {
		t.Fatal(err)
	}
	lowestAdmin, err := NewLadder([]string{"editor", "owner"}, "editor", "owner")
	if err != nil {
		t.Fatal(err)
	}
	levels := AuthLevelMap{
		"/x.v1.S/Edit":  protoauthzv1.AuthLevel_AUTH_LEVEL_ADMIN,
		"/x.v1.S/Own":   protoauthzv1.AuthLevel_AUTH_LEVEL_SUPER_ADMIN,
		"/x.v1.S/Read":  protoauthzv1.AuthLevel_AUTH_LEVEL_AUTHENTICATED,
		"/x.v1.S/Hello": protoauthzv1.AuthLevel_AUTH_LEVEL_UNAUTHENTICATED,
	}
	tests := []struct {
		name   string
		ladder Ladder
		role   string
		method string
		want   codes.Code
	}{
		{"custom: viewer denied admin level", custom, "viewer", "/x.v1.S/Edit", codes.PermissionDenied},
		{"custom: editor passes admin level", custom, "editor", "/x.v1.S/Edit", codes.OK},
		{"custom: owner passes admin level", custom, "owner", "/x.v1.S/Edit", codes.OK},
		{"custom: editor denied super admin level", custom, "editor", "/x.v1.S/Own", codes.PermissionDenied},
		{"custom: owner passes super admin level", custom, "owner", "/x.v1.S/Own", codes.OK},
		{"custom: default role names mean nothing", custom, "super_admin", "/x.v1.S/Edit", codes.PermissionDenied},
		{"custom: roles are case-sensitive", custom, "Owner", "/x.v1.S/Edit", codes.PermissionDenied},
		{"admin role lowest: role not on the ladder denied admin level", lowestAdmin, "stranger", "/x.v1.S/Edit", codes.PermissionDenied},
		{"admin role lowest: empty role denied admin level", lowestAdmin, "", "/x.v1.S/Edit", codes.PermissionDenied},
		{"admin role lowest: editor passes admin level", lowestAdmin, "editor", "/x.v1.S/Edit", codes.OK},
		{"empty Ladder{} denies admin level", Ladder{}, "admin", "/x.v1.S/Edit", codes.PermissionDenied},
		{"empty Ladder{} denies super admin level", Ladder{}, "", "/x.v1.S/Own", codes.PermissionDenied},
		{"empty Ladder{} still admits authenticated level", Ladder{}, "", "/x.v1.S/Read", codes.OK},
		{"empty Ladder{} still admits public level", Ladder{}, "", "/x.v1.S/Hello", codes.OK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := both(caller("u-1", tt.role), t, tt.ladder, levels, nil, tt.method); got != tt.want {
				t.Fatalf("code %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultLadderMatchesRoleConstants(t *testing.T) {
	l := DefaultLadder()
	if l.admin != RoleAdmin || l.superAdmin != RoleSuperAdmin {
		t.Fatalf("admin %q, super admin %q", l.admin, l.superAdmin)
	}
	if l.rank[RoleMember] >= l.rank[RoleAdmin] || l.rank[RoleAdmin] >= l.rank[RoleSuperAdmin] {
		t.Fatalf("ranks %v", l.rank)
	}
}

func TestInterceptorCopiesItsInputs(t *testing.T) {
	levels := AuthLevelMap{"/x.v1.S/M": protoauthzv1.AuthLevel_AUTH_LEVEL_ADMIN}
	prefixes := []string{"/x.v1."}
	unary := NewAuthLevelInterceptor(levels, nil, prefixes...)
	stream := NewAuthLevelStreamInterceptor(levels, nil, prefixes...)
	levels["/x.v1.S/M"] = protoauthzv1.AuthLevel_AUTH_LEVEL_UNAUTHENTICATED
	prefixes[0] = "/nothing."
	for _, method := range []string{"/x.v1.S/M", "/x.v1.S/Other"} {
		_, err := unary(caller("u-1", "member"), nil, &grpc.UnaryServerInfo{FullMethod: method},
			func(context.Context, any) (any, error) { return nil, nil })
		if err == nil {
			t.Fatalf("unary %s: a change to the caller's map or slice after construction took effect", method)
		}
		err = stream(nil, &mockServerStream{ctx: caller("u-1", "member")}, &grpc.StreamServerInfo{FullMethod: method},
			func(any, grpc.ServerStream) error { return nil })
		if err == nil {
			t.Fatalf("stream %s: a change to the caller's map or slice after construction took effect", method)
		}
	}
}

func TestErrorMessages(t *testing.T) {
	levels := AuthLevelMap{
		"/x.v1.S/Admin":  protoauthzv1.AuthLevel_AUTH_LEVEL_ADMIN,
		"/x.v1.S/List":   protoauthzv1.AuthLevel_AUTH_LEVEL_ALLOWLIST,
		"/x.v1.S/Zero":   protoauthzv1.AuthLevel_AUTH_LEVEL_UNSPECIFIED,
		"/x.v1.S/Future": protoauthzv1.AuthLevel(99),
	}
	failing := func(context.Context, string, string) (bool, error) { return false, errors.New("store down") }
	tests := []struct {
		ladder Ladder
		allow  AllowlistCheck
		method string
		ctx    context.Context
		want   string
	}{
		{DefaultLadder(), nil, "/x.v1.S/Admin", caller("u-1", "member"), "admin role required"},
		{DefaultLadder(), nil, "/x.v1.S/Admin", context.Background(), "authentication required"},
		{DefaultLadder(), nil, "/x.v1.S/Zero", caller("u-1", "member"), "auth_level AUTH_LEVEL_UNSPECIFIED on /x.v1.S/Zero is denied"},
		{DefaultLadder(), nil, "/x.v1.S/Future", caller("u-1", "member"), "auth_level 99 on /x.v1.S/Future is denied"},
		{DefaultLadder(), nil, "/x.v1.S/Missing", caller("u-1", "member"), "missing auth_level annotation on /x.v1.S/Missing (annotate the RPC, or import its generated package before loading the levels)"},
		{DefaultLadder(), nil, "x.v1.S/Admin", caller("u-1", "member"), `malformed method name "x.v1.S/Admin": it must start with a single "/" followed by the service`},
		{DefaultLadder(), nil, "/x.v1.S/List", caller("u-1", "member"), "caller is not on the allow-list"},
		{DefaultLadder(), failing, "/x.v1.S/List", caller("u-1", "member"), "allow-list check failed"},
		{Ladder{}, nil, "/x.v1.S/Admin", caller("u-1", "admin"), "no role on the ladder admits this call"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.want, func(t *testing.T) {
			_, err := tt.ladder.UnaryInterceptor(levels, tt.allow)(tt.ctx, nil, &grpc.UnaryServerInfo{FullMethod: tt.method},
				func(context.Context, any) (any, error) { return nil, nil })
			if got := status.Convert(err).Message(); got != tt.want {
				t.Fatalf("message %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadAuthLevelsFromRegistry(t *testing.T) {
	walkerFile(t)
	levels := LoadAuthLevelsFromRegistry()
	want := map[string]protoauthzv1.AuthLevel{
		"/protoauthz.example.v1.NoteService/GetVersion":      protoauthzv1.AuthLevel_AUTH_LEVEL_UNAUTHENTICATED,
		"/protoauthz.example.v1.NoteService/ListNotes":       protoauthzv1.AuthLevel_AUTH_LEVEL_AUTHENTICATED,
		"/protoauthz.example.v1.NoteService/WatchNotes":      protoauthzv1.AuthLevel_AUTH_LEVEL_AUTHENTICATED,
		"/protoauthz.example.v1.NoteService/DeleteNote":      protoauthzv1.AuthLevel_AUTH_LEVEL_ADMIN,
		"/protoauthz.example.v1.NoteService/PurgeNotes":      protoauthzv1.AuthLevel_AUTH_LEVEL_SUPER_ADMIN,
		"/protoauthz.example.v1.NoteService/ExportNotes":     protoauthzv1.AuthLevel_AUTH_LEVEL_ALLOWLIST,
		"/protoauthz.example.draft.v1.NoteService/ListNotes": protoauthzv1.AuthLevel_AUTH_LEVEL_AUTHENTICATED,
	}
	for method, level := range want {
		if got, ok := levels[method]; !ok || got != level {
			t.Errorf("%s: got %v (present %v), want %v", method, got, ok, level)
		}
	}
	for _, m := range []string{"/protoauthz.example.draft.v1.NoteService/DeleteNote", "/walker.v1.WalkerService/Bare", "/walker.v1.WalkerService/OtherOption"} {
		if _, ok := levels[m]; ok {
			t.Errorf("%s has no auth_level but was loaded", m)
		}
	}
	for m, l := range map[string]protoauthzv1.AuthLevel{"/walker.v1.WalkerService/Zero": 0, "/walker.v1.WalkerService/Future": 99} {
		if got, ok := levels[m]; !ok || got != l {
			t.Errorf("%s: got %v (present %v), want %v", m, got, ok, l)
		}
	}
}

func TestMissingAuthLevels(t *testing.T) {
	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{"draft service names its one unannotated RPC", MissingAuthLevels(draftv1.File_protoauthz_example_draft_v1_notes_proto),
			[]string{"/protoauthz.example.draft.v1.NoteService/DeleteNote"}},
		{"annotated service passes", MissingAuthLevels(examplev1.File_protoauthz_example_v1_notes_proto), nil},
		{"a file with no services passes", MissingAuthLevels(protoauthzv1.File_protoauthz_v1_auth_level_proto), nil},
		{"no files", MissingAuthLevels(), nil},
		{"a nil descriptor is skipped", MissingAuthLevels(nil), nil},
		{"a nil descriptor beside a real one is skipped", MissingAuthLevels(nil, draftv1.File_protoauthz_example_draft_v1_notes_proto),
			[]string{"/protoauthz.example.draft.v1.NoteService/DeleteNote"}},
		{"the same file twice is reported once", MissingAuthLevels(draftv1.File_protoauthz_example_draft_v1_notes_proto, draftv1.File_protoauthz_example_draft_v1_notes_proto),
			[]string{"/protoauthz.example.draft.v1.NoteService/DeleteNote"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !slices.Equal(tt.got, tt.want) {
				t.Fatalf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

// walkerFile builds a service in memory with one method per kind of
// annotation, including a method that sets another option but not auth_level.
func walkerFile(t *testing.T) protoreflect.FileDescriptor {
	t.Helper()
	walkerOnce.Do(func() { walkerFD, walkerErr = buildWalkerFile() })
	if walkerErr != nil {
		t.Fatal(walkerErr)
	}
	return walkerFD
}

var (
	walkerOnce sync.Once
	walkerFD   protoreflect.FileDescriptor
	walkerErr  error
)

// buildWalkerFile builds the file and registers it in the global registry, so
// LoadAuthLevelsFromRegistry sees it too.
func buildWalkerFile() (protoreflect.FileDescriptor, error) {
	opts := func(set func(*descriptorpb.MethodOptions)) *descriptorpb.MethodOptions {
		o := &descriptorpb.MethodOptions{}
		set(o)
		return o
	}
	level := func(l protoauthzv1.AuthLevel) *descriptorpb.MethodOptions {
		return opts(func(o *descriptorpb.MethodOptions) { proto.SetExtension(o, protoauthzv1.E_AuthLevel, l) })
	}
	method := func(name string, o *descriptorpb.MethodOptions) *descriptorpb.MethodDescriptorProto {
		return &descriptorpb.MethodDescriptorProto{
			Name: proto.String(name), InputType: proto.String(".google.protobuf.Empty"),
			OutputType: proto.String(".google.protobuf.Empty"), Options: o,
		}
	}
	fdp := &descriptorpb.FileDescriptorProto{
		Name:       proto.String("walker/v1/walker.proto"),
		Package:    proto.String("walker.v1"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"google/protobuf/empty.proto"},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: proto.String("WalkerService"),
			Method: []*descriptorpb.MethodDescriptorProto{
				method("Bare", nil),
				method("OtherOption", opts(func(o *descriptorpb.MethodOptions) { o.Deprecated = proto.Bool(true) })),
				method("Zero", level(protoauthzv1.AuthLevel_AUTH_LEVEL_UNSPECIFIED)),
				method("Future", level(protoauthzv1.AuthLevel(99))),
				method("Good", level(protoauthzv1.AuthLevel_AUTH_LEVEL_AUTHENTICATED)),
			},
		}},
	}
	fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	if err != nil {
		return nil, err
	}
	return fd, protoregistry.GlobalFiles.RegisterFile(fd)
}

func TestMissingAuthLevelsEveryKind(t *testing.T) {
	want := []string{
		"/walker.v1.WalkerService/Bare",
		"/walker.v1.WalkerService/Future",
		"/walker.v1.WalkerService/OtherOption",
		"/walker.v1.WalkerService/Zero",
	}
	if got := MissingAuthLevels(walkerFile(t)); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMissingAuthLevelsFromRegistry(t *testing.T) {
	// Register the in-memory walker and pkg.v10 services and reference the
	// generated example, draft, health and reflection services, so all of them
	// sit in the global registry.
	walkerFile(t)
	pkgV10File(t)
	_ = examplev1.File_protoauthz_example_v1_notes_proto
	_ = draftv1.File_protoauthz_example_draft_v1_notes_proto
	_ = healthpb.File_grpc_health_v1_health_proto
	_ = reflectionpb.File_grpc_reflection_v1_reflection_proto
	// The walker's UNSPECIFIED and unknown levels are reported under every
	// prefix, since the interceptors deny them whatever the prefixes.
	plus := func(methods ...string) []string {
		all := append([]string{"/walker.v1.WalkerService/Future", "/walker.v1.WalkerService/Zero"}, methods...)
		slices.Sort(all)
		return slices.Compact(all)
	}
	const deleteNote = "/protoauthz.example.draft.v1.NoteService/DeleteNote"
	tests := []struct {
		name     string
		prefixes []string
		want     []string
	}{
		{"walker prefix names its unenforceable kinds", []string{"/walker.v1."},
			plus("/walker.v1.WalkerService/Bare", "/walker.v1.WalkerService/OtherOption")},
		{"a package prefix passes its annotated service and not its draft sibling", []string{"/protoauthz.example.v1."}, plus()},
		{"draft prefix names its one unannotated RPC", []string{"/protoauthz.example.draft.v1."}, plus(deleteNote)},
		{"a prefix without its leading slash matches as the interceptors do", []string{"protoauthz.example.draft.v1."}, plus(deleteNote)},
		{"a prefix with several leading slashes gets exactly one", []string{"///protoauthz.example.draft.v1."}, plus(deleteNote)},
		{"two prefixes union their results", []string{"/walker.v1.WalkerService/Bare", "/protoauthz.example.draft.v1."},
			plus(deleteNote, "/walker.v1.WalkerService/Bare")},
		{"a prefix that matches nothing adds no unannotated RPC", []string{"/no.such.v1."}, plus()},
		{"a prefix ending after the package dot leaves out pkg.v10", []string{"/pkg.v1."}, plus()},
		{"a prefix ending at pkg.v1 also covers pkg.v10", []string{"/pkg.v1"}, plus("/pkg.v10.S/M")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MissingAuthLevelsFromRegistry(tt.prefixes...); !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}

	// gRPC's own services carry no auth_level. With no prefixes they are
	// reported, and a package prefix leaves them out.
	grpcOwn := []string{"/grpc.health.v1.Health/Check", "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo"}
	all := MissingAuthLevelsFromRegistry()
	scoped := MissingAuthLevelsFromRegistry("/protoauthz.example.v1.")
	for _, method := range grpcOwn {
		if !slices.Contains(all, method) {
			t.Errorf("no prefixes: %s not reported in %v", method, all)
		}
		if slices.Contains(scoped, method) {
			t.Errorf("package prefix: %s reported in %v", method, scoped)
		}
	}
}

// pkgV10File registers an in-memory service in package pkg.v10 with one
// unannotated method, to check where a prefix ends.
func pkgV10File(t *testing.T) {
	t.Helper()
	pkgV10Once.Do(func() {
		fdp := &descriptorpb.FileDescriptorProto{
			Name:       proto.String("pkg/v10/pkg.proto"),
			Package:    proto.String("pkg.v10"),
			Syntax:     proto.String("proto3"),
			Dependency: []string{"google/protobuf/empty.proto"},
			Service: []*descriptorpb.ServiceDescriptorProto{{
				Name: proto.String("S"),
				Method: []*descriptorpb.MethodDescriptorProto{{
					Name: proto.String("M"), InputType: proto.String(".google.protobuf.Empty"),
					OutputType: proto.String(".google.protobuf.Empty"),
				}},
			}},
		}
		fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
		if err == nil {
			err = protoregistry.GlobalFiles.RegisterFile(fd)
		}
		pkgV10Err = err
	})
	if pkgV10Err != nil {
		t.Fatal(pkgV10Err)
	}
}

var (
	pkgV10Once sync.Once
	pkgV10Err  error
)

func TestContextValues(t *testing.T) {
	ctx := context.Background()
	if UserIDFromContext(ctx) != "" || RoleFromContext(ctx) != "" || EmailFromContext(ctx) != "" {
		t.Fatal("empty context carries a value")
	}
	ctx = WithEmail(WithRole(WithUserID(ctx, "u-1"), "admin"), "jane@example.com")
	if UserIDFromContext(ctx) != "u-1" || RoleFromContext(ctx) != "admin" || EmailFromContext(ctx) != "jane@example.com" {
		t.Fatalf("round trip lost a value: %q %q %q", UserIDFromContext(ctx), RoleFromContext(ctx), EmailFromContext(ctx))
	}
	// A plain string key with the same text does not collide with this
	// package's keys. The keys go through an any so the linters, which reject
	// a string key on sight, let the check compile; their dynamic type is string.
	plain := context.Background()
	for key, value := range map[any]string{"user_id": "u-9", "role": "admin", "email": "x@example.com"} {
		plain = context.WithValue(plain, key, value)
	}
	if UserIDFromContext(plain) != "" || RoleFromContext(plain) != "" || EmailFromContext(plain) != "" {
		t.Fatalf("a plain string key was read: %q %q %q", UserIDFromContext(plain), RoleFromContext(plain), EmailFromContext(plain))
	}
}

// A prefix holding a byte or a "/" that no method name can hold panics, for
// every constructor.
func TestInvalidPrefixPanics(t *testing.T) {
	constructors := map[string]func(prefixes ...string){
		"NewAuthLevelInterceptor":       func(p ...string) { NewAuthLevelInterceptor(nil, nil, p...) },
		"NewAuthLevelStreamInterceptor": func(p ...string) { NewAuthLevelStreamInterceptor(nil, nil, p...) },
		"Ladder.UnaryInterceptor":       func(p ...string) { DefaultLadder().UnaryInterceptor(nil, nil, p...) },
		"Ladder.StreamInterceptor":      func(p ...string) { DefaultLadder().StreamInterceptor(nil, nil, p...) },
		"MissingAuthLevelsFromRegistry": func(p ...string) { MissingAuthLevelsFromRegistry(p...) },
	}
	invalid := []struct {
		name     string
		prefixes []string
		bad      string // the prefix the panic names
	}{
		{"space first", []string{" /pkg."}, " /pkg."},
		{"tab first", []string{"\t/pkg."}, "\t/pkg."},
		{"trailing space", []string{"/pkg. "}, "/pkg. "},
		{"hyphen", []string{"/pkg-x."}, "/pkg-x."},
		{"non-ASCII letter", []string{"/pkg\u00e9."}, "/pkg\u00e9."},
		{"division slash", []string{"/pkg\u2215x."}, "/pkg\u2215x."},
		// One byte either side of each allowed range, and other punctuation.
		{"colon, after 9", []string{"/x:"}, "/x:"},
		{"at sign, before A", []string{"/x@"}, "/x@"},
		{"bracket, after Z", []string{"/x["}, "/x["},
		{"backtick, before a", []string{"/x`"}, "/x`"},
		{"brace, after z", []string{"/x{"}, "/x{"},
		{"star", []string{"/x*"}, "/x*"},
		{"dollar", []string{"/x$"}, "/x$"},
		{"newline", []string{"/x\n"}, "/x\n"},
		{"NUL", []string{"/x\x00"}, "/x\x00"},
		// Allowed bytes only, but no method name can start with them.
		{"double slash after a service", []string{"/x.v1.S//"}, "/x.v1.S//"},
		{"slash after a method", []string{"/x.v1.S/M/"}, "/x.v1.S/M/"},
		{"text after a method", []string{"/x.v1.S/M/extra"}, "/x.v1.S/M/extra"},
		{"double slash inside", []string{"/x.v1//S/"}, "/x.v1//S/"},
		// The count applies after normalising, whatever the raw leading slashes.
		{"text after a method, no leading slash", []string{"pkg.S/M/x"}, "pkg.S/M/x"},
		{"slash after a method, two leading slashes", []string{"//x.v1.S/M/"}, "//x.v1.S/M/"},
		// Every prefix of a list is checked, not only the first.
		{"invalid prefix last in a list", []string{"/a.", "/b.", "/x v1."}, "/x v1."},
	}
	for _, tt := range invalid {
		for name, build := range constructors {
			t.Run(name+" "+tt.name, func(t *testing.T) {
				want := fmt.Sprintf(`protoauthz: invalid prefix %q: a prefix may hold only ASCII letters, digits, "_", "." and "/", and after normalising to one leading "/", no "//" and at most two "/"`, tt.bad)
				defer func() {
					if got := recover(); got != want {
						t.Fatalf("panic %v, want %q", got, want)
					}
				}()
				build(tt.prefixes...)
			})
		}
	}
	valid := []string{"", "/", "example.v1.", "//example.v1.", "/example.v1.Service/", "/example.v1.Service/Method", "/x_y.v1.", "/AZaz09_.Z/", "pkg.S/M", "///pkg.S/"}
	for _, prefix := range valid {
		t.Run(fmt.Sprintf("valid %q", prefix), func(*testing.T) {
			for _, build := range constructors {
				build(prefix) // must not panic
			}
		})
	}
}

// The interceptors deny an RPC whose level is set but UNSPECIFIED or unknown,
// whatever the prefixes, so the registry check reports it outside them too.
// Prefixes narrow only the report of RPCs with no level at all.
func TestMissingAuthLevelsFromRegistryReportsUnenforceableLevelsEverywhere(t *testing.T) {
	walkerFile(t)
	_ = draftv1.File_protoauthz_example_draft_v1_notes_proto
	want := []string{
		"/protoauthz.example.draft.v1.NoteService/DeleteNote",
		"/walker.v1.WalkerService/Future",
		"/walker.v1.WalkerService/Zero",
	}
	if got := MissingAuthLevelsFromRegistry("/protoauthz."); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
