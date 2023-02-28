let
  _pkgs = import <nixpkgs> {};
in
  {
    pkgs ?
      import (_pkgs.fetchFromGitHub {
        owner = "NixOS";
        repo = "nixpkgs";
        #branch@date: nixos-21.11@2022-08-02
        rev = "eabc38219184cc3e04a974fe31857d8e0eac098d";
        sha256 = "04ffwp2gzq0hhz7siskw6qh9ys8ragp7285vi1zh8xjksxn1msc5";
      }) {},
  }:
    with pkgs; let
    in
      mkShellNoCC {
        buildInputs =
          [
            curl
            expect
            gcc9
            git
            gnumake
            gnused
            go
            perl
            xorriso
            xz
          ]
          ++ lib.optionals stdenv.isLinux [
            pkgsCross.aarch64-multiplatform.buildPackages.gcc9
          ];
      }
