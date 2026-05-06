# AGENTS.md

## Project map

- Orchard is a Go orchestration system for Tart- and Vetu-backed VMs on macOS and Linux hosts.
- `internal/controller/` owns the API surface, scheduling, SSH server, and exec/session coordination.
- `internal/worker/` owns worker-side RPC handling and VM lifecycle integration.
- `pkg/client/` and `pkg/resource/v1/` are the public client/resource packages.
- `api/openapi.yaml` documents the REST API.
- `rpc/` contains protobuf definitions plus generated Go output.

## Local workflow

- Format Go changes with `gofmt`.
- Run targeted tests while iterating, then prefer `go test ./...` before shipping when the host environment supports it.
- Lint with the repository configuration in `.golangci.yml`, for example:
  - `golangci-lint run`
  - or `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run`
- Some integration tests require host-specific VM support and may not be portable to every development host; when possible, add synthetic coverage for controller/worker behavior as well.

## Generated code

- If you change files under `rpc/*.proto`, run `buf generate` from the repository root and commit the regenerated outputs.
- Keep API behavior changes aligned with `api/openapi.yaml`.

## Change guidance

- Prefer small, behavior-focused changes that keep controller and worker protocol expectations in sync.
- When touching `/exec`, port-forwarding, or reconnectable session behavior, cover both lifecycle cleanup and concurrent access paths.
- Preserve public API compatibility unless the change explicitly calls for an API revision.
