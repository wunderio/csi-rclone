{ pkgs }:

let
  myApp = pkgs.buildGoModule {
    pname = "csi-rclone-pvc-1";
    version = "1.0.0-pre1";
    src = ../../.;
    vendorHash = "sha256-8izMWeLnOqTVEG9YmuUsM7JP9oNgmFLcb+JHUYPmTI4=";
    # CGO = 0;
  };

  myAppLinux = myApp.overrideAttrs (old: old // { CGO_ENABLED = 0; GOOS = "linux"; });
  #myAppLinux = myApp.overrideAttrs (old: old // { CGO_ENABLED = 0; GOOS = "linux"; GOARCH = "arm64"; });
in
{
  inherit myApp myAppLinux;
}
