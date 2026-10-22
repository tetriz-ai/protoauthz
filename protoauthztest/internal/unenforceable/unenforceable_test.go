// Copyright 2026 The protoauthz Authors
// SPDX-License-Identifier: MIT

// Package unenforceable_test registers an RPC set to AUTH_LEVEL_UNSPECIFIED
// outside the prefixes it checks. The interceptors deny such an RPC whatever
// the prefixes, so RequireAuthLevelsFromRegistry fails on it too.
package unenforceable_test

import (
	"fmt"
	"slices"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	_ "google.golang.org/protobuf/types/known/emptypb" // registers google/protobuf/empty.proto

	_ "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/v1" // registers the example service
	protoauthzv1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/v1"
	"github.com/tetriz-ai/protoauthz/protoauthztest"
)

// recorder is a testing.TB that keeps the errors instead of failing.
type recorder struct {
	testing.TB
	errors []string
}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func TestRequireAuthLevelsFromRegistryFailsOnAnUnenforceableLevelOutsideThePrefixes(t *testing.T) {
	options := &descriptorpb.MethodOptions{}
	proto.SetExtension(options, protoauthzv1.E_AuthLevel, protoauthzv1.AuthLevel_AUTH_LEVEL_UNSPECIFIED)
	fdp := &descriptorpb.FileDescriptorProto{
		Name:       proto.String("zero/v1/zero.proto"),
		Package:    proto.String("zero.v1"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"google/protobuf/empty.proto"},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: proto.String("S"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name: proto.String("Zero"), InputType: proto.String(".google.protobuf.Empty"),
				OutputType: proto.String(".google.protobuf.Empty"), Options: options,
			}},
		}},
	}
	fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatal(err)
	}
	if err := protoregistry.GlobalFiles.RegisterFile(fd); err != nil {
		t.Fatal(err)
	}

	r := &recorder{TB: t}
	protoauthztest.RequireAuthLevelsFromRegistry(r, "/protoauthz.example.v1.")
	want := []string{"/zero.v1.S/Zero has no usable (protoauthz.v1.auth_level) option"}
	if !slices.Equal(r.errors, want) {
		t.Fatalf("errors %q, want %q", r.errors, want)
	}
}
