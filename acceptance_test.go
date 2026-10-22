// Copyright 2026 The protoauthz Authors
// SPDX-License-Identifier: MIT

package protoauthz_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/tetriz-ai/protoauthz"
	draftv1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/draft/v1"
	examplev1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/v1"
	protoauthzv1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/v1"
	"github.com/tetriz-ai/protoauthz/protoauthztest"
)

// TestAcceptance runs a service with two RPCs, one annotated and one
// not. The build-time check names the unannotated one. Once both carry an
// auth_level, an unauthenticated call to the protected RPC is denied and an
// authenticated one succeeds.

type user struct{ id, role string }

var tokens = map[string]user{
	"member-token": {"u-1", protoauthz.RoleMember},
	"admin-token":  {"u-2", protoauthz.RoleAdmin},
}

// login stands in for the application's authentication: a known bearer token
// sets the caller, an unknown one is rejected, and no token leaves the call
// anonymous for the auth-level interceptor to judge.
func login(ctx context.Context) (context.Context, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return ctx, nil
	}
	u, ok := tokens[strings.TrimPrefix(vals[0], "Bearer ")]
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unknown token")
	}
	return protoauthz.WithRole(protoauthz.WithUserID(ctx, u.id), u.role), nil
}

func unaryLogin(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	ctx, err := login(ctx)
	if err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

type loggedInStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s loggedInStream) Context() context.Context { return s.ctx }

func streamLogin(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	ctx, err := login(ss.Context())
	if err != nil {
		return err
	}
	return handler(srv, loggedInStream{ss, ctx})
}

type notes struct {
	examplev1.UnimplementedNoteServiceServer
}

func (notes) GetVersion(context.Context, *examplev1.GetVersionRequest) (*examplev1.GetVersionResponse, error) {
	return &examplev1.GetVersionResponse{Version: "1.0.0"}, nil
}

func (notes) ListNotes(context.Context, *examplev1.ListNotesRequest) (*examplev1.ListNotesResponse, error) {
	return &examplev1.ListNotesResponse{Notes: []string{"renew the domain"}}, nil
}

func (notes) DeleteNote(context.Context, *examplev1.DeleteNoteRequest) (*examplev1.DeleteNoteResponse, error) {
	return &examplev1.DeleteNoteResponse{}, nil
}

func (notes) WatchNotes(_ *examplev1.WatchNotesRequest, stream grpc.ServerStreamingServer[examplev1.WatchNotesResponse]) error {
	return stream.Send(&examplev1.WatchNotesResponse{Note: "renew the domain"})
}

type draftNotes struct {
	draftv1.UnimplementedNoteServiceServer
}

func (draftNotes) ListNotes(context.Context, *draftv1.ListNotesRequest) (*draftv1.ListNotesResponse, error) {
	return &draftv1.ListNotesResponse{}, nil
}

func (draftNotes) DeleteNote(context.Context, *draftv1.DeleteNoteRequest) (*draftv1.DeleteNoteResponse, error) {
	return &draftv1.DeleteNoteResponse{}, nil
}

// serve starts a server with login, then the auth-level interceptors, and
// returns a client connection to it over an in-memory listener.
func serve(t *testing.T, register func(*grpc.Server)) *grpc.ClientConn {
	t.Helper()
	levels := protoauthz.LoadAuthLevelsFromRegistry()
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(unaryLogin, protoauthz.NewAuthLevelInterceptor(levels, nil)),
		grpc.ChainStreamInterceptor(streamLogin, protoauthz.NewAuthLevelStreamInterceptor(levels, nil)),
	)
	register(srv)
	lis := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func as(token string) context.Context {
	if token == "" {
		return context.Background()
	}
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
}

// errorRecorder is a testing.TB that keeps the errors instead of failing.
type errorRecorder struct {
	testing.TB
	errors []string
}

func (r *errorRecorder) Helper() {}

func (r *errorRecorder) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func TestAcceptance(t *testing.T) {
	t.Run("before: the build-time check fails naming the unannotated RPC", func(t *testing.T) {
		r := &errorRecorder{TB: t}
		protoauthztest.RequireAuthLevels(r, draftv1.File_protoauthz_example_draft_v1_notes_proto)
		want := []string{"/protoauthz.example.draft.v1.NoteService/DeleteNote has no usable (protoauthz.v1.auth_level) option"}
		if !slices.Equal(r.errors, want) {
			t.Fatalf("RequireAuthLevels reported %q, want %q", r.errors, want)
		}
	})

	t.Run("before: the unannotated RPC is denied at run time too", func(t *testing.T) {
		client := draftv1.NewNoteServiceClient(serve(t, func(s *grpc.Server) { draftv1.RegisterNoteServiceServer(s, draftNotes{}) }))
		_, err := client.DeleteNote(as("admin-token"), &draftv1.DeleteNoteRequest{Note: "n-1"})
		if status.Code(err) != codes.Internal {
			t.Fatalf("DeleteNote: %v, want Internal", err)
		}
		if _, err := client.ListNotes(as("member-token"), &draftv1.ListNotesRequest{}); err != nil {
			t.Fatalf("annotated ListNotes: %v", err)
		}
	})

	t.Run("after: the build-time check passes", func(t *testing.T) {
		protoauthztest.RequireAuthLevels(t, examplev1.File_protoauthz_example_v1_notes_proto)
	})

	client := examplev1.NewNoteServiceClient(serve(t, func(s *grpc.Server) { examplev1.RegisterNoteServiceServer(s, notes{}) }))
	unary := []struct {
		name  string
		token string
		call  func(context.Context) error
		want  codes.Code
	}{
		{"unauthenticated ListNotes denied", "", listNotes(client), codes.Unauthenticated},
		{"authenticated ListNotes succeeds", "member-token", listNotes(client), codes.OK},
		{"unknown token rejected by login", "forged-token", listNotes(client), codes.Unauthenticated},
		{"unauthenticated DeleteNote denied", "", deleteNote(client), codes.Unauthenticated},
		{"member DeleteNote denied", "member-token", deleteNote(client), codes.PermissionDenied},
		{"admin DeleteNote succeeds", "admin-token", deleteNote(client), codes.OK},
		{"GetVersion is public", "", getVersion(client), codes.OK},
		{"unauthenticated WatchNotes denied", "", watchNotes(client), codes.Unauthenticated},
		{"authenticated WatchNotes succeeds", "member-token", watchNotes(client), codes.OK},
	}
	for _, tt := range unary {
		t.Run("after: "+tt.name, func(t *testing.T) {
			if got := status.Code(tt.call(as(tt.token))); got != tt.want {
				t.Fatalf("code %v, want %v", got, tt.want)
			}
		})
	}
}

func listNotes(c examplev1.NoteServiceClient) func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := c.ListNotes(ctx, &examplev1.ListNotesRequest{})
		return err
	}
}

func deleteNote(c examplev1.NoteServiceClient) func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := c.DeleteNote(ctx, &examplev1.DeleteNoteRequest{Note: "n-1"})
		return err
	}
}

func getVersion(c examplev1.NoteServiceClient) func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := c.GetVersion(ctx, &examplev1.GetVersionRequest{})
		return err
	}
}

func watchNotes(c examplev1.NoteServiceClient) func(context.Context) error {
	return func(ctx context.Context) error {
		stream, err := c.WatchNotes(ctx, &examplev1.WatchNotesRequest{})
		if err != nil {
			return err
		}
		for {
			if _, err := stream.Recv(); err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				return err
			}
		}
	}
}

// threeNotes streams three notes.
type threeNotes struct {
	examplev1.UnimplementedNoteServiceServer
}

func (threeNotes) WatchNotes(_ *examplev1.WatchNotesRequest, stream grpc.ServerStreamingServer[examplev1.WatchNotesResponse]) error {
	for _, n := range []string{"one", "two", "three"} {
		if err := stream.Send(&examplev1.WatchNotesResponse{Note: n}); err != nil {
			return err
		}
	}
	return nil
}

// A stream is checked once, when it opens: an allow-list callback that says
// yes once and no afterwards still lets every message of an open stream
// through, and it is called once per stream, not once per message.
func TestStreamCheckedOnceWhenItOpens(t *testing.T) {
	const watch = "/protoauthz.example.v1.NoteService/WatchNotes"
	levels := protoauthz.AuthLevelMap{watch: protoauthzv1.AuthLevel_AUTH_LEVEL_ALLOWLIST}
	calls := 0
	yesOnce := func(context.Context, string, string) (bool, error) {
		calls++
		return calls == 1, nil
	}
	srv := grpc.NewServer(grpc.ChainStreamInterceptor(streamLogin, protoauthz.NewAuthLevelStreamInterceptor(levels, yesOnce)))
	examplev1.RegisterNoteServiceServer(srv, threeNotes{})
	lis := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := examplev1.NewNoteServiceClient(conn)

	stream, err := client.WatchNotes(as("member-token"), &examplev1.WatchNotesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("after %v: %v", got, err)
		}
		got = append(got, msg.GetNote())
	}
	if !slices.Equal(got, []string{"one", "two", "three"}) || calls != 1 {
		t.Fatalf("got %v with %d callback calls, want all three notes and 1 call", got, calls)
	}
	if err := watchNotes(client)(as("member-token")); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("second stream: %v, want PermissionDenied from the callback's no", err)
	}
}

// A grpc.Server passes a malformed name such as "//pkg.S/M" to an
// UnknownServiceHandler with the raw path as the full method. The stream
// interceptor refuses it, so the handler never runs, even under a prefix the
// raw name would not match.
func TestUnknownServiceHandlerGetsNoMalformedName(t *testing.T) {
	var handled []string
	unknown := func(_ any, stream grpc.ServerStream) error {
		method, _ := grpc.MethodFromServerStream(stream)
		handled = append(handled, method)
		return nil
	}
	srv := grpc.NewServer(
		grpc.UnknownServiceHandler(unknown),
		grpc.StreamInterceptor(protoauthz.NewAuthLevelStreamInterceptor(protoauthz.AuthLevelMap{}, nil, "/pkg.")),
	)
	lis := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	tests := []struct {
		method string
		want   codes.Code
	}{
		{"//pkg.S/M", codes.Internal}, // malformed: refused before the prefix check
		{"/pkg.S/M", codes.Internal},  // well-formed, under the prefix, unannotated
		{"/other.S/M", codes.OK},      // well-formed, outside the prefix: reaches the handler
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			stream, err := conn.NewStream(context.Background(), &grpc.StreamDesc{ServerStreams: true}, tt.method)
			if err == nil {
				err = stream.RecvMsg(&examplev1.GetVersionResponse{})
			}
			if errors.Is(err, io.EOF) {
				err = nil
			}
			if got := status.Code(err); got != tt.want {
				t.Fatalf("code %v (%v), want %v", got, err, tt.want)
			}
		})
	}
	if !slices.Equal(handled, []string{"/other.S/M"}) {
		t.Fatalf("handler ran for %v, want only /other.S/M", handled)
	}
}
