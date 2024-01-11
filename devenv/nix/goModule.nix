{ pkgs}:

let
  csiDriver = pkgs.buildGoModule {
    pname = "csi-rclone-pvc-1";
    version = "0.1.5";
    src = ../../.;
    vendorHash = "sha256-XY0XgDky2g7DQ210VsT+KKjyYL1EJPCNGP0F5GhY2gM=";
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

  csiDriverLinux = csiDriver.overrideAttrs (old: old // { CGO_ENABLED = 0; GOOS = "linux";  });
in
{
  inherit csiDriver csiDriverLinux ;
}
