# Difficult Hardware

Most modern hardware is capable of PXE booting just fine.
Sometimes strange combinations of different NIC hardware / firmware connected
to specific switches can misbehave.

In those situations you might want to boot into a build of iPXE but completely
sidestep the PXE stack in your NIC firmware.

We already ship ipxe.iso that can be used in many situations, but most of the
time that requires either an active connection from a virtual KVM client
or network access from the BMC to a storage target hosting the ISO.

Some BMCs support uploading a floppy image into BMC memory and booting from that.
To support that use case we have started packaging our EFI build into a bootable
floppy image that can be used for this purpose.

For other projects or use cases that wish to replicate this functionality, with
the appropriate versions of qemu-img, dosfstools and mtools you can build something
similar yourself from upstream iPXE like so:

```
# create a 1440K raw disk image
qemu-img create -f raw ipxe-efi.img 1440K
# format it with an MBR and a FAT12 filesystem
mkfs.vfat --mbr=y -F 12 -n IPXE ipxe-efi.img

# Create the EFI expected directory structure
mmd -i ipxe-efi.img ::/EFI
mmd -i ipxe-efi.img ::/EFI/BOOT

# Copy ipxe.efi as the default x86_64 efi boot file
curl -LO https://boot.ipxe.org/ipxe.efi
mcopy -i ipxe-efi.img ipxe.efi ::/EFI/BOOT/BOOTX64.efi
```

As of writing other projects are working on automating the upload
of this floppy to a BMC.
See draft PR https://github.com/bmc-toolbox/bmclib/pull/347
