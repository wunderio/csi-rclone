{ pkgs}:

let
  myApp = pkgs.buildGoModule {
    pname = "csi-rclone-pvc-1";
    version = "1.0.0-pre3";
    src = ../../.;
    vendorHash = "sha256-F0vlfdOgCVlw6WYYvx3kOXYmT6pFjtJO2zu0GC/yrw4=";
    # CGO = 0;
    # preBuild = ''
    #   whoami
    #   mkdir -p $TMP/conf
    #   kind get kubeconfig --name csi-rclone-k8s > $TMP/conf/kubeconfig
    #   export KUBECONFIG=$TMP/conf/kubeconfig
    # '';
    # nativeBuildInputs = with pkgs; [ kubectl kind docker ];
    doCheck = false; # tests need docker and kind, which nixbld user might not have access to
  };

  myAppLinux = myApp.overrideAttrs (old: old // { CGO_ENABLED = 0; GOOS = "linux";  });
  #myAppLinux = myApp.overrideAttrs (old: old // { CGO_ENABLED = 0; GOOS = "linux"; GOARCH = "arm64"; });
in
{
  inherit myApp myAppLinux;
}
