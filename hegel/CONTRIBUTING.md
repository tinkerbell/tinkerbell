# Contributing

## Expectations

When contributing you must adhere to the [Code of Conduct](coc). All contributions must be signed 
off in accordance with the [Developer Certificate of Origin](dco).

## Developing

### Pre-requisits

- Make
- [Docker](https://www.docker.com/)
- [Go](https://go.dev/) installed at the version specified in the [CI workflow](ci-workflow)
- [Python 3](https://www.python.org/) installed and available on the path as `python3`
- [Pip](https://pypi.org/project/pip/) for Python 3
- [cURL](https://curl.se/)

### Developer workflow

All builds happen via the Makefile at the root of the project. `make help` provides the set of
most commonly used targets with short descriptions.

When developing, ensure you write unit tests and leverage the various `test` Makefile targets
to validate your code.

The CI invokes little more than a Makefile target for each job. The one exception is image building
as we optimize for cross-platform builds. In brief, we cross compile using the Go toolchain before
constructing the image by copying the appropriate binary for the target platform.


### Package structure

Given Hegel is not a library of reusable components most of its code lives in `/internal`.
Appropriate justification will be required to create packages outside `/internal`.

The `main()` func for Hegel is located in `/cmd/hegel`. It is extremely short with the core command
logic residing in `/internal/cmd`.

Hegel is split into frontends and backends. The frontends are the core domain logic while the 
backends are clients into a particular kind of backend. Frontends declare the models they require 
and the backends are responsible for retrieving and supplying the data in the required format. 
See the [frontend-backend][frontend-backend] Plant UML for a depiction.

## How to submit change requests

Please submit change requests and features via [Issues].

## How to report bugs

Please submit bugs via [Issues].

[issues]: https://github.com/tinkerbell/hegel/issues
[coc]: https://github.com/tinkerbell/.github/blob/main/CODE_OF_CONDUCT.md
[dco]: /docs/DCO.md
[ci-workflow}]: /.github/workflows/ci.yaml