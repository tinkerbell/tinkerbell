# CAPT development setup with the Playground

This document describes how to set up a development environment for CAPT using the Playground.

The following repositories are required for the CAPT development setup.

- `github.com/tinkerbell/cluster-api-provider-tinkerbell`
- `github.com/tinkerbell/playground`.

## Setup

1. Fork the CAPT repository in GitHub.
1. Clone CAPT and the Playground repositories to your local machine.

    ```bash
    git clone git@github.com:<your user>/cluster-api-provider-tinkerbell.git
    git clone https://github.com/tinkerbell/playground.git
    ```

1. Create a local release in CAPT.

    ```bash
    cd cluster-api-provider-tinkerbell
    make release-local
    ```

1. In the `config.yaml`, set `capt.providerRepository` to the local release directory, minus the version. For example: `capt.providerRepository: /home/user/cluster-api-provider-tinkerbell/out/release/infrastructure-tinkerbell`

    ```bash
    cd playground/capt
    sed -i 's|providerRepository:.*|providerRepository: /home/user/cluster-api-provider-tinkerbell/out/release/infrastructure-tinkerbell|' config.yaml
    ```

1. Create the CAPT playground. Once the playground gets to the prompt waiting for user input, leave it for now and move on to the next step in this doc.

    ```bash
    cd playground/capt
    task create-playground
    ```

1. Deploy the local CAPT code.

    ```bash
    cd cluster-api-provider-tinkerbell
    export KUBECONFIG=<playground repo>/capt/output/kind.kubeconfig
    tilt up --stream
    ```

1. Enter `y` in the CAPT Playground prompt and follow the post creation instructions.
