{
  description = "sqlite-bitmap-store";
  inputs = {
    nixpkgs.url = "https://channels.nixos.org/nixos-unstable/nixexprs.tar.xz";

    systems.url = "github:nix-systems/default";
  };

  outputs =
    {
      nixpkgs,
      systems,
      ...
    }:
    let
      eachSystem =
        f: nixpkgs.lib.genAttrs (import systems) (system: f system nixpkgs.legacyPackages.${system});
    in
    {

      devShells = eachSystem (
        system: pkgs: {
          default = pkgs.mkShell {
            shellHook = ''
              # Set here the env vars you want to be available in the shell
            '';
            hardeningDisable = [ "all" ];

            packages = with pkgs; [ go ];
          };
        }
      );
    };
}
