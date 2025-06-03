# Releasing

## Process

This repo has two Go modules, the top level module and the `api` module. The top level module is for the Tinkerbell and Agent binaries. The `api` module is for the API definitions.
The top level module is versioned as `vX.Y.Z`. The `api` module is versioned as `api/vX.Y.Z`. Both version tags are created using the `./script/tag-release.sh` script.

> [!NOTE]  
> Both modules must be released at the same time, so the version numbers match.

For version vX.Y.Z:

1. Create the annotated tag
   > NOTE: To use your GPG signature when pushing the tag, use `SIGN_TAG=1 ./script/tag-release.sh v0.x.y` instead
   - `./script/tag-release.sh v0.x.y`
1. Push the tag to the GitHub repository. This will automatically trigger a [Github Action](https://github.com/tinkerbell/tinkerbell/actions) to create a release.
   > NOTE: `origin` should be the name of the remote pointing to `github.com/tinkerbell/tinkerbell`
   - `git push origin v0.x.y`
1. Review the release on GitHub.

For version `api/vX.Y.Z`:

1. Create the annotated tag
   > NOTE: To use your GPG signature when pushing the tag, use `SIGN_TAG=1 ./script/tag-release.sh api/v0.x.y` instead
   - `./script/tag-release.sh api/v0.y.z`
1. Push the tag to the GitHub repository. This will **NOT** trigger a [Github Action](https://github.com/tinkerbell/tinkerbell/actions) of any kind.
   > NOTE: `origin` should be the name of the remote pointing to `github.com/tinkerbell/tinkerbell`
   - `git push origin api/v0.x.y`
1. The `api` module is now available for use by importing it as `github.com/tinkerbell/tinkerbell/api vX.Y.Z`.

> [!NOTE]  
> The `api` module release does not create a GitHub release. It only creates a tag in the repository.


### Permissions

Releasing requires a particular set of permissions.

- Tag push access to the GitHub repository
