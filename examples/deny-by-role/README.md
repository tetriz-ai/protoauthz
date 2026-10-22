Calls the auth-level interceptor directly for `DeleteNote`, an RPC whose proto sets `AUTH_LEVEL_ADMIN`, as an anonymous caller, a member and an admin. It is the README's quick start. The caller comes from `WithUserID` and `WithRole`, which your login step would call.

```sh
go run ./examples/deny-by-role
```

```text
Unauthenticated
PermissionDenied
OK
```
