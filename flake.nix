{
  description = "A basic flake with a shell";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system: let
      pkgs = nixpkgs.legacyPackages.${system};

      myApp = pkgs.buildGoModule {
        pname = "my-go-app";
        version = "1.0.0";
        src = ./.;
        vendorSha256 = "sha256-V0DWAfnAHmpuFLn+/IIIO7qecidnvGSYTVOJ/3qAsMg=";
      };

      dockerImage = pkgs.dockerTools.buildImage {
        name = "my-app-with-rclone";
        tag = "latest";
        contents = [ myApp pkgs.rclone ];
        config = {
          Cmd = [ "${myApp}/bin/my-go-app" ];  # Adjust the path to your binary
        };
      };
    in {
      devShells.default = pkgs.mkShell {
        packages = with pkgs; [
          bashInteractive
          just
        ];
      };

      packages.my-go-app = myApp;
      packages.my-app-with-rclone = dockerImage;

    });
}
