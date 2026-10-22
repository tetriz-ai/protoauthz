// Command grpc-server-with-login runs the example notes service behind two
// interceptors: a small login step that turns a bearer token into a user and
// a role, then protoauthz. It calls the server over an in-memory connection,
// so no call leaves the machine.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/tetriz-ai/protoauthz"
	examplev1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/v1"
)

// The demo's token table. A real service verifies a session or a signed token.
var tokens = map[string]struct{ userID, role string }{
	"token-jane": {"u-1", protoauthz.RoleMember},
	"token-john": {"u-2", protoauthz.RoleAdmin},
}

// login records the caller for protoauthz. A known token sets the user and
// the role, an unknown one is rejected, and a call with no token goes on
// anonymous, for protoauthz to allow only on public RPCs.
func login(ctx context.Context) (context.Context, error) {
	values := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(values) == 0 {
		return ctx, nil
	}
	user, ok := tokens[strings.TrimPrefix(values[0], "Bearer ")]
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unknown token")
	}
	ctx = protoauthz.WithUserID(ctx, user.userID)
	return protoauthz.WithRole(ctx, user.role), nil
}

func unaryLogin(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	ctx, err := login(ctx)
	if err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

type streamWithContext struct {
	grpc.ServerStream
	ctx context.Context
}

func (s streamWithContext) Context() context.Context { return s.ctx }

func streamLogin(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	ctx, err := login(ss.Context())
	if err != nil {
		return err
	}
	return handler(srv, streamWithContext{ss, ctx})
}

type notes struct {
	examplev1.UnimplementedNoteServiceServer
}

func (notes) GetVersion(context.Context, *examplev1.GetVersionRequest) (*examplev1.GetVersionResponse, error) {
	return &examplev1.GetVersionResponse{Version: "1.0.0"}, nil
}

func (notes) ListNotes(ctx context.Context, _ *examplev1.ListNotesRequest) (*examplev1.ListNotesResponse, error) {
	return &examplev1.ListNotesResponse{Notes: []string{"notes for " + protoauthz.UserIDFromContext(ctx)}}, nil
}

func (notes) DeleteNote(context.Context, *examplev1.DeleteNoteRequest) (*examplev1.DeleteNoteResponse, error) {
	return &examplev1.DeleteNoteResponse{}, nil
}

func (notes) WatchNotes(_ *examplev1.WatchNotesRequest, stream grpc.ServerStreamingServer[examplev1.WatchNotesResponse]) error {
	for _, n := range []string{"renew the domain", "send the invoice"} {
		if err := stream.Send(&examplev1.WatchNotesResponse{Note: n}); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	levels := protoauthz.LoadAuthLevelsFromRegistry()
	fmt.Println("public:", protoauthz.UnauthenticatedMethods(levels))

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(unaryLogin, protoauthz.NewAuthLevelInterceptor(levels, nil)),
		grpc.ChainStreamInterceptor(streamLogin, protoauthz.NewAuthLevelStreamInterceptor(levels, nil)),
	)
	examplev1.RegisterNoteServiceServer(server, notes{})
	listener := bufconn.Listen(1 << 20)
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()

	conn, err := grpc.NewClient("passthrough:///notes",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	client := examplev1.NewNoteServiceClient(conn)

	version, err := client.GetVersion(as(""), &examplev1.GetVersionRequest{})
	fmt.Println("GetVersion, no token:", version.GetVersion(), status.Code(err))
	for _, token := range []string{"", "token-forged", "token-jane"} {
		list, err := client.ListNotes(as(token), &examplev1.ListNotesRequest{})
		fmt.Printf("ListNotes, token %q: %v %v\n", token, list.GetNotes(), status.Code(err))
	}
	for _, token := range []string{"token-jane", "token-john"} {
		_, err := client.DeleteNote(as(token), &examplev1.DeleteNoteRequest{Note: "n-1"})
		fmt.Printf("DeleteNote, token %q: %v\n", token, status.Code(err))
	}
	for _, token := range []string{"", "token-jane"} {
		got, err := watch(as(token), client)
		fmt.Printf("WatchNotes, token %q: %v %v\n", token, got, status.Code(err))
	}
}

func as(token string) context.Context {
	if token == "" {
		return context.Background()
	}
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
}

func watch(ctx context.Context, client examplev1.NoteServiceClient) ([]string, error) {
	stream, err := client.WatchNotes(ctx, &examplev1.WatchNotesRequest{})
	if err != nil {
		return nil, err
	}
	var got []string
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return got, nil
		}
		if err != nil {
			return got, err
		}
		got = append(got, msg.GetNote())
	}
}
