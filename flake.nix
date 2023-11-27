{
  description = "A basic flake with a shell";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";

  outputs = { self, nixpkgs, flake-utils }:
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

      localDeployScript = pkgs.writeShellApplication {
        name = "local-deploy";

        runtimeInputs = with pkgs; [ cowsay kubernetes-helm kubectl nix kubectl-convert ];

        text = ''
          echo "Building container image"
          nix build .#my-app-with-rclone

          echo "Loading container image into kind"
          docker load < result
          kind load docker-image my-app-with-rclone:latest  --name csi-rclone-k8s

          echo "Render helm chart with new container version"
          helm template csi-rclone deploy/helm_chart

          echo "Deploy to kind cluster"
          kubectl apply -f devenv/deploy-kind

          echo "Done"
        '';
      };

    in {
      devShells.default = pkgs.mkShell {
        packages = with pkgs; [
          bashInteractive
          macfuse-stubs
          just
          kind
          kubectl
          kubectl-convert
          kubernetes-helm
          rclone
          yazi
          nushell
        ];

        shellHook = ''
          export PROJECT_ROOT="$(pwd)"
          export CLUSTER_NAME="csi-rclone-k8s"
          export KUBECONFIG="$PROJECT_ROOT/devenv/kind/kubeconfig"
          export RCLONE_CONFIG=$PROJECT_ROOT/devenv/local-s3/switch-engine-ceph-rclone-config.conf
        '';
      };

      packages.my-go-app = myApp;
      packages.my-app-with-rclone = dockerImage;
      packages.startKindCluster = startKindCluster;
      packages.localDeployScript = localDeployScript;
    });


}
