Lists the RPCs with no usable `auth_level` in a draft notes service that leaves `DeleteNote` bare, and in the finished service. `annotations_test.go` is the one-line test to copy into your own suite: `protoauthztest.RequireAuthLevels` fails it, naming each such RPC, so a new RPC without an `auth_level` fails the build before it can be denied in production.

```sh
go run ./examples/find-unannotated-rpcs
go test -count=1 ./examples/find-unannotated-rpcs
```

```text
protoauthz/example/draft/v1/notes.proto: 1 missing [/protoauthz.example.draft.v1.NoteService/DeleteNote]
protoauthz/example/v1/notes.proto: 0 missing []
ok  	github.com/tetriz-ai/protoauthz/examples/find-unannotated-rpcs	<time>
```
