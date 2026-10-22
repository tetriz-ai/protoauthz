Passes a prefix, so that only unannotated methods under it are denied, as a server that also registers the gRPC health check needs. `ArchiveNote`, a method the notes service does not define, is under the prefix and denied with `Internal`, while `/grpc.health.v1.Health/Check` is outside it and passes. With no prefix, both are denied.

```sh
go run ./examples/scope-by-prefix
```

```text
prefix /protoauthz.example.v1.: /protoauthz.example.v1.NoteService/ListNotes: OK
prefix /protoauthz.example.v1.: /protoauthz.example.v1.NoteService/ArchiveNote: Internal
prefix /protoauthz.example.v1.: /grpc.health.v1.Health/Check: OK
no prefix: /protoauthz.example.v1.NoteService/ListNotes: OK
no prefix: /protoauthz.example.v1.NoteService/ArchiveNote: Internal
no prefix: /grpc.health.v1.Health/Check: Internal
```
