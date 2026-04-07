{ pkgs ? import <nixpkgs> {}, ... }:
pkgs.mkShellNoCC {
  packages = [
    pkgs.go
    pkgs.kubebuilder
    pkgs.docker
    pkgs.docker-buildx
  ];
}
