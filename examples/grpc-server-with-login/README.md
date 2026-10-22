Runs the notes service behind a small login interceptor and protoauthz, for unary and streaming RPCs. The login step turns a bearer token into a user ID and a role with `WithUserID` and `WithRole`, rejects an unknown token, and lets a call with no token through anonymous. protoauthz then allows the anonymous call only on the public `GetVersion`.

```sh
go run ./examples/grpc-server-with-login
```

```text
public: [/protoauthz.example.v1.NoteService/GetVersion]
GetVersion, no token: 1.0.0 OK
ListNotes, token "": [] Unauthenticated
ListNotes, token "token-forged": [] Unauthenticated
ListNotes, token "token-jane": [notes for u-1] OK
DeleteNote, token "token-jane": PermissionDenied
DeleteNote, token "token-john": OK
WatchNotes, token "": [] Unauthenticated
WatchNotes, token "token-jane": [renew the domain send the invoice] OK
```
