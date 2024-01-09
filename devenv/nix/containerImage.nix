{ pkgs, containerPkgs, csiDriverLinux}:

pkgs.dockerTools.streamLayeredImage {
  name = "csi-rclone";
  tag = "latest";
  architecture = "amd64";

  contents = [
    csiDriverLinux

    containerPkgs.bashInteractive
    containerPkgs.cacert
    containerPkgs.coreutils
    containerPkgs.fuse3
    containerPkgs.gawk
    containerPkgs.rclone
  ];

  extraCommands = ''
    mkdir -p ./plugin
    mkdir -p ./tmp
  '';
}
