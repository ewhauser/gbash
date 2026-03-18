{
  description = "gbash development tools";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in {
      packages = forAllSystems (pkgs: {
        bash = pkgs.bash;
        bats = pkgs.bats;
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = [
            # Shell and testing
            pkgs.bash
            pkgs.bats

            # Go toolchain
            pkgs.go
            pkgs.golangci-lint
            pkgs.goreleaser

            # Build tools
            pkgs.gnumake

            # Version control and GitHub
            pkgs.git
            pkgs.gh

            # Standard utilities
            pkgs.coreutils
            pkgs.findutils
            pkgs.curl
            pkgs.jq

            # Container runtime (for compat tests)
            pkgs.docker

            # Website development
            pkgs.nodejs
            pkgs.pnpm
          ];
        };
      });
    };
}
