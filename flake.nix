{
  description = "OpenTofu/Terraform provider for Fogpipe";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            gotools
            just
            gnupg
            goreleaser
            terraform-plugin-docs
            opentofu
          ];

          # Let the go.mod toolchain directive fetch the exact Go if the nixpkgs
          # Go is older than the module requires.
          env.GOTOOLCHAIN = "auto";

          shellHook = ''
            echo "terraform-provider-fpcloud dev shell — run 'just' for tasks"
          '';
        };
      });
}
