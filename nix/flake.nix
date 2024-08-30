{
  description = "prometheus-healthchecks";

  inputs = {
    nixpkgs.url = "nixpkgs/nixpkgs-unstable";
    utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, utils }: {
    nixosModules.default = { config, lib, pkgs, ... }: {
      options.services.prometheus-healthchecks = {
        enable = lib.mkEnableOption "prometheus-healthchecks";

        check-uuid-file = lib.mkOption { type = lib.types.str; };

        prometheus-base-url = lib.mkOption {
          type = lib.types.str;
          default =
            let inherit (config.services.prometheus) listenAddress port; in
            "http://${listenAddress}:${toString port}";
        };
      };

      config = lib.mkIf config.services.prometheus-healthchecks.enable {
        nixpkgs.overlays = [ self.overlays.default ];

        systemd.services.prometheus-healthchecks = {
          description = "prometheus-healthchecks";
          serviceConfig.ExecStart =
            let inherit (config.services.prometheus-healthchecks)
              check-uuid-file prometheus-base-url;
            in
            "${pkgs.prometheus-healthchecks}/bin/prometheus-healthchecks --check-uuid-file ${check-uuid-file} --prometheus-base-url ${prometheus-base-url}";
          wantedBy = [ "multi-user.target" ];
        };
      };
    };
    nixosModules.prometheus-healthchecks = self.nixosModules.default;

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
