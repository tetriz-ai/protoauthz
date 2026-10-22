Replaces the default roles with viewer, editor and owner, using `NewLadder` and the ladder's own interceptor. `AUTH_LEVEL_ADMIN` then admits editor and owner, and `AUTH_LEVEL_SUPER_ADMIN` admits owner. A role name from the default ladder means nothing on this one, and `NewLadder` rejects a ladder that does not list the roles it names.

```sh
go run ./examples/custom-role-ladder
```

```text
DeleteNote as viewer: PermissionDenied
DeleteNote as editor: OK
DeleteNote as owner: OK
DeleteNote as admin: PermissionDenied
PurgeNotes as viewer: PermissionDenied
PurgeNotes as editor: PermissionDenied
PurgeNotes as owner: OK
PurgeNotes as admin: PermissionDenied
protoauthz: role "owner" is not on the ladder
```
