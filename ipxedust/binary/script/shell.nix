let
  _pkgs = import <nixpkgs> {};
in
  {
    pkgs ?
      import (_pkgs.fetchFromGitHub {
        owner = "NixOS";
        repo = "nixpkgs";
        #branch@date: release-23.05@2023-09-11
        rev = "4610292e25a414c2b111a7d99075cf1683e5a359";
        hash = "sha256-ddMK+MKncPuPgMzsR72ndx4VObAjnM+R73+F6wL0aPs=";
      }) {},
  }:
    with pkgs; let
    in
      mkShellNoCC {
        buildInputs =
          [
            curl
            dosfstools
            expect
            gcc12
            git
            gnumake
            gnused
            go
            gptfdisk
            mtools
            perl
            qemu-utils
            syslinux
            xorriso
            xz
          ]
          ++ lib.optionals stdenv.isLinux [
            pkgsCross.aarch64-multiplatform.buildPackages.gcc12
          ];
      }
