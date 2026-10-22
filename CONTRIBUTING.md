# Contributing

Thank you for helping. The scope is small on purpose. Open an issue before starting a new entry
point or a new `auth_level` value, so we agree on it first.

## Run the checks

CI runs all of these. None needs a service. The suite covers every level through both
interceptors and an end-to-end server over an in-memory connection. `readme_test.go` also
builds and runs every Go block in `README.md` and `docs/`, and compares each program's output
with the block that follows it.

```sh
go build ./...
go vet ./...
go test -race -count=1 ./...
GOTOOLCHAIN=go1.26.0 golangci-lint run   # golangci-lint v2.11.4 on Go 1.26, as CI runs it; configuration in .golangci.yml
scripts/run-examples.sh
buf lint            # after editing a .proto file; then buf generate
GOTOOLCHAIN=go1.27.3 go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
```

Install the pinned linter, buf, and the code generators `buf generate` uses:

```sh
GOTOOLCHAIN=go1.26.0 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.1
go install github.com/bufbuild/buf/cmd/buf@v1.67.0
```

Go 1.26 is the floor. The linter is built with Go 1.26, the version the Lint CI job uses, and
runs on it too, since golangci-lint v2.11 cannot read Go 1.27 export data. Naming the toolchain
keeps `go install` and the run line from choosing one on their own.

## Every change needs a test

A pull request without a test is not merged. Tests are table-driven, and a new rule is a row in
`TestAuthLevelInterceptors`, which runs through both interceptors. A change a user can see, such
as a level, status code or message, also needs a `CHANGELOG.md` entry.

## Commit style

Use [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/): a short subject
such as `docs: clarify the allow-list level`, a blank line, then a body that says why.

## Licensing of contributions

No sign-off line and no separately signed agreement are needed. By opening a pull request you
agree that your contribution is released under the project's [LICENSE](LICENSE).
