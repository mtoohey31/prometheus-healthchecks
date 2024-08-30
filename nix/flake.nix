{
  description = "prometheus-healthchecks";

  inputs = {
    nixpkgs.url = "nixpkgs/nixpkgs-unstable";
    utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, utils }: {
    overlays.default = final: _: {
      prometheus-healthchecks = final.callPackage
        ({ buildGoModule }: buildGoModule {
          pname = "prometheus-healthchecks";
          version = "0.1.0";
          src = builtins.path {
            path = ./..;
            name = "prometheus-healthchecks-src";
          };
          vendorHash = null;
        })
        { };
    };
  } // utils.lib.eachDefaultSystem (system:
    let
      pkgs = import nixpkgs {
        overlays = [ self.overlays.default ];
        inherit system;
      };
      inherit (pkgs) gopls mkShell prometheus-healthchecks;
    in
    {
      packages.default = prometheus-healthchecks;

      devShells.default = mkShell {
        inputsFrom = [ prometheus-healthchecks ];
        packages = [ gopls ];
      };
    });
}
