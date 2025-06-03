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
