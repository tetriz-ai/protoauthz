// Copyright 2026 The protoauthz Authors
// SPDX-License-Identifier: MIT

package protoauthz

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	protoauthzv1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/v1"
	"github.com/tetriz-ai/protoauthz/internal/prefix"
)

// AuthLevelMap maps gRPC full method names ("/package.Service/Method") to
// their required auth level.
type AuthLevelMap map[string]protoauthzv1.AuthLevel

// AllowlistCheck reports whether the authenticated caller, identified by the
// email and user ID in the context, may call an AUTH_LEVEL_ALLOWLIST RPC.
// An error denies the call with codes.Internal. email is "" when the login
// step never called WithEmail, so a callback must not treat "" as a match.
type AllowlistCheck func(ctx context.Context, email, userID string) (bool, error)

// DenyAllAllowlist rejects every AUTH_LEVEL_ALLOWLIST call. A nil callback
// falls back to it.
func DenyAllAllowlist(context.Context, string, string) (bool, error) {
	return false, nil
}

// LoadAuthLevelsFromRegistry reads the auth_level option of every method in
// the global proto registry. Generated code registers its descriptors when its
// package is initialised, so call it after importing the generated packages.
// Every method that sets the option is stored, whatever its value.
func LoadAuthLevelsFromRegistry() AuthLevelMap {
	levels := make(AuthLevelMap)
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		forEachMethod(fd, func(fullMethod string, md protoreflect.MethodDescriptor) {
			if level, ok := authLevelOf(md); ok {
				levels[fullMethod] = level
			}
		})
		return true
	})
	return levels
}

// MissingAuthLevels returns, sorted, the full method name of every RPC in the
// given files whose auth_level the interceptors deny outright: no option at
// all, AUTH_LEVEL_UNSPECIFIED, or a value this package does not know. A nil
// file is skipped, and a file with no services adds nothing.
func MissingAuthLevels(files ...protoreflect.FileDescriptor) []string {
	var missing []string
	for _, fd := range files {
		if fd == nil {
			continue
		}
		forEachMethod(fd, func(fullMethod string, md protoreflect.MethodDescriptor) {
			level, ok := authLevelOf(md)
			if !ok || !enforceable(level) {
				missing = append(missing, fullMethod)
			}
		})
	}
	slices.Sort(missing)
	return slices.Compact(missing)
}

// MissingAuthLevelsFromRegistry is MissingAuthLevels over every RPC in the
// global proto registry instead of a listed set of files. It returns, sorted,
// each method that the interceptors deny for its level. An RPC set to
// AUTH_LEVEL_UNSPECIFIED or to a value this package does not know is reported
// wherever it is, since the interceptors deny it whatever the prefixes. An RPC
// with no option is reported only under one of the prefixes, or anywhere when
// there are none, as the interceptors deny it only there.
//
// It sees only the services linked into the binary, as
// LoadAuthLevelsFromRegistry does. Call it where every service you serve is
// imported, such as the package that registers them on your grpc.Server. Pass
// the prefixes your interceptors take ("/your.pkg.v1."), which keeps gRPC's
// own health and reflection services out. It panics on a prefix that can match
// no method name, as the interceptors do. A prefix with no linked method under
// it adds nothing, and protoauthztest.RequireAuthLevelsFromRegistry fails on
// one.
func MissingAuthLevelsFromRegistry(prefixes ...string) []string {
	normalised := prefix.Normalise(prefixes)
	var missing []string
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		forEachMethod(fd, func(fullMethod string, md protoreflect.MethodDescriptor) {
			level, ok := authLevelOf(md)
			switch {
			case !ok && prefix.Under(fullMethod, normalised):
				missing = append(missing, fullMethod)
			case ok && !enforceable(level):
				missing = append(missing, fullMethod)
			}
		})
		return true
	})
	slices.Sort(missing)
	return slices.Compact(missing)
}

// UnauthenticatedMethods returns, sorted, the methods annotated with
// AUTH_LEVEL_UNAUTHENTICATED.
func UnauthenticatedMethods(levels AuthLevelMap) []string {
	var methods []string
	for method, level := range levels {
		if level == protoauthzv1.AuthLevel_AUTH_LEVEL_UNAUTHENTICATED {
			methods = append(methods, method)
		}
	}
	slices.Sort(methods)
	return methods
}

// NewAuthLevelInterceptor returns a unary interceptor that enforces each
// method's auth_level with DefaultLadder. See Ladder.UnaryInterceptor for the
// rules, including the prefixes that make it panic.
func NewAuthLevelInterceptor(levels AuthLevelMap, allowlist AllowlistCheck, prefixes ...string) grpc.UnaryServerInterceptor {
	return DefaultLadder().UnaryInterceptor(levels, allowlist, prefixes...)
}

// NewAuthLevelStreamInterceptor is NewAuthLevelInterceptor for streaming RPCs.
func NewAuthLevelStreamInterceptor(levels AuthLevelMap, allowlist AllowlistCheck, prefixes ...string) grpc.StreamServerInterceptor {
	return DefaultLadder().StreamInterceptor(levels, allowlist, prefixes...)
}

// UnaryInterceptor returns a unary interceptor that enforces each method's
// auth_level from levels, ranking roles on l.
//
// A full method name that does not start with a single "/" followed by the
// service is denied with
// codes.Internal, whatever the prefixes and levels. A well-formed method
// missing from levels is denied with codes.Internal too. When prefixes are
// given, only methods under one of them are held to that. Any other
// well-formed method missing from levels passes through unchecked.
//
// A prefix is matched against the start of the full method name,
// "/package.Service/Method". A prefix gets exactly one leading "/", so
// "example.v1.", "/example.v1." and "//example.v1." are the same prefix, and
// "" covers every method. A prefix may hold only ASCII letters, digits, "_",
// "." and "/", with one leading "/", no "//", and at most two "/". The
// constructor panics on any other prefix, such as one with a space or
// "/x.v1.S/M/", because such a prefix can match no method name.
//
// allowlist decides AUTH_LEVEL_ALLOWLIST calls. Nil denies them all.
func (l Ladder) UnaryInterceptor(levels AuthLevelMap, allowlist AllowlistCheck, prefixes ...string) grpc.UnaryServerInterceptor {
	p := newPolicy(l, levels, allowlist, prefixes)
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := p.authorize(ctx, info.FullMethod); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// StreamInterceptor is UnaryInterceptor for streaming RPCs. It reads the
// caller from the stream's context.
func (l Ladder) StreamInterceptor(levels AuthLevelMap, allowlist AllowlistCheck, prefixes ...string) grpc.StreamServerInterceptor {
	p := newPolicy(l, levels, allowlist, prefixes)
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := p.authorize(ss.Context(), info.FullMethod); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

// policy is the decision both interceptors share.
type policy struct {
	ladder    Ladder
	levels    AuthLevelMap
	allowlist AllowlistCheck
	prefixes  []string
}

func newPolicy(l Ladder, levels AuthLevelMap, allowlist AllowlistCheck, prefixes []string) policy {
	if allowlist == nil {
		allowlist = DenyAllAllowlist
	}
	// Copies, so a caller changing its map or slice later cannot race a call.
	return policy{ladder: l, levels: maps.Clone(levels), allowlist: allowlist, prefixes: prefix.Normalise(prefixes)}
}

// authorize returns nil when the caller in ctx may call fullMethod, and the
// gRPC status error to send back otherwise.
func (p policy) authorize(ctx context.Context, fullMethod string) error {
	// grpc.Server passes a malformed name only to an UnknownServiceHandler,
	// where the raw path reaches the stream interceptor. Such a name could slip
	// past the prefixes, so it is refused.
	if !wellFormed(fullMethod) {
		return status.Errorf(codes.Internal, "malformed method name %q: it must start with a single \"/\" followed by the service", fullMethod)
	}
	level, ok := p.levels[fullMethod]
	if !ok {
		if p.enforced(fullMethod) {
			return status.Errorf(codes.Internal, "missing auth_level annotation on %s (annotate the RPC, or import its generated package before loading the levels)", fullMethod)
		}
		return nil
	}
	if level == protoauthzv1.AuthLevel_AUTH_LEVEL_UNAUTHENTICATED {
		return nil
	}
	if !enforceable(level) {
		return status.Errorf(codes.PermissionDenied, "auth_level %v on %s is denied", level, fullMethod)
	}

	userID := UserIDFromContext(ctx)
	if userID == "" {
		return status.Error(codes.Unauthenticated, "authentication required")
	}

	switch level {
	case protoauthzv1.AuthLevel_AUTH_LEVEL_ADMIN:
		return p.ladder.require(RoleFromContext(ctx), p.ladder.admin)
	case protoauthzv1.AuthLevel_AUTH_LEVEL_SUPER_ADMIN:
		return p.ladder.require(RoleFromContext(ctx), p.ladder.superAdmin)
	case protoauthzv1.AuthLevel_AUTH_LEVEL_ALLOWLIST:
		allowed, err := p.allowlist(ctx, EmailFromContext(ctx), userID)
		if err != nil {
			return status.Error(codes.Internal, "allow-list check failed")
		}
		if !allowed {
			return status.Error(codes.PermissionDenied, "caller is not on the allow-list")
		}
	}
	return nil // AUTH_LEVEL_AUTHENTICATED, or a check above passed
}

// wellFormed reports whether fullMethod starts with exactly one "/" and has
// something after it.
func wellFormed(fullMethod string) bool {
	return len(fullMethod) > 1 && fullMethod[0] == '/' && fullMethod[1] != '/'
}

// enforced reports whether an unannotated method is denied: always with no
// prefixes, otherwise only under one of them.
func (p policy) enforced(fullMethod string) bool {
	return prefix.Under(fullMethod, p.prefixes)
}

// enforceable reports whether level is one the interceptors can check.
// AUTH_LEVEL_UNSPECIFIED and unknown values are not.
func enforceable(level protoauthzv1.AuthLevel) bool {
	switch level {
	case protoauthzv1.AuthLevel_AUTH_LEVEL_UNAUTHENTICATED,
		protoauthzv1.AuthLevel_AUTH_LEVEL_AUTHENTICATED,
		protoauthzv1.AuthLevel_AUTH_LEVEL_ADMIN,
		protoauthzv1.AuthLevel_AUTH_LEVEL_SUPER_ADMIN,
		protoauthzv1.AuthLevel_AUTH_LEVEL_ALLOWLIST:
		return true
	}
	return false
}

func forEachMethod(fd protoreflect.FileDescriptor, fn func(fullMethod string, md protoreflect.MethodDescriptor)) {
	for i := range fd.Services().Len() {
		sd := fd.Services().Get(i)
		for j := range sd.Methods().Len() {
			md := sd.Methods().Get(j)
			fn(fmt.Sprintf("/%s/%s", sd.FullName(), md.Name()), md)
		}
	}
}

func authLevelOf(md protoreflect.MethodDescriptor) (protoauthzv1.AuthLevel, bool) {
	opts, ok := md.Options().(*descriptorpb.MethodOptions)
	if !ok || !proto.HasExtension(opts, protoauthzv1.E_AuthLevel) {
		return 0, false
	}
	level, ok := proto.GetExtension(opts, protoauthzv1.E_AuthLevel).(protoauthzv1.AuthLevel)
	return level, ok
}
