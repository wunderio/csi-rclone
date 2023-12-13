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
        inherit (goModule) myApp myAppLinux;

        dockerLayerdImage = import ./devenv/nix/containerImage.nix { inherit pkgs containerPkgs myAppLinux; };
        
        scripts = import ./devenv/nix/scripts.nix { inherit pkgs; };
        inherit (scripts) initKindCluster deleteKindCluster getKindKubeconfig localDeployScript reloadScript;

      in
      {
        devShells.default = import ./devenv/nix/shell.nix { inherit pkgs; };

        packages.csi-rclone-binary = myApp;
        packages.csi-rclone-binary-linux = myAppLinux;
        packages.csi-rclone-container-layerd = dockerLayerdImage;
        packages.deployToKind = localDeployScript;
        packages.reload = reloadScript;
        packages.initKind = initKindCluster;
        packages.deleteKind = deleteKindCluster;
        packages.getKubeconfig = getKindKubeconfig;
      });
}
