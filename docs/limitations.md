# Limitations

Check these before you ship on a service whose data matters. Each entry is a limit of gRPC or of
the design. The harmful direction comes first in each group. The exact rules are in
[reference.md](reference.md).

## The interceptors

**Allows more than it should (can leak):**

- **Unary interceptors do not check streams.** Install both unary and stream login and authorization chains.
- **The context is trusted as is.** The interceptors believe the user ID and role your login
  step records.
- **Never copy a client-sent header into `WithUserID` or `WithRole`.** Anyone could send it and
  gain that identity.
- **Only calls through a `grpc.Server` are checked.** `RegisterXxxHandlerServer`, connect-go HTTP handlers and direct
  struct calls skip it. Dial the server instead.
- **With prefixes, unannotated methods outside them pass.** Annotate those methods, or pass no
  prefixes.
- **A prefix that matches nothing denies no unannotated method.** `RequireAuthLevelsFromRegistry`
  on the same prefixes catches it.
- **Proxy-style `UnknownServiceHandler` paths need their own authorization.** Noncanonical paths can fall outside configured prefixes; gRPC does not route those paths to registered methods.
- **A stream is checked once, when it opens.** A role taken away during a long stream does not
  end it.

**Denies more than it should (harmless):**

- **An RPC whose generated package is not imported has no level,** so it is denied with
  `Internal`.
- **Roles are exact strings.** `Admin` is not `admin`, and an unknown role ranks lowest.
- **gRPC's health and reflection services have no `auth_level`.** Without prefixes they are
  denied. Pass prefixes to let them through.

## The missing-level checks

**Misses an RPC with no level:**

- **`RequireAuthLevels` checks only the files you pass.** List a new proto file there when you
  add one.
- **The registry checks see only linked RPCs.** Put the test in the package that registers your
  services on the `grpc.Server`.
- **A broad prefix such as `/your.` passes** when some linked RPC is under it.
- **A served package that the test does not link goes unchecked.** The interceptor then denies
  its unannotated RPCs with `Internal`.

**Reports an RPC that is fine:**

None known.

## Out of scope

- **It does not authenticate anyone.** Your login step checks the token or session and calls
  `WithUserID`.
- **It does not store roles or allow-lists.** Your login step supplies the role, and your
  callback decides the allow-list.
- **It checks a whole RPC.** It does not look at the request's fields, such as which record is
  asked for.
- **It handles no tenant or organization.** Your own code does that.
