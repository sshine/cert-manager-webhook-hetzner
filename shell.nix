{ pkgs ? import <nixpkgs> {}, ... }:
pkgs.mkShellNoCC {
  packages = [
    pkgs.go
    pkgs.golangci-lint
    pkgs.kubebuilder
    pkgs.docker
    pkgs.docker-buildx
  ];
}
