{
  description = "A basic flake with a shell";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";
  inputs.nix-pre-commit.url = "github:jmgilman/nix-pre-commit";

  outputs = { self, nixpkgs, flake-utils, nix-pre-commit }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        containerPkgs = import nixpkgs { localSystem = system; crossSystem = "x86_64-linux"; };

        myApp = pkgs.buildGoModule {
          pname = "csi-rclone-pvc-1";
          version = "1.0.0";
          src = ./.;
          vendorSha256 = "sha256-ZoBRdJunTcpy1OhAUxpmlIFfR3rBUPV/GN0bdT1ONMo=";
          CGO = 0;
        };

        #myAppLinux = myApp.overrideAttrs (old: old // { CGO_ENABLED = 0; GOOS = "linux"; });
        #myAppLinux = myApp.overrideAttrs (old: old // { CGO_ENABLED = 0; GOOS = "linux"; GOARCH = "arm64"; });
        dockerLayerdImage = pkgs.dockerTools.streamLayeredImage {
          name = "csi-rclone";
          tag = "latest";
          architecture = "amd64";

          contents = [
            myApp

            containerPkgs.bashInteractive
            containerPkgs.cacert
            containerPkgs.coreutils
            containerPkgs.fuse3
            containerPkgs.gawk
            containerPkgs.rclone
          ];

          fakeRootCommands = ''
            sudo mkdir -p /tmp
            sudo mkdir -p /plugin
          '';
        };

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
            mkdir -p /plugin
          '';

          copyToRoot = pkgs.buildEnv {
            name = "image-root";

            paths = [
              myApp

              pkgs.bashInteractive
              pkgs.cacert
              pkgs.coreutils
              pkgs.fuse3
              pkgs.gawk
              pkgs.rclone
            ];

            pathsToLink = [ "/bin" "/bin/linux_arm64" ];
          };
        };

        startKindCluster = pkgs.runCommand "start-kind-cluster" { } ''
          #!${pkgs.bash}/bin/bash
          kind create cluster --name mycluster
          # You can add additional kind configuration or setup steps here
        '';

        initKindCluster = pkgs.writeShellApplication {
          name = "init-kind-cluster";

          runtimeInputs = with pkgs; [ kubectl kind ];

          text = ''
            echo "Init Kind cluster"
            kind create cluster --name "$CLUSTER_NAME"
          '';
        };

        deleteKindCluster = pkgs.writeShellApplication {
          name = "delete-kind-cluster";

          runtimeInputs = with pkgs; [ kubectl kind ];

          text = ''
            echo "Delete Kind cluster"
            kind delete cluster --name "$CLUSTER_NAME"
          '';
        };

        getKindKubeconfig = pkgs.writeShellApplication {
          name = "get-kind-kubeconfig";

          runtimeInputs = with pkgs; [ kubectl kind ];

          text = ''
            echo "Get kubeconfig"
            kind get kubeconfig --name "$CLUSTER_NAME" > "$PROJECT_ROOT"/devenv/kind/kubeconfig
          '';
        };

        localDeployScript = pkgs.writeShellApplication {
          name = "local-deploy";

          runtimeInputs = with pkgs; [ kubernetes-helm kubectl nix kubectl-convert ];

          text = ''
            echo "Building container image"
            nix build .#packages.x86_64-linux.csi-rclone-container

            echo "Loading container image into kind"
            docker load < result
            kind load docker-image csi-rclone:latest  --name csi-rclone-k8s

            echo "Render helm chart with new container version"
            helm template csi-rclone deploy/chart > devenv/kind/deploy-kind/csi-rclone-templated-chart.yaml

            echo "Deploy to kind cluster"
            kubectl apply -f devenv/kind/deploy-kind

            echo "Done"
          '';
        };

        reloadScript = pkgs.writeShellApplication {
          name = "reload";

          runtimeInputs = with pkgs; [ kubernetes-helm kubectl nix kubectl-convert ];

          text = ''
            echo "Building container image"
            nix build .#csi-rclone-container-layerd | docker load

            echo "Loading container image into kind"
            kind load docker-image csi-rclone:latest  --name csi-rclone-k8s

            echo "Restart Node and Controller"
            kubectl rollout restart statefulset csi-rclone-controller -n default
            kubectl rollout restart daemonset csi-rclone-nodeplugin -n default

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
            # DevTools
            bashInteractive
            kind # K8s in docker
            pre-commit # Git pre-commit hooks
            yazi # Filemanager
            vals # configuration values loader 

            # Go
            go_1_20 # Go v1.20
            golangci-lint # Linter
            gopls # LSP
            gotools # Additional Tooling
            # VSCode GO: https://mgdm.net/weblog/vscode-nix-go-tools/
            go-outline
            gocode
            gopkgs
            gocode-gomod
            godef
            golint

            # Kubernetes
            k9s
            kubectl
            kubernetes-helm # Helm

            # Rclone
            rclone
            macfuse-stubs # Fuse on MacOS

            # Nix
            nil # LSP
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
        #packages.csi-rclone-binary-linux = myApp.overrideAttrs (old: old // { CGO_ENABLED = 0; GOOS = "linux"; GOARCH = "arm64"; });
        packages.csi-rclone-container = dockerImage;
        packages.csi-rclone-container-layerd = dockerLayerdImage;
        packages.startKindCluster = startKindCluster;
        packages.deployToKind = localDeployScript;
        packages.reload = reloadScript;
        packages.initKind = initKindCluster;
        packages.deleteKind = deleteKindCluster;
        packages.getKubeconfig = getKindKubeconfig;

      });


}
