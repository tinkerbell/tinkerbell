# OSIE files

`spec.metadata.osieFiles` delivers per-host files to the OSIE (Operating System
Installation Environment) before its networking starts, for example network
configuration with multiple interfaces, VLANs or bonds. Smee puts the files in an
uncompressed CPIO archive that the kernel extracts as an additional initramfs, so they
appear under `/osie_files` in the OSIE.

## Example

```yaml
apiVersion: tinkerbell.org/v1alpha1
kind: Hardware
metadata:
  name: edge-01
spec:
  metadata:
    osieFiles:
      - path: network/config.json
        contents: '{"bond0": {"interfaces": ["eth0", "eth1"]}}'
        mode: "0644"
```

The OSIE sees `/osie_files/network/config.json`.

- `path` is relative to `/osie_files`. A leading `/` is ignored and `..` is rejected.
- `contents` is a string.
- `mode` is an octal string, at most `0777`. The default is `0400`.

## Enable delivery

Set `--bootstrap-enabled`, or `deployment.envs.smee.bootstrapEnabled: true` with the
Helm chart. Both routes below follow this setting.

### iPXE

Smee's default iPXE script loads `/bootstrap/<mac>/bootstrap.cpio` as a second initrd,
after the OS initramfs. The path is root-relative, so iPXE fetches it from the same
scheme, host and port it fetched the script from. A custom iPXE script must add the
extra `initrd` line itself, with Smee's absolute URL if Smee does not serve that
script. As with the iPXE script, the archive is
only served while the interface's `netboot.allowPXE` is `true`; otherwise it is a 403.

### ISO

When Smee serves an ISO that reserves a bootstrap slot, it writes the archive into the
slot as the ISO streams through. CaptainOS ISOs reserve one; other ISOs, such as
HookOS, are served unmodified. Smee fetches no metadata about the ISO: every 2 KiB
sector of an unpersonalized slot starts with a header that identifies it, so Smee
finds the slot in whatever bytes it serves, the same way it finds the kernel command
line it patches. Redirects, and ISOs replaced at the same URL, need no special care.

Like the kernel command line patch, this is best effort. The slot is personalized in
every response that contains a whole sector header, which covers full downloads and
the sector reads BMCs make when mounting virtual media. A response that cannot be
placed is served unmodified: a range shorter than a sector that starts and ends inside
one sector, or a multipart response to a request for several ranges at once. An
unpersonalized slot holds no files, so the ISO still boots.

The slot has a fixed capacity set when the ISO is built (4 MiB for CaptainOS). An
archive that does not fit is not written, and Smee logs an error; the iPXE route has no
such limit. Like the rest of the ISO, the slot does not depend on `netboot.allowPXE`.

## Notes

- **Rendering:** with Hardware rendering enabled, `contents` are rendered, so a file can
  carry data from a referenced Secret or ConfigMap, including binary data through
  `b64dec`. Skip literal Jinja or other `{{` text with the `tinkerbell.org/render-skip`
  annotation. See [Hardware templating](HARDWARE_TEMPLATING.md).
- **No authentication:** both routes serve the archive to anyone who knows the
  machine's MAC address, as Tootles serves metadata and Smee serves iPXE scripts.
- **Edits during a boot:** a BMC mounts an ISO with many range requests. Editing the
  Hardware during a mount can mix the old and new archive in what the machine reads.
