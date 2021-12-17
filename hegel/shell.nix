let _pkgs = import <nixpkgs> { };
in { pkgs ? import (_pkgs.fetchFromGitHub {
  owner = "NixOS";
  repo = "nixpkgs";
  #branch@date: 21.11@2021-12-15
  rev = "3253c2c9636d4213df47e94fd19b259fba43f323";
  sha256 = "1z618cgxsdkbqvvbvjbggigwd2m64l2d1ixjs3dg0m1zjv0id969";
}) { } }:

with pkgs;

mkShell { buildInputs = [ go_1_17 protobuf ]; }
