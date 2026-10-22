# Reference

The exact rules for each part of protoauthz. Signatures and doc comments are on
[pkg.go.dev](https://pkg.go.dev/github.com/tetriz-ai/protoauthz). Task-shaped walkthroughs are in
[usage.md](usage.md), and the known gaps are in [limitations.md](limitations.md).

## The `auth_level` option

`protoauthz.v1.auth_level` is a method option, defined in `proto/protoauthz/v1/auth_level.proto`.
Its extension number is 2800105. No other option in your binary may use it.

| Level | Denied with | Who passes |
|---|---|---|
| `AUTH_LEVEL_UNSPECIFIED` (0) | `PermissionDenied` | nobody |
| `AUTH_LEVEL_UNAUTHENTICATED` | never denied | everyone, with or without a user ID |
| `AUTH_LEVEL_AUTHENTICATED` | `Unauthenticated` | any caller with a user ID |
| `AUTH_LEVEL_ADMIN` | `Unauthenticated` with no user ID, else `PermissionDenied` | a caller whose role ranks at or above the admin role, `admin` unless a [ladder](#newladder) says otherwise |
| `AUTH_LEVEL_SUPER_ADMIN` | `Unauthenticated` with no user ID, else `PermissionDenied` | a caller whose role ranks at or above the super admin role, `super_admin` unless a [ladder](#newladder) says otherwise |
| `AUTH_LEVEL_ALLOWLIST` | `Unauthenticated` with no user ID, else `PermissionDenied` | a caller with a user ID that the callback accepts |

A value this package does not know, such as one a newer proto adds, is denied like
`AUTH_LEVEL_UNSPECIFIED`.

## Loading the levels

- **`LoadAuthLevelsFromRegistry()`** reads every level from the generated code linked into the
  binary, so import your generated packages first.
- **`UnauthenticatedMethods(levels)`** lists the RPCs anyone may call, sorted, for you to review.

## The interceptors

`NewAuthLevelInterceptor(levels, allowlist, prefixes...)` returns the unary interceptor.
`NewAuthLevelStreamInterceptor` takes the same arguments and reads the caller from the stream's
context. Your `allowlist` callback decides who passes `AUTH_LEVEL_ALLOWLIST`.

Chain your login step first, then protoauthz. Put the stream interceptor behind a stream login
step in the same way.

### Unannotated methods

An unannotated method is denied with `Internal`. With prefixes, only the unannotated methods
under a prefix are denied, and the rest pass unchecked. The levels and prefixes are copied when
the interceptor is built, so a later change to your map does not reach it.

### Prefixes

A prefix is a plain string prefix of the full method name, `/package.Service/Method`. It gets
exactly one leading `/`. Where it ends decides what it covers:

| Prefix | Covers |
|---|---|
| `/x.v1.` | every service in package `x.v1` |
| `/x.v1.NoteService/` | that service only |
| `/x.v1` | package `x.v1`, and also longer packages such as `x.v1beta` |
| `/x.v1/` | service `v1` in package `x`, not package `x.v1` |
| `""` | every method |

A prefix may hold only ASCII letters, digits, `_`, `.` and `/`. After the leading `/` is added,
it may hold no `//` and at most two `/`. The constructor panics on any other prefix, such as
`/x.v1.S/M/`, since it can match no generated method name.

### Method names and caller fields

A method name must start with a single `/` followed by the service. Any other, such as
`x.v1.S/M`, `//x.v1.S/M`, `/` or "", is denied with `Internal`. `grpc.Server` passes such a name
only to an `UnknownServiceHandler`, which is where this check matters.

- **The user ID** decides whether a caller is authenticated.
- **The role** is read only for the two admin levels, and the email only for the allow-list.

### Status codes and messages

| Condition | Code | Message |
|---|---|---|
| Method name not starting with a single `/` and the service | `Internal` | `malformed method name "<name>": it must start with a single "/" followed by the service` |
| No level, and the method is enforced | `Internal` | `missing auth_level annotation on <method> (annotate the RPC, or import its generated package before loading the levels)` |
| Unspecified or unknown level | `PermissionDenied` | `auth_level <level> on <method> is denied` |
| No user ID | `Unauthenticated` | `authentication required` |
| Role too low | `PermissionDenied` | `<required role> role required`, naming the lowest role that passes |
| Admin level on an empty `Ladder{}` | `PermissionDenied` | `no role on the ladder admits this call` |
| Callback said no | `PermissionDenied` | `caller is not on the allow-list` |
| Callback returned an error | `Internal` | `allow-list check failed` |

## The caller in the context

Your login step records the caller, and the interceptors read it back.

| Record with | Read with |
|---|---|
| `WithUserID` | `UserIDFromContext` |
| `WithRole` | `RoleFromContext` |
| `WithEmail` | `EmailFromContext` |

- **A reader returns ""** when the matching `With` call never ran.
- **The allow-list callback receives that ""** as the email. It must not treat "" as a match.
- **A nil callback denies every call.**
- **The callback sees only authenticated callers.** An error from it denies the call with
  `Internal`.

## `NewLadder`

`NewLadder(roles, admin, superAdmin)` builds a ladder of your own roles.
`ladder.UnaryInterceptor` and `ladder.StreamInterceptor` take the place of the two package-level
interceptors, which always use the default roles.

- **Roles** are listed lowest first. Each must be a distinct, non-empty name.
- **The admin and super admin roles** must be on the list, and the super admin role no lower.
- **`DefaultLadder()`** is member, admin, super_admin, with admin and super_admin as the two
  roles. The constants are `RoleMember`, `RoleAdmin` and `RoleSuperAdmin`.
- **An empty `Ladder{}`** has no roles, so it denies both admin levels.
- **Roles are exact strings.** `Admin` is not `admin`, and an unknown role ranks lowest.

## The missing-level checks

`MissingAuthLevels(files...)` and `protoauthztest.RequireAuthLevels(t, files...)` check the proto
files you pass. `MissingAuthLevelsFromRegistry(prefixes...)` and
`protoauthztest.RequireAuthLevelsFromRegistry(t, prefixes...)` check every linked RPC.

- **They report** every RPC with no `auth_level`, with `AUTH_LEVEL_UNSPECIFIED`, or with a value
  this package does not know.
- **`MissingAuthLevels`** returns full method names, sorted.
- **`RequireAuthLevels`** calls `t.Errorf` once per RPC, naming each. It also fails when the
  files hold no RPC.
- **The registry checks** use the global registry. Prefixes take the interceptors' form and
  rules.
- **Prefixes limit only which RPCs with no `auth_level` are reported.** Unspecified or unknown
  levels are reported everywhere.
- **Only linked RPCs are seen.** `MissingAuthLevelsFromRegistry` adds nothing for a prefix with
  no linked RPC under it.
- **`RequireAuthLevelsFromRegistry` fails on a prefix with no linked RPC under it.** Import the
  package, or fix the prefix.

## Versioning and compatibility

protoauthz follows [Semantic Versioning](https://semver.org/). Before v1.0, a minor release may
rename or change the API, and [CHANGELOG.md](../CHANGELOG.md) says so. A changed status code or
error message counts as a breaking change.

The proto package `protoauthz.v1`, its level values and the extension number 2800105 change
only in a major release. Go 1.26 is the floor, and CI tests 1.26 and 1.27.

## Protobuf extension registration

`protoauthz.v1.auth_level` uses MethodOptions extension number `2800105`.
This number is not yet reserved in the [global extension registry](https://github.com/protocolbuffers/protobuf/blob/main/docs/options.md).
Another library using the same number on MethodOptions can cause descriptor registration to
fail when both are linked. Check for collisions in applications that combine custom options.
Do not renumber compiled descriptors independently of their source proto files.
