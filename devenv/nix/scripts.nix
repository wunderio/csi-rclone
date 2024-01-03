{ pkgs }:

let
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
       nix build .#csi-rclone-container-layerd && ./result | docker load
  
       echo "Loading container image into kind"
       kind load docker-image csi-rclone:latest  --name csi-rclone-k8s
  
       echo "Render helm chart with new container version"
       helm template -n csi-rclone csi-rclone deploy/csi-rclone > devenv/kind/deploy-kind/csi-rclone-templated-chart.yaml
  
       # TODO: use tee
  
       echo "Deploy to kind cluster"
       envsubst < devenv/kind/deploy-kind/csi-rclone-templated-chart.yaml | kubectl apply -f -
  
       echo "Done"
     '';
   };
  
   reloadScript = pkgs.writeShellApplication {
     name = "reload";
  
     runtimeInputs = with pkgs; [ kubernetes-helm kubectl nix kubectl-convert ];
  
     text = ''
       echo "Building container image"
       nix build .#csi-rclone-container-layerd && ./result | docker load
  
       echo "Loading container image into kind"
       kind load docker-image csi-rclone:latest  --name csi-rclone-k8s
  
       echo "Restart Node and Controller"
       kubectl rollout restart statefulset csi-rclone-controller -n csi-rclone
       kubectl rollout restart daemonset csi-rclone-nodeplugin -n csi-rclone
  
       echo "Done"
     '';
   };
in
{
  inherit initKindCluster deleteKindCluster getKindKubeconfig localDeployScript reloadScript;
}
