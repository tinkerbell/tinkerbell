## Hello Contributors!

Thanks for your interest!
We're so glad you're here.

### Important Resources

#### bugs: [https://github.com/tinkerbell/hegel/issues](https://github.com/tinkerbell/hegel/issues)

### Code of Conduct

Please read and understand the code of conduct found [here](https://github.com/tinkerbell/.github/blob/master/CODE_OF_CONDUCT.md).

### Environment Details

#### Nix

This repo's build environment can be reproduced using `nix`.

##### Install Nix

Follow the [Nix installation](https://nixos.org/download.html) guide to setup Nix on your box.

##### Load Dependencies

Loading build dependencies is as simple as running `nix-shell` or using [lorri](https://github.com/nix-community/lorri).
If you have `direnv` installed the included `.envrc` will make that step automatic.

### How to Submit Change Requests

Please submit change requests and / or features via [Issues](https://github.com/tinkerbell/hegel/issues).
There's no guarantee it'll be changed, but you never know until you try.
We'll try to add comments as soon as possible, though.

### How to Report a Bug

Bugs are problems in code, in the functionality of an application or in its UI design; you can submit them through [Issues](https://github.com/tinkerbell/hegel/issues).

## Code Style Guides

#### Protobuf

Please ensure protobuf related files are generated along with _any_ change to a protobuf file.
In the future CI will enforce this, but for the time being does not.
