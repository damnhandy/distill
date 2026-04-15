{
  description = "Build minimal, immutable OCI images from enterprise Linux base distributions";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          version = self.shortRev or self.dirtyShortRev or "dev";
        in
        {
          distill = pkgs.buildGoModule {
            pname = "distill";
            inherit version;
            src = ./.;

            # Run `nix build` with vendorHash = pkgs.lib.fakeHash to compute this.
            vendorHash = "sha256-mBbmZK8u+vLskVpm0Y0aot3noXQ1J75cATGtmhVg5Pc=";

            ldflags = [
              "-s"
              "-w"
              "-X github.com/damnhandy/distill/cmd.Version=${version}"
            ];

            meta = with pkgs.lib; {
              description = "Build minimal, immutable OCI images from enterprise Linux base distributions";
              homepage = "https://github.com/damnhandy/distill";
              license = licenses.asl20;
              maintainers = [ ];
              mainProgram = "distill";
              platforms = platforms.unix;
            };
          };

          default = self.packages.${system}.distill;
        }
      );

      # Development shell — equivalent to `devbox shell` for users without devbox.
      # Usage: nix develop
      devShells = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go
              golangci-lint
              goreleaser
              cosign
              syft
            ];
          };
        }
      );
    };
}
