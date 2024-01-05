{ pkgs }:
let
  optionalPkgs = with pkgs; if stdenv.isDarwin then [
    macfuse-stubs
  ] else [];
  scripts = import ./scripts.nix { inherit pkgs; };
  inherit (scripts) initKindCluster deleteKindCluster getKindKubeconfig localDeployScript reloadScript;
in
pkgs.mkShell {
  packages = with pkgs; [
    # DevTools
    bashInteractive
    envsubst # substitute environment variables (used for secrets)
    kind # K8s in docker
    pre-commit # Git pre-commit hooks
    sops
    yazi # Filemanager

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

    # Nix
    nil # LSP
    nixfmt # Formatter
  ] ++ optionalPkgs ++ [initKindCluster deleteKindCluster getKindKubeconfig localDeployScript reloadScript];

  shellHook = ''
    export PROJECT_ROOT="$(pwd)"
    export CLUSTER_NAME="csi-rclone-k8s"
    export KUBECONFIG="$PROJECT_ROOT/devenv/kind/kubeconfig"
    export RCLONE_CONFIG=$PROJECT_ROOT/devenv/local-s3/switch-engine-ceph-rclone-config.conf
    
    # Load secrets as ENVs
    eval "$("$direnv" dotenv bash <(sops -d .env))"
  '';
}
