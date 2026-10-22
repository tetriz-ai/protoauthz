// Copyright 2026 The protoauthz Authors
// SPDX-License-Identifier: MIT

package protoauthz

import "context"

// contextKey keys the caller's identity, as the application's authentication
// step records it. The interceptors read these values and nothing else.
type contextKey string

const (
	ctxUserID contextKey = "user_id"
	ctxEmail  contextKey = "email"
	ctxRole   contextKey = "role"
)

// WithUserID returns a copy of ctx carrying the authenticated user's ID. An
// empty ID means the caller is not authenticated.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ctxUserID, userID)
}

// UserIDFromContext returns the authenticated user's ID, or "" when none is set.
func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxUserID).(string)
	return v
}

// WithRole returns a copy of ctx carrying the authenticated user's role.
func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, ctxRole, role)
}

// RoleFromContext returns the authenticated user's role, or "" when none is set.
func RoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxRole).(string)
	return v
}

// WithEmail returns a copy of ctx carrying the authenticated user's email.
func WithEmail(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, ctxEmail, email)
}

// EmailFromContext returns the authenticated user's email, or "" when none is set.
func EmailFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxEmail).(string)
	return v
}
