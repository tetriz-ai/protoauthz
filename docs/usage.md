# Usage

Walkthroughs for the tasks the README's quick start leads into. Every Go block here is a
complete program or test file. The exact rules are in [reference.md](reference.md), and every
example is listed in [examples/README.md](../examples/README.md).

## Annotate your RPCs

Annotating needs a code generation step. These steps use buf, with protoc-gen-go and
protoc-gen-go-grpc on your `PATH`. [CONTRIBUTING.md](../CONTRIBUTING.md#run-the-checks) pins
versions of all three that CI tests.

1. From your module's root, after `go get`, copy `proto/protoauthz/v1/auth_level.proto` out of
   the module into your proto root, here `proto/`:

   ```sh
   mkdir -p proto/protoauthz/v1
   cp "$(go list -m -f '{{.Dir}}' github.com/tetriz-ai/protoauthz)/proto/protoauthz/v1/auth_level.proto" proto/protoauthz/v1/
   ```

   Keep its `go_package` unchanged, so your generated code imports
   `github.com/tetriz-ai/protoauthz/gen/protoauthz/v1`. A path in your own module makes the
   binary fail at startup with
   `panic: proto: file "protoauthz/v1/auth_level.proto" is already registered`.

2. Set the option on each method, here in `proto/example/v1/notes.proto`:

   ```proto
   syntax = "proto3";

   package example.v1;

   import "protoauthz/v1/auth_level.proto";

   option go_package = "example.com/notes/gen/example/v1;examplev1";

   service NoteService {
     rpc DeleteNote(DeleteNoteRequest) returns (DeleteNoteResponse) {
       option (protoauthz.v1.auth_level) = AUTH_LEVEL_ADMIN;
     }
   }

   message DeleteNoteRequest {}

   message DeleteNoteResponse {}
   ```

3. Leave the copied file out of your own code generation. With buf, `exclude_paths` on the
   input in `buf.gen.yaml` does that:

   ```yaml
   version: v2
   inputs:
     - directory: proto
       exclude_paths: [proto/protoauthz]
   plugins:
     - local: protoc-gen-go
       out: gen
       opt: paths=source_relative
     - local: protoc-gen-go-grpc
       out: gen
       opt: paths=source_relative
   ```

4. Run `buf generate`. It writes `gen/example/v1/notes.pb.go` and `notes_grpc.pb.go`.

## Enforce the levels on a server

The README's [server program](../README.md#enforce-the-levels-on-a-server) shows the wiring.
[grpc-server-with-login](../examples/grpc-server-with-login/) is the full program, with
streaming RPCs and a client over an in-memory connection.

1. Import your generated packages, since `LoadAuthLevelsFromRegistry` reads only the code linked
   into the binary.
2. For streaming RPCs, put `NewAuthLevelStreamInterceptor` behind a stream login step, as the
   unary chain does.
3. Print `UnauthenticatedMethods(levels)` at startup or in a test, and review the RPCs anyone
   may call.

## Check every linked RPC in a test

The README's test lists proto files. To skip the file list, check every linked RPC under the
prefixes your interceptors take. The test sees only what its binary links, so put it in the
package that registers your services on the `grpc.Server`:

```go
package main

import (
	"testing"

	_ "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/v1" // registers the example service
	"github.com/tetriz-ai/protoauthz/protoauthztest"
)

// TestEveryLinkedRPCHasAnAuthLevel checks every RPC under the prefix that
// the test binary links, so a new RPC needs no list entry.
func TestEveryLinkedRPCHasAnAuthLevel(t *testing.T) {
	protoauthztest.RequireAuthLevelsFromRegistry(t, "/protoauthz.example.v1.")
}
```

A prefix with no linked RPC under it fails the test. That happens when the generated package is
not imported or the prefix is mistyped.

To see a missing level reported, run the example from your quick start folder. It calls
`MissingAuthLevels` on two proto files and names the RPC the draft leaves without a level:

```sh
go run github.com/tetriz-ai/protoauthz/examples/find-unannotated-rpcs
```

```text
protoauthz/example/draft/v1/notes.proto: 1 missing [/protoauthz.example.draft.v1.NoteService/DeleteNote]
protoauthz/example/v1/notes.proto: 0 missing []
```

## Use your own roles

`NewLadder` takes your roles lowest first, then the lowest role that passes each admin level. Use
the ladder's own interceptors, since `NewAuthLevelInterceptor` always uses the default roles.
[custom-role-ladder](../examples/custom-role-ladder/) also checks `AUTH_LEVEL_SUPER_ADMIN`:

```go
// Command roles checks DeleteNote, at AUTH_LEVEL_ADMIN, on a ladder of
// viewer, editor and owner.
package main

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/tetriz-ai/protoauthz"
	_ "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/v1" // registers the example service
)

func main() {
	ladder, err := protoauthz.NewLadder([]string{"viewer", "editor", "owner"}, "editor", "owner")
	if err != nil {
		log.Fatal(err)
	}
	authz := ladder.UnaryInterceptor(protoauthz.LoadAuthLevelsFromRegistry(), nil)
	info := &grpc.UnaryServerInfo{FullMethod: "/protoauthz.example.v1.NoteService/DeleteNote"}
	for _, role := range []string{"viewer", "editor"} {
		ctx := protoauthz.WithRole(protoauthz.WithUserID(context.Background(), "u-1"), role)
		_, err := authz(ctx, nil, info, func(context.Context, any) (any, error) { return nil, nil })
		fmt.Println(role, status.Code(err))
	}
}
```

```text
viewer PermissionDenied
editor OK
```

Record roles in the ladder's own spelling. `Admin` is not `admin`.

## Admit a named set of people

Set `AUTH_LEVEL_ALLOWLIST` on an RPC that only certain people may call, whatever their role, and
pass a callback. [staff-allow-list](../examples/staff-allow-list/) also shows anonymous callers
and a nil callback:

```go
// Command allow guards ExportNotes, at AUTH_LEVEL_ALLOWLIST, with a callback
// that admits support staff by email.
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
	for _, email := range []string{"jane@example.com", "john@example.com"} {
		ctx := protoauthz.WithEmail(protoauthz.WithUserID(context.Background(), "u-1"), email)
		_, err := authz(ctx, nil, info, func(context.Context, any) (any, error) { return nil, nil })
		fmt.Println(email, status.Code(err))
	}
}
```

```text
jane@example.com OK
john@example.com PermissionDenied
```

- **`email` is "" when your login step never called `WithEmail`.** A callback must not treat ""
  as a match.
- **A nil callback denies every call.** [reference.md](reference.md#the-caller-in-the-context) has
  the callback's other rules.

## Deny unannotated RPCs under a prefix

A server that also registers services you do not annotate, such as gRPC's health check, passes
one prefix per package it annotates. Unannotated methods under a prefix are then denied, and the
rest pass. [scope-by-prefix](../examples/scope-by-prefix/) compares a prefix with none:

```go
// Command prefix denies unannotated methods under /protoauthz.example.v1.
// only. ArchiveNote is a method the notes service does not define, so it has
// no level. The health check outside the prefix still passes.
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
	authz := protoauthz.NewAuthLevelInterceptor(protoauthz.LoadAuthLevelsFromRegistry(), nil, "/protoauthz.example.v1.")
	ctx := protoauthz.WithUserID(context.Background(), "u-1")
	for _, method := range []string{"/protoauthz.example.v1.NoteService/ArchiveNote", "/grpc.health.v1.Health/Check"} {
		_, err := authz(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, func(context.Context, any) (any, error) { return nil, nil })
		fmt.Println(method, status.Code(err))
	}
}
```

```text
/protoauthz.example.v1.NoteService/ArchiveNote Internal
/grpc.health.v1.Health/Check OK
```

- **Annotate a method outside every prefix unless anyone may call it.** An unannotated one there
  passes unchecked.
- **A mistyped prefix matches nothing, so every unannotated method passes.** `/pkg/` for
  `/pkg.` constructs without error.
- **Run `RequireAuthLevelsFromRegistry` on the same prefixes.** It fails on a prefix with no RPC
  under it.

## Change protoauthz itself

[CONTRIBUTING.md](../CONTRIBUTING.md) installs the pinned linter, buf and code generators that CI
uses, and lists the checks it runs.
