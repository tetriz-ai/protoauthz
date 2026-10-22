# CLAUDE.md

## What this is

protoauthz enforces a per-RPC access level that each gRPC method declares with the `protoauthz.v1.auth_level` proto option: a unary and a stream interceptor check the caller in the context, and `protoauthztest.RequireAuthLevels` fails a test for every RPC without a usable level.
It does no authentication and stores no roles. The application's own login step records the caller with `WithUserID`, `WithRole` and `WithEmail`.

## Layout

- `auth_level.go`: `AuthLevelMap`, `LoadAuthLevelsFromRegistry`, `MissingAuthLevels`, `MissingAuthLevelsFromRegistry`, `UnauthenticatedMethods`, the interceptors, and `policy.authorize`, the one decision both interceptors share.
- `ladder.go`: `Ladder`, `NewLadder`, `DefaultLadder`, the `Role*` constants. `context.go`: the caller's user ID, role and email.
- `protoauthztest/`: `RequireAuthLevels` and `RequireAuthLevelsFromRegistry`, for a service's own test suite.
- `internal/prefix`: `Normalise` and `Under`, the prefix normalising, validation and matching that the root package and `protoauthztest` share.
- `proto/protoauthz/v1/auth_level.proto`: the option. `proto/protoauthz/example/v1/notes.proto`: the service tests and examples use, with `GetVersion` UNAUTHENTICATED, `ListNotes` and `WatchNotes` (stream) AUTHENTICATED, `DeleteNote` ADMIN, `PurgeNotes` SUPER_ADMIN, `ExportNotes` ALLOWLIST. `proto/protoauthz/example/draft/v1/notes.proto`: the same service with `DeleteNote` unannotated. `gen/`: code `buf generate` writes from them.
- Tests: `auth_level_test.go` (every case through both interceptors), `levels_test.go` (ladder, allow-list, walker, context), `acceptance_test.go` (a server over bufconn with a login interceptor), `example_test.go`, `readme_test.go`.
- `examples/`: one folder per example, indexed in `examples/README.md`. Copy `examples/deny-by-role` for a new one. `readme_test.go` keeps the README quick start, that example and `ExampleNewAuthLevelInterceptor` one program.

## Commands

Go 1.26 is the floor. CI tests 1.26 and 1.27. A pull request runs five jobs: "Test (Go 1.26)", "Test (Go 1.27)", "Lint", "Generated code" and "Examples". "Vulnerability check" (govulncheck) runs on pushes to main and weekly. Their commands:

```sh
go build ./...
go vet ./...
go test -race -count=1 ./...
GOTOOLCHAIN=go1.26.0 golangci-lint run
scripts/run-examples.sh
```

Lint uses golangci-lint v2.11.4 built with Go 1.26, as the Lint job does. The install lines come from CONTRIBUTING.md:

```sh
GOTOOLCHAIN=go1.26.0 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.1
go install github.com/bufbuild/buf/cmd/buf@v1.67.0
```

Check `golangci-lint version` shows v2.11.4 built with go1.26. If another comes first on `PATH`, run `GOTOOLCHAIN=go1.26.0 "$(go env GOPATH)/bin/golangci-lint" run`.

The "Vulnerability check" job runs govulncheck on Go 1.27. Locally:

```sh
GOTOOLCHAIN=go1.27.3 go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
```

After editing a `.proto` file, run `buf lint`, then `buf generate`. The "Generated code" job runs both and fails when `gen/` changes.

## Contract to keep

- Every level but `AUTH_LEVEL_UNAUTHENTICATED` needs a non-empty user ID, else `Unauthenticated`. Pinned by `TestAuthLevelInterceptors` in `auth_level_test.go`.
- `AUTH_LEVEL_UNSPECIFIED` and unknown levels are denied with `PermissionDenied`. Pinned by `TestAuthLevelInterceptors` and `TestErrorMessages`.
- With no prefixes, an unannotated method is denied with `Internal`. Pinned by the "no prefixes" cases of `TestAuthLevelInterceptors`.
- The unary and stream interceptors decide the same way. Pinned by `TestAuthLevelInterceptors`, which runs every case through both.
- A nil allow-list callback denies all, and a callback error is `Internal` without its text. Pinned by `TestAllowlistLevel` in `levels_test.go`.
- An empty `Ladder{}` admits no one at the admin levels. Pinned by `TestLadderRanksRoles`.
- A server over bufconn denies the unannotated RPC and admits an authenticated call to the annotated one. Pinned by `TestAcceptance` in `acceptance_test.go`.

## Making a change

- Write the test first. A fix needs a case that fails before it and passes after. Tests are table-driven. A new rule is a row in `TestAuthLevelInterceptors`.
- A change a user can see gets a `CHANGELOG.md` line under Unreleased. Tests, docs, CI and examples need none.
- Do not silence a lint finding with an inline `//nolint`. Fix the code.
- The examples runner runs each example README's `sh` block from the repo root and compares standard output with its last fenced block. It rewrites a `go test` line's duration to `<time>`. Example output must not depend on map order or time.
- A new example gets a row in `examples/README.md`, in the form `| <you want> | [name](name/) |`. The runner fails when the index and the folders differ.
- Files under `examples/` carry no license header. Every other `.go` and `.proto` file does, and `gen/` gets it from the proto.
- Commits use Conventional Commits.

## Do not

- Do not edit `gen/` by hand. Regenerate it with `buf generate`.
- Do not raise `go 1.26` in `go.mod`. It is the floor `golang.org/x/net` sets.
- Do not move the lint job to Go 1.27. golangci-lint v2.11 cannot read Go 1.27 export data.
- Do not change the extension number 2800105 or renumber `AuthLevel`. Compiled descriptors in users' binaries carry them.
