{ pkgs }:

let
  myApp = pkgs.buildGoModule {
    pname = "csi-rclone-pvc-1";
    version = "1.0.0";
    src = ../../.;
    vendorHash = "sha256-ZoBRdJunTcpy1OhAUxpmlIFfR3rBUPV/GN0bdT1ONMo=";
    # CGO = 0;
  };

  myAppLinux = myApp.overrideAttrs (old: old // { CGO_ENABLED = 0; GOOS = "linux"; });
  #myAppLinux = myApp.overrideAttrs (old: old // { CGO_ENABLED = 0; GOOS = "linux"; GOARCH = "arm64"; });
in
{
  inherit myApp myAppLinux;
}
