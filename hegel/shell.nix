let _pkgs = import <nixpkgs> { };
in { pkgs ? import (_pkgs.fetchFromGitHub {
    owner = "NixOS";
    repo = "nixpkgs-channels";
    #branch@date: nixpkgs-unstable@2020-01-21
    rev = "e48d9e6871624e016b1281378ac0a68490aeb31e";
    sha256 = "1fg445jkv30a2ikzxx1m2697sp83y2n220r76r3n45dc7bmj4875";
  }) { } }:

with pkgs;

mkShell { buildInputs = [ go protobuf ]; }
