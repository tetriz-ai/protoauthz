# Changelog

All notable changes to this project are documented in this file. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-10-22

### Added

- Initial release.
- The `protoauthz.v1.auth_level` method option declares each RPC's access level.
- gRPC interceptors deny any call whose level is missing, unknown or not met.
- `protoauthztest.RequireAuthLevels` fails a test when an RPC has no level.
- `MissingAuthLevelsFromRegistry` and `protoauthztest.RequireAuthLevelsFromRegistry` check every
  linked RPC: those with no level under your prefixes, and those with an unspecified or unknown
  level anywhere.

[Unreleased]: https://github.com/tetriz-ai/protoauthz/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/tetriz-ai/protoauthz/releases/tag/v0.1.0
