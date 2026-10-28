# Host script Actions

A Tinkerbell Action normally names an OCI image, which the Agent hands to a container runtime
(Docker, containerd, or Kubernetes). An Action can instead carry an inline script in `run:`, which
the Agent writes to a file on the host and executes directly — no image, no registry, no container.

This is aimed at deployments where `tink-agent` already runs as root on a capable host OS with a
shell and the usual tooling available. Wrapping a handful of commands in an image, publishing it,
and pulling it back onto that host is overhead with nothing to show for it.

The choice is made per Action, not per Agent, so a single Template can freely mix image Actions and
script Actions. No Agent flag or runtime mode enables this.

## Fields

| Field | Type | Description |
|---|---|---|
| `run` | string | The script body. Mutually exclusive with `image`; exactly one of the two must be set. |
| `shell` | list of strings | The interpreter argv. Defaults to `["bash", "-x", "-e"]`. Only valid together with `run`. |
| `background` | bool | Report the Action successful *before* running it, then run it detached. Valid on image Actions too. |

```yaml
version: "0.1"
name: provision
tasks:
  - name: provision
    worker: "{{.device_1}}"
    actions:
      # An ordinary image action, unchanged.
      - name: stream-image
        image: quay.io/tinkerbell/actions/image2disk:v1.0.0
        timeout: 600
        environment:
          DEST_DISK: /dev/nvme0n1

      # A host script, run with the default `bash -x -e`.
      - name: label-filesystem
        timeout: 60
        run: |
          e2label /dev/nvme0n1p2 root
          blkid /dev/nvme0n1p2

      # A different interpreter. `shell` replaces the default entirely, argv and all.
      - name: collect-inventory
        shell: ["python3", "-u"]
        timeout: 120
        run: |
          import json, subprocess
          print(json.dumps({"lscpu": subprocess.check_output(["lscpu"]).decode()}))

      # Reported successful, then detached. The workflow completes; then the machine goes down.
      - name: reboot
        background: true
        run: |
          systemctl reboot
```

## The default shell

`bash -x -e` is the default because both flags matter for an Action:

- `-e` aborts at the first failing command, so a script that dies halfway cannot be reported as a
  success.
- `-x` traces every command to stderr, so the Agent log shows what actually ran — the equivalent of
  being able to read a container's layers.

Setting `shell:` replaces this entirely. `shell: ["bash"]` gets you neither flag.

## Environment

The script inherits the Agent's own environment (`PATH`, `HOME`, and so on), and the Action's
variables are layered on top. Precedence, lowest to highest:

1. The Agent process's environment
2. The Task's `environment:` map
3. The Action's `environment:` map

This differs from container Actions, which start from the image's environment rather than the
Agent's.

## Output

Stdout and stderr are streamed to the Agent's log, one entry per line, tagged with the Action name
and the stream it came from. This matches the container runtimes, which likewise surface Action
output only through the Agent log — nothing is written back to the Workflow object.

## `background:`

A background Action is reported `SUCCESS` before it is executed, and then run detached, with no
timeout and no cancellation.

This exists for Actions that take the Agent down with them: reboot, kexec, power off. Run normally,
such an Action can never report its result — the machine is gone before the report is sent — and
the Workflow hangs in-flight forever. Reporting first lets the Workflow reach a completed state,
and the machine goes down afterwards.

The cost is real and worth stating plainly: **a background Action's outcome is invisible to the
Workflow.** If the script fails, the Workflow still shows the Action as successful. The failure is
logged by the Agent and nowhere else. Use it only where the Action's whole purpose is to end the
Agent's participation.

The next Action, if any, is served immediately, while the background one is still running.

## Cancellation and timeouts

A non-background script Action honours the Action's `timeout:`. The script runs in its own process
group, and cancellation kills the whole group — a shell script's real work usually happens in child
processes, and killing only the shell would leave them running against a machine the Workflow has
moved on from.

A `timeout:` of zero, or an omitted one, means no timeout.

## Security

`run:` executes with the full privileges of the Agent process. Where the Agent runs as root, so
does every `run:` script — there is no container boundary of any kind. Anyone who can create or
edit a Template can run arbitrary code as root on every machine that Template targets.

This is not a change in the trust model so much as a change in how obvious it is: image Actions
already run privileged, with host devices and namespaces available. Treat write access to Templates
as equivalent to root on the fleet either way.

The script file is written mode `0700` and removed once the script exits.

## API versions

v1alpha1 is implemented, and is what this document describes.

v1alpha2's `Action` carries the same `run` and `shell` fields, and already had `background` — which
is why v1alpha1 uses that name rather than something like `daemon`. v1alpha2 has no execution path
yet, so those fields are shape only for now.
