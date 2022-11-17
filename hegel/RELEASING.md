# Releasing

## Process

For version v0.x.y:

1. Ensure you have a remote configured for https://github.com/tinkerbell/hegel (you'll need 
maintainer permissions to push).
1. Ensure the CI has successfully built an image on the target branch containing the commit you wish
to release.
1. Verify you're checked out and up-to-date on the desired branch. 
1. Create a pre-release by tagging the `vM.m.p-rc<n>` and push. For example, v0.1.0 release 
candidate 1 would be `v0.1.0-rc1`.
1. Validate the release exists on GitHub and that the image works as expected.
1. Create a release by tagging the vM.m.p and push.
1. Validate the release exists on GitHub and that the image works as expected.

This release process is relevant for major version 0 and should be reviewed before moving to v1.

