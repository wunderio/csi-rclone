{
  description = "A basic flake with a shell";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";
  inputs.nix-pre-commit.url = "github:jmgilman/nix-pre-commit";

  outputs = { self, nixpkgs, flake-utils, nix-pre-commit }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        myApp = (pkgs.buildGoModule {
          pname = "csi-rclone";
          version = "1.0.0";
          src = ./.;
          vendorSha256 = "sha256-V0DWAfnAHmpuFLn+/IIIO7qecidnvGSYTVOJ/3qAsMg=";
          CGO = 0;
        }).overrideAttrs (old: old // { CGO_ENABLED = 0; GOOS = "linux"; GOARCH = "arm64"; });

        dockerImage = pkgs.dockerTools.buildImage {
          name = "csi-rclone";
          tag = "latest";
          config = {
            Cmd = [ "/bin/linux_arm64/csi-rclone-plugin" ]; # Adjust the path to your binary
            #Env = { PATH = /bin; };
          };

          runAsRoot = ''
            #!${pkgs.runtimeShell}
            mkdir -p /tmp
          '';

          copyToRoot = pkgs.buildEnv {
            name = "image-root";
            paths = [ myApp pkgs.rclone ];
            pathsToLink = [ "/bin" "/bin/linux_arm64" ];
          };
        };

        startKindCluster = pkgs.runCommand "start-kind-cluster" { } ''
          #!${pkgs.bash}/bin/bash
          kind create cluster --name mycluster
          # You can add additional kind configuration or setup steps here
        '';

        localDeployScript = pkgs.writeShellApplication {
          name = "local-deploy";

          runtimeInputs = with pkgs; [ cowsay kubernetes-helm kubectl nix kubectl-convert ];

          text = ''
            echo "Building container image"
            nix build --builders "ssh://eu.nixbuild.net aarch64-linux - 100 1 big-parallel,benchmark" .#csi-rclone-container

            echo "Loading container image into kind"
            docker load < result
            kind load docker-image csi-rclone:latest  --name csi-rclone-k8s

            echo "Installing chart"
            #helm install csi-rclone deploy/helm_chart --namespace csi-rclone --create-namespace 

            echo "Render helm chart with new container version"
            helm template csi-rclone deploy/helm_chart > devenv/kind/deploy-kind/csi-rclone-templated-chart.yaml

            echo "Deploy to kind cluster"
            kubectl apply -f devenv/kind/deploy-kind

            echo "Done"
          '';
        };

        config = {
          repos = [
            {
              repo = "local";
              hooks = [
                {
                  id = "nixpkgs-fmt";
                  entry = "${pkgs.nixpkgs-fmt}/bin/nixpkgs-fmt";
                  language = "system";
                  files = "\\.nix";
                }
                #{
                #  id = "sops-encryption";
                #  entry = "${pkgs.pre-commit-hook-ensure-sops}/bin/pre-commit-hook-ensure-sops";
                #  language = "system";
                #}
              ];
            }
          ];
        };

      in
      {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            bashInteractive
            macfuse-stubs
            just
            kind
            kubectl
            kubernetes-helm
            pre-commit
            pre-commit-hook-ensure-sops
            rclone
            yazi
          ];

          shellHook = ''
            export PROJECT_ROOT="$(pwd)"
            export CLUSTER_NAME="csi-rclone-k8s"
            export KUBECONFIG="$PROJECT_ROOT/devenv/kind/kubeconfig"
            export RCLONE_CONFIG=$PROJECT_ROOT/devenv/local-s3/switch-engine-ceph-rclone-config.conf

            ${((nix-pre-commit.lib.${system}.mkConfig {
              inherit pkgs config;
            }).shellHook)}
          '';
        };

        packages.csi-rclone-binary = myApp;
        packages.csi-rclone-container = dockerImage;
        packages.startKindCluster = startKindCluster;
        packages.deployToKind = localDeployScript;



      });


}
