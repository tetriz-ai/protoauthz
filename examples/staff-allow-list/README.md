Guards `ExportNotes`, an RPC whose proto sets `AUTH_LEVEL_ALLOWLIST`, with a callback that admits support staff by email. The role does not matter at this level, so a super_admin who is not on the list is denied. The callback only sees authenticated callers, and a nil callback denies every call.

```sh
go run ./examples/staff-allow-list
```

```text
anonymous: Unauthenticated
john@example.com, super_admin: PermissionDenied
jane@example.com, member: OK
jane@example.com, no callback: PermissionDenied
```
