// Copyright 2026 The protoauthz Authors
// SPDX-License-Identifier: MIT

package protoauthz

import (
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The roles of DefaultLadder, lowest first.
const (
	RoleMember     = "member"
	RoleAdmin      = "admin"
	RoleSuperAdmin = "super_admin"
)

// Ladder ranks role names, so that AUTH_LEVEL_ADMIN and AUTH_LEVEL_SUPER_ADMIN
// mean "this role or any role above it". Build one with NewLadder or
// DefaultLadder. An empty Ladder{} ranks no role, so it denies every call at
// those two levels.
type Ladder struct {
	rank       map[string]int
	admin      string
	superAdmin string
}

// NewLadder returns a ladder of roles, lowest first. admin is the lowest role
// that passes AUTH_LEVEL_ADMIN and superAdmin the lowest that passes
// AUTH_LEVEL_SUPER_ADMIN. Both must be in roles, superAdmin no lower than
// admin, and every role a distinct, non-empty name.
func NewLadder(roles []string, admin, superAdmin string) (Ladder, error) {
	if len(roles) == 0 {
		return Ladder{}, errors.New("protoauthz: ladder has no roles")
	}
	rank := make(map[string]int, len(roles))
	for i, role := range roles {
		if role == "" {
			return Ladder{}, errors.New("protoauthz: ladder has an empty role name")
		}
		if _, dup := rank[role]; dup {
			return Ladder{}, fmt.Errorf("protoauthz: ladder lists role %q twice", role)
		}
		rank[role] = i + 1
	}
	for _, r := range []string{admin, superAdmin} {
		if _, ok := rank[r]; !ok {
			return Ladder{}, fmt.Errorf("protoauthz: role %q is not on the ladder", r)
		}
	}
	if rank[superAdmin] < rank[admin] {
		return Ladder{}, fmt.Errorf("protoauthz: super admin role %q ranks below admin role %q", superAdmin, admin)
	}
	return Ladder{rank: rank, admin: admin, superAdmin: superAdmin}, nil
}

// DefaultLadder is member, then admin, then super_admin. AUTH_LEVEL_ADMIN
// admits admin and super_admin. AUTH_LEVEL_SUPER_ADMIN admits super_admin.
func DefaultLadder() Ladder {
	l, err := NewLadder([]string{RoleMember, RoleAdmin, RoleSuperAdmin}, RoleAdmin, RoleSuperAdmin)
	if err != nil {
		panic(err) // unreachable: the arguments are constants that satisfy NewLadder
	}
	return l
}

// require returns nil when role ranks at or above minRole, and a
// PermissionDenied status otherwise. A role not on the ladder ranks below
// every role. A minRole not on the ladder, as in an empty Ladder{}, admits no one.
func (l Ladder) require(role, minRole string) error {
	need, ok := l.rank[minRole]
	if !ok {
		return status.Error(codes.PermissionDenied, "no role on the ladder admits this call")
	}
	if l.rank[role] < need {
		return status.Errorf(codes.PermissionDenied, "%s role required", minRole)
	}
	return nil
}
