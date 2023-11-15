{
  description = "A basic flake with a shell";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";
  inputs.kubenix.url = "github:hall/kubenix";

  outputs = { self, nixpkgs, flake-utils, kubenix }:
    flake-utils.lib.eachDefaultSystem (system:
    let
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

      startKindCluster = pkgs.runCommand "start-kind-cluster" {} ''
        #!${pkgs.bash}/bin/bash
        kind create cluster --name mycluster
        # You can add additional kind configuration or setup steps here
      '';

    in {
      devShells.default = pkgs.mkShell {
        packages = with pkgs; [
          bashInteractive
          just
          kind
          kubectl
          kubernetes-helm
          rclone
        ];

        shellHook = ''
          export PROJECT_ROOT="$(pwd)"
          export CLUSTER_NAME="csi-rclone-k8s"
          export KUBECONFIG="$PROJECT_ROOT/kubeconfig"
        '';
      };

      packages.my-go-app = myApp;
      packages.my-app-with-rclone = dockerImage;
      packages.startKindCluster = startKindCluster;
    });
}
