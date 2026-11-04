# AGENTS.md

Guidance for AI coding agents working in this repository. [docs/AI_POLICY.md](docs/AI_POLICY.md) applies at all times and takes precedence over this file.

## Project

Tinkerbell provisions bare metal machines. It's one Go binary made of isolated components, plus a separate Agent binary:

| Path | Component |
|---|---|
| `smee` | Network boot server (DHCP, TFTP, iPXE) |
| `tink/server`, `tink/controller`, `tink/agent` | Workflow server, controller, and Agent |
| `tootles` | Metadata service |
| `secondstar` | Serial over SSH |
| `rufio` | BMC controller |
| `api` | CRD Go types (separate Go module) |
| `crd` | CRD manifests (generated, in `crd/bases`) and code to install them |
| `helm` | Helm chart |
| `ui` | Web UI |
| `cmd` | Binary entry points and flags |
| `pkg` | Code shared between components; add here only when necessary |

Components must not import each other. Check with `make dep-graph`.

## Commands

```sh
make build          # Tinkerbell binary
make test           # Unit tests; TEST_PKG=smee scopes to one package tree
make test-api       # Tests for the api module
make lint           # golangci-lint and other linters
make vet
make ci-checks      # Fails if generated code or go.mod is out of date
make ci             # Everything CI runs
make helm-lint helm-test
```

## Generated code

Never edit generated files by hand: CRD manifests, `zz_generated.deepcopy.go`, protobuf code, and UI templ/CSS output. Change the source, then run:

```sh
make generate generate-proto ui-generate manifests
```

Commit the results in the same commit as the source change.

## Code

- Follow [docs/DESIGN_PHILOSOPHY.md](docs/DESIGN_PHILOSOPHY.md): no package-level variables or `func init`, `context.Context` first, accept interfaces and return structs, and only `main` decides flags and config.
- Write doc comments per [Go Doc Comments](https://go.dev/doc/comment). Comment why, not how.
- Code must not rely on API server defaults. Objects built in Go, including those passed to fake clients in tests, aren't defaulted.
- Keep coverage within 1% of `main`.
- Put design docs in `docs/technical/designs/`.

## Commits and PRs

- Commit with `git commit -s`. Never write a `Signed-off-by` trailer yourself.
- Never add `Co-authored-by`, `Assisted-by`, or similar trailers naming an AI tool.
- Commit title says what changed; body says why.
- Fill in the AI Assistance section of the [PR template](.github/PULL_REQUEST_TEMPLATE.md). When drafting a PR description, name your tool and model there.
- Keep each PR small and focused on one change. Split large designs into a series of PRs that can each be reviewed on their own.
- Don't push to `main`. Use a fork and a PR.
