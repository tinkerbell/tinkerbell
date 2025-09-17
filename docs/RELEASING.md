# Releasing

## Process

This repo has two Go modules, the top level module and the `api` module. The top level module is for the Tinkerbell and Agent binaries. The `api` module is for the API definitions.
The top level module is versioned as `vX.Y.Z`. The `api` module is versioned as `api/vX.Y.Z`. Both version tags are created using the `./script/tag-release.sh` script.

For version vX.Y.Z:

1. Create the annotated tag
   > NOTE: To use your GPG signature when pushing the tag, use `SIGN_TAG=1 ./script/tag-release.sh v0.x.y` instead
   - `./script/tag-release.sh v0.x.y`
1. Push the tags to the GitHub repository. This will automatically trigger a [Github Action](https://github.com/tinkerbell/tinkerbell/actions) to create a release.
   > NOTE: `origin` should be the name of the remote pointing to `github.com/tinkerbell/tinkerbell`
   - `git push origin v0.x.y`
   - `git push origin api/v0.x.y`
1. Review the release on GitHub.

> [!NOTE]  
> The `api` module tag does not create a GitHub release. It only creates a tag in the repository.

### Permissions

Releasing requires a particular set of permissions.

- Tag push access to the GitHub repository

### Cadence

There are 2 types of releases: (1) Git tagged releases and (2) mainline releases. Both types of releases publish container images and Helm chart images to the GitHub Container Registry (ghcr.io). They can be found at the [GitHub Packages page](https://github.com/orgs/tinkerbell/packages?repo_name=tinkerbell).

#### Git tagged releases

Git tagged releases are releases that have a corresponding Git tag (vX.Y.X). Run `git tag` to see the list of Git tags or view them on the [GitHub tags page](https://github.com/tinkerbell/tinkerbell/tags). Git tagged releases are made at the discretion of the maintainers. Generally, if there is a major bug fix in main then a new Git tagged release will be made.
If there are no major bug fixes, releases are generally made when a maintainer deems enough fixes and/or features have accumulated.

In a Git tagged release, at a minimum, the following artifacts are created along with a Git tag:

- [Tinkerbell container image](https://github.com/tinkerbell/tinkerbell/pkgs/container/tinkerbell)
- [Tinkerbell Agent container image](https://github.com/tinkerbell/tinkerbell/pkgs/container/tink-agent)
- [Tinkerbell Helm chart OCI image](https://github.com/tinkerbell/tinkerbell/pkgs/container/charts%2Ftinkerbell)
- [Tinkerbell AMD64 and ARM64 embedded binaries](https://github.com/tinkerbell/tinkerbell/releases)
- [An official GitHub release](https://github.com/tinkerbell/tinkerbell/releases)

#### Mainline releases

Mainline releases are releases that do not have a corresponding Git tag. The tagging that occurs is only container image and Helm chart image tagging. They are made automatically on each merge to main. The tag format is `vX.Y.(Z+1)-<short-commit-hash>`. For example, if the latest Git tagged release is `v0.21.0` and the commit hash of the merge to main is `0f5c5863`, then the mainline release artifacts tag will be `v0.21.1-0f5c5863`.

In a mainline release, at a minimum, the following artifacts are created:

- [Tinkerbell container image](https://github.com/tinkerbell/tinkerbell/pkgs/container/tinkerbell)
- [Tinkerbell Agent container image](https://github.com/tinkerbell/tinkerbell/pkgs/container/tink-agent)
- [Tinkerbell Helm chart OCI image](https://github.com/tinkerbell/tinkerbell/pkgs/container/charts%2Ftinkerbell)
