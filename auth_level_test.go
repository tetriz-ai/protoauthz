// Copyright 2026 The protoauthz Authors
// SPDX-License-Identifier: MIT

package protoauthz

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	protoauthzv1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/v1"
)

// mockServerStream is a grpc.ServerStream that only carries a context.
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context { return m.ctx }

// caller builds the context an authentication step would leave behind.
func caller(userID, role string) context.Context {
	ctx := context.Background()
	if userID != "" {
		ctx = WithUserID(ctx, userID)
	}
	if role != "" {
		ctx = WithRole(ctx, role)
	}
	return ctx
}

var testLevels = AuthLevelMap{
	"/example.v1.Service/Public":         protoauthzv1.AuthLevel_AUTH_LEVEL_UNAUTHENTICATED,
	"/example.v1.Service/AnyUser":        protoauthzv1.AuthLevel_AUTH_LEVEL_AUTHENTICATED,
	"/example.v1.Service/AdminOnly":      protoauthzv1.AuthLevel_AUTH_LEVEL_ADMIN,
	"/example.v1.Service/SuperAdminOnly": protoauthzv1.AuthLevel_AUTH_LEVEL_SUPER_ADMIN,
	"/example.v1.Service/ZeroValue":      protoauthzv1.AuthLevel(0),
	"/example.v1.Service/FutureLevel":    protoauthzv1.AuthLevel(99),
	"/other.v1.Internal/InternalMethod":  protoauthzv1.AuthLevel_AUTH_LEVEL_ADMIN,
	// A malformed key: a name without the leading "/" is refused before the
	// map is read, so even this public level never lets it through.
	"example.v1.Service/Public": protoauthzv1.AuthLevel_AUTH_LEVEL_UNAUTHENTICATED,
}

// TestAuthLevelInterceptors runs every case through the unary and the stream
// interceptor, which must agree with each other and with the case.
func TestAuthLevelInterceptors(t *testing.T) {
	const examplePrefix = "/example.v1."
	var (
		anonymous     = caller("", "")
		member        = caller("u-1", "member")
		admin         = caller("u-2", "admin")
		superAdmin    = caller("u-3", "super_admin")
		noRole        = caller("u-4", "")
		unknownRole   = caller("u-5", "owner")
		roleButNoUser = caller("", "super_admin")
	)

	tests := []struct {
		name     string
		levels   AuthLevelMap
		prefixes []string
		method   string
		ctx      context.Context
		want     codes.Code
	}{
		{"member accesses authenticated endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/AnyUser", member, codes.OK},
		{"admin accesses authenticated endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/AnyUser", admin, codes.OK},
		{"super_admin accesses authenticated endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/AnyUser", superAdmin, codes.OK},
		{"user with no role accesses authenticated endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/AnyUser", noRole, codes.OK},

		{"member denied admin endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/AdminOnly", member, codes.PermissionDenied},
		{"admin accesses admin endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/AdminOnly", admin, codes.OK},
		{"super_admin accesses admin endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/AdminOnly", superAdmin, codes.OK},
		{"user with no role denied admin endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/AdminOnly", noRole, codes.PermissionDenied},
		{"role not on the ladder denied admin endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/AdminOnly", unknownRole, codes.PermissionDenied},

		{"member denied super_admin endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/SuperAdminOnly", member, codes.PermissionDenied},
		{"admin denied super_admin endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/SuperAdminOnly", admin, codes.PermissionDenied},
		{"super_admin accesses super_admin endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/SuperAdminOnly", superAdmin, codes.OK},

		{"anonymous accesses public endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/Public", anonymous, codes.OK},
		{"member accesses public endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/Public", member, codes.OK},

		// Every level but UNAUTHENTICATED needs an authenticated caller.
		{"anonymous denied authenticated endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/AnyUser", anonymous, codes.Unauthenticated},
		{"anonymous denied admin endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/AdminOnly", anonymous, codes.Unauthenticated},
		{"role without a user denied authenticated endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/AnyUser", roleButNoUser, codes.Unauthenticated},
		{"role without a user denied super_admin endpoint", testLevels, []string{examplePrefix}, "/example.v1.Service/SuperAdminOnly", roleButNoUser, codes.Unauthenticated},

		// The enum's zero value is AUTH_LEVEL_UNSPECIFIED and denies.
		{"zero value denied", testLevels, []string{examplePrefix}, "/example.v1.Service/ZeroValue", superAdmin, codes.PermissionDenied},
		// A level the interceptor does not know denies.
		{"unknown level denied", testLevels, []string{examplePrefix}, "/example.v1.Service/FutureLevel", superAdmin, codes.PermissionDenied},
		// Prefixes narrow only the rule for unannotated methods. A level that is
		// set but unenforceable denies outside them too.
		{"zero value outside the prefixes denied", testLevels, []string{"/other.v1."}, "/example.v1.Service/ZeroValue", superAdmin, codes.PermissionDenied},
		{"unknown level outside the prefixes denied", testLevels, []string{"/other.v1."}, "/example.v1.Service/FutureLevel", superAdmin, codes.PermissionDenied},

		{"unannotated method under a prefix rejected", testLevels, []string{examplePrefix}, "/example.v1.Service/Unknown", superAdmin, codes.Internal},
		{"unannotated method outside the prefixes passes through", testLevels, []string{examplePrefix}, "/other.v1.Internal/Unknown", anonymous, codes.OK},
		{"annotated method outside the prefixes still checked", testLevels, []string{examplePrefix}, "/other.v1.Internal/InternalMethod", member, codes.PermissionDenied},

		// A prefix is matched against the start of "/package.Service/Method",
		// and one given without the leading slash gets it.
		{"prefix without the leading slash: unannotated method under it rejected", testLevels, []string{"example.v1."}, "/example.v1.Service/Unknown", superAdmin, codes.Internal},
		{"prefix without the leading slash: method outside it passes through", testLevels, []string{"example.v1."}, "/other.v1.Internal/Unknown", anonymous, codes.OK},
		{"prefix without the leading slash: annotated method checked as usual", testLevels, []string{"example.v1."}, "/example.v1.Service/AnyUser", member, codes.OK},
		{"prefix matches only the start of the method name", testLevels, []string{"/Unknown"}, "/other.v1.Internal/Unknown", anonymous, codes.OK},

		// A method name must start with "/", as grpc.Server passes it. One that
		// does not is refused whatever the prefixes, even when the rest of the
		// name is annotated.
		{"method name without the leading slash rejected under prefixes", testLevels, []string{examplePrefix}, "example.v1.Service/AdminOnly", anonymous, codes.Internal},
		{"method name without the leading slash rejected with no prefixes", testLevels, nil, "example.v1.Service/AdminOnly", superAdmin, codes.Internal},

		// A prefix gets exactly one leading slash, so "//example.v1." still
		// covers "/example.v1.Service/..." rather than matching nothing.
		{"prefix with two leading slashes: unannotated method under it rejected", testLevels, []string{"//example.v1."}, "/example.v1.Service/Unknown", superAdmin, codes.Internal},
		{"prefix with three leading slashes: unannotated method under it rejected", testLevels, []string{"///example.v1."}, "/example.v1.Service/Unknown", superAdmin, codes.Internal},

		// Slash-only prefixes normalise to "/", which covers every method.
		{"prefix /: unannotated method rejected", testLevels, []string{"/"}, "/other.v1.Internal/Unknown", superAdmin, codes.Internal},
		{"prefix //: unannotated method rejected", testLevels, []string{"//"}, "/other.v1.Internal/Unknown", superAdmin, codes.Internal},
		{"prefix ///: unannotated method rejected", testLevels, []string{"///"}, "/other.v1.Internal/Unknown", superAdmin, codes.Internal},

		// Every prefix in the list counts, not only the first or the last.
		{"second of two prefixes: unannotated method under it rejected", testLevels, []string{"/zzz.", "/example.v1."}, "/example.v1.Service/Unknown", superAdmin, codes.Internal},
		{"second of two prefixes: method outside both passes", testLevels, []string{"/zzz.", "/example.v1."}, "/other.v1.Internal/Unknown", anonymous, codes.OK},
		{"first of two prefixes: unannotated method under it rejected", testLevels, []string{"/example.v1.", "/zzz."}, "/example.v1.Service/Unknown", superAdmin, codes.Internal},
		{"first of two prefixes: method outside both passes", testLevels, []string{"/example.v1.", "/zzz."}, "/other.v1.Internal/Unknown", anonymous, codes.OK},
		{"empty prefix beside another: method under the other rejected", testLevels, []string{"", "/example.v1."}, "/example.v1.Service/Unknown", superAdmin, codes.Internal},
		{"empty prefix beside another: every other method rejected too", testLevels, []string{"", "/example.v1."}, "/other.v1.Internal/Unknown", superAdmin, codes.Internal},

		// Prefixes are plain string prefixes of "/package.Service/Method".
		{"service prefix: unannotated method of that service rejected", testLevels, []string{"/example.v1.Service/"}, "/example.v1.Service/Unknown", superAdmin, codes.Internal},
		{"service prefix: another service of the package passes", testLevels, []string{"/example.v1.Service/"}, "/example.v1.Other/Unknown", anonymous, codes.OK},
		{"prefix without the trailing dot also covers a longer package", testLevels, []string{"/example.v1"}, "/example.v1beta.S/Unknown", superAdmin, codes.Internal},
		{"prefix /example.v1/ names service v1 of package example, not package example.v1", testLevels, []string{"/example.v1/"}, "/example.v1.Service/Unknown", anonymous, codes.OK},
		{"prefix /example.v1/ covers service v1 of package example", testLevels, []string{"/example.v1/"}, "/example.v1/Unknown", superAdmin, codes.Internal},
		// A method prefix without the leading slash, and a service prefix with
		// three, are counted after normalising and so are valid.
		{"prefix pkg.S/M: unannotated method it covers rejected", testLevels, []string{"pkg.S/M"}, "/pkg.S/M", superAdmin, codes.Internal},
		{"prefix pkg.S/M: method outside it passes", testLevels, []string{"pkg.S/M"}, "/pkg.S/N", anonymous, codes.OK},
		{"prefix ///pkg.S/: unannotated method it covers rejected", testLevels, []string{"///pkg.S/"}, "/pkg.S/Unknown", superAdmin, codes.Internal},
		{"prefix ///pkg.S/: method outside it passes", testLevels, []string{"///pkg.S/"}, "/pkg.T/Unknown", anonymous, codes.OK},
		{"full-method prefix /pkg.S/M: the unannotated method it names rejected", testLevels, []string{"/pkg.S/M"}, "/pkg.S/M", superAdmin, codes.Internal},

		// An empty prefix covers every method.
		{"empty prefix: unannotated method rejected", testLevels, []string{""}, "/other.v1.Internal/Unknown", superAdmin, codes.Internal},

		// With no prefixes, every unannotated method is denied.
		{"no prefixes: unannotated method rejected", testLevels, nil, "/other.v1.Internal/Unknown", superAdmin, codes.Internal},
		{"no prefixes: empty map rejects every method", AuthLevelMap{}, nil, "/x.v1.S/M", superAdmin, codes.Internal},
		{"no prefixes: nil map rejects every method", nil, nil, "/x.v1.S/M", superAdmin, codes.Internal},
		{"no prefixes: annotated method checked as usual", testLevels, nil, "/example.v1.Service/AnyUser", member, codes.OK},
		// gRPC's own health service carries no auth_level, so it is denied too.
		{"no prefixes: gRPC health check denied", testLevels, nil, "/grpc.health.v1.Health/Check", superAdmin, codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unaryCalled := false
			_, unaryErr := NewAuthLevelInterceptor(tt.levels, nil, tt.prefixes...)(
				tt.ctx, nil, &grpc.UnaryServerInfo{FullMethod: tt.method},
				func(context.Context, any) (any, error) { unaryCalled = true; return "ok", nil })

			streamCalled := false
			streamErr := NewAuthLevelStreamInterceptor(tt.levels, nil, tt.prefixes...)(
				nil, &mockServerStream{ctx: tt.ctx}, &grpc.StreamServerInfo{FullMethod: tt.method},
				func(any, grpc.ServerStream) error { streamCalled = true; return nil })

			for _, r := range []struct {
				kind   string
				err    error
				called bool
			}{{"unary", unaryErr, unaryCalled}, {"stream", streamErr, streamCalled}} {
				if got := status.Code(r.err); got != tt.want {
					t.Errorf("%s: code %v (%v), want %v", r.kind, got, r.err, tt.want)
				}
				if r.called != (tt.want == codes.OK) {
					t.Errorf("%s: handler called = %v, want %v", r.kind, r.called, tt.want == codes.OK)
				}
			}
		})
	}
}

// The result is sorted, so it is the same on every call.
func TestUnauthenticatedMethodsSorted(t *testing.T) {
	levels := AuthLevelMap{"/x.v1.S/Private": protoauthzv1.AuthLevel_AUTH_LEVEL_ADMIN}
	var want []string
	for i := range 20 {
		m := fmt.Sprintf("/x.v1.S/M%02d", i)
		levels[m] = protoauthzv1.AuthLevel_AUTH_LEVEL_UNAUTHENTICATED
		want = append(want, m)
	}
	for range 5 {
		if got := UnauthenticatedMethods(levels); !slices.Equal(got, want) {
			t.Fatalf("UnauthenticatedMethods = %v, want %v", got, want)
		}
	}
	if got := UnauthenticatedMethods(AuthLevelMap{}); len(got) != 0 {
		t.Fatalf("empty map: got %v", got)
	}
}

// TestMalformedMethodNames checks that a full method name that does not start
// with exactly one "/" is refused with Internal and the same message, whatever
// the prefixes, the caller and the levels map, through both interceptors.
func TestMalformedMethodNames(t *testing.T) {
	names := []string{
		"example.v1.Service/Public",      // in testLevels as UNAUTHENTICATED
		"example.v1.Service/AdminOnly",   // no leading slash
		"//example.v1.Service/AdminOnly", // two leading slashes
		"/",                              // no service or method
		"",                               // empty
		" /example.v1.Service/AdminOnly", // leading space
		"//",                             // two slashes and nothing else
		"///pkg.S/M",                     // three leading slashes
		"////",                           // only slashes
	}
	for _, name := range names {
		for setName, prefixes := range edgePrefixSets {
			for callerName, ctx := range map[string]context.Context{"anonymous": context.Background(), "super_admin": caller("u-3", "super_admin")} {
				t.Run(fmt.Sprintf("%q %s %s", name, setName, callerName), func(t *testing.T) {
					want := fmt.Sprintf("malformed method name %q: it must start with a single \"/\" followed by the service", name)
					_, uErr := NewAuthLevelInterceptor(testLevels, nil, prefixes...)(ctx, nil, &grpc.UnaryServerInfo{FullMethod: name},
						func(context.Context, any) (any, error) { return "ok", nil })
					sErr := NewAuthLevelStreamInterceptor(testLevels, nil, prefixes...)(nil, &mockServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: name},
						func(any, grpc.ServerStream) error { return nil })
					for kind, err := range map[string]error{"unary": uErr, "stream": sErr} {
						if st := status.Convert(err); st.Code() != codes.Internal || st.Message() != want {
							t.Errorf("%s: %v %q, want Internal %q", kind, st.Code(), st.Message(), want)
						}
					}
				})
			}
		}
	}
}

// edgePrefixSets are the prefix lists the method-name tests run under,
// including the forms the policy normalises.
var edgePrefixSets = map[string][]string{
	"no prefixes":          nil,
	"example prefix":       {"/example.v1."},
	"empty prefix":         {""},
	"prefix without slash": {"example.v1."},
	"slash prefix":         {"/"},
}

// TestWellFormedEdgeNames checks that unusual but well-formed names (one
// leading "/" and something after it) are judged by their level, not
// refused, under every prefix set and through both interceptors.
func TestWellFormedEdgeNames(t *testing.T) {
	levels := AuthLevelMap{
		"/p":        protoauthzv1.AuthLevel_AUTH_LEVEL_AUTHENTICATED,
		"/pkg.S//M": protoauthzv1.AuthLevel_AUTH_LEVEL_ADMIN,
		"/pkg.S/M/": protoauthzv1.AuthLevel_AUTH_LEVEL_UNAUTHENTICATED,
	}
	want := map[string]codes.Code{"/p": codes.OK, "/pkg.S//M": codes.PermissionDenied, "/pkg.S/M/": codes.OK}
	member := caller("u-1", "member")
	for name, code := range want {
		for setName, prefixes := range edgePrefixSets {
			t.Run(fmt.Sprintf("%q %s", name, setName), func(t *testing.T) {
				_, uErr := NewAuthLevelInterceptor(levels, nil, prefixes...)(member, nil, &grpc.UnaryServerInfo{FullMethod: name},
					func(context.Context, any) (any, error) { return "ok", nil })
				sErr := NewAuthLevelStreamInterceptor(levels, nil, prefixes...)(nil, &mockServerStream{ctx: member}, &grpc.StreamServerInfo{FullMethod: name},
					func(any, grpc.ServerStream) error { return nil })
				for kind, err := range map[string]error{"unary": uErr, "stream": sErr} {
					if got := status.Code(err); got != code {
						t.Errorf("%s: %v (%v), want %v", kind, got, err, code)
					}
				}
			})
		}
	}
}
