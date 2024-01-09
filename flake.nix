{
  description = "A basic flake with a shell";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";

  outputs = { self, nixpkgs, flake-utils}:
    flake-utils.lib.eachDefaultSystem (system:
      let
        # Import pkgs (for your platform) and containerPkgs (x86_64-linux) for crosscompile on MacOS
        pkgs = nixpkgs.legacyPackages.${system};
        containerPkgs = import nixpkgs { localSystem = system; crossSystem = "x86_64-linux"; };

        goModule = import ./devenv/nix/goModule.nix { inherit pkgs; };
        inherit (goModule) csiDriver csiDriverLinux;

        dockerLayerdImage = import ./devenv/nix/containerImage.nix { inherit pkgs containerPkgs csiDriverLinux; };
        
        scripts = import ./devenv/nix/scripts.nix { inherit pkgs; };
        inherit (scripts) initKindCluster deleteKindCluster getKindKubeconfig localDeployScript reloadScript;

      in
      {
        devShells.default = import ./devenv/nix/shell.nix { inherit pkgs; };

        packages.csi-rclone-binary = csiDriver;
        packages.csi-rclone-binary-linux = csiDriverLinux;
        packages.csi-rclone-container-layerd = dockerLayerdImage;
        packages.deployToKind = localDeployScript;
        packages.reload = reloadScript;
        packages.initKind = initKindCluster;
        packages.deleteKind = deleteKindCluster;
        packages.getKubeconfig = getKindKubeconfig;
      });
}
