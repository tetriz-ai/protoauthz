# Examples

| You want | Example |
|---|---|
| To see one RPC's auth_level allow and deny callers | [deny-by-role](deny-by-role/) |
| A test that fails when an RPC has no auth_level | [find-unannotated-rpcs](find-unannotated-rpcs/) |
| Your own role names in place of member, admin and super_admin | [custom-role-ladder](custom-role-ladder/) |
| To deny unannotated methods under one prefix only | [scope-by-prefix](scope-by-prefix/) |
| An RPC only a named set of people may call | [staff-allow-list](staff-allow-list/) |
| A gRPC server with a login step in front of protoauthz | [grpc-server-with-login](grpc-server-with-login/) |

Run them all with `scripts/run-examples.sh` from the repo root. They use the notes service in `proto/protoauthz/example/`, and no call leaves the machine. The runner rewrites the duration at the end of a `go test` line to `<time>`.

The examples are under the repository's [LICENSE](../LICENSE), so a copy carries those terms.
