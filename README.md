
# CSI rclone mount plugin

This project implements Container Storage Interface (CSI) plugin that allows using [rclone mount](https://rclone.org/) as storage backend. Rclone mount points and [parameters](https://rclone.org/commands/rclone_mount/) can be configured using Secret or PersistentVolume volumeAttibutes. 


## Installing CSI driver to kubernetes cluster

## Changelog

See [CHANGELOG.txt](CHANGELOG.txt)

## Dev Environment
This repo uses `nix` for the dev environment. Alternatively, run `nix develop` to enter a dev shell.

Ensure that `nix`, `direnv` and `nix-direnv` are installed.
Also add the following to your nix.conf:
```
experimental-features = nix-command flakes
```
then commands can be run like e.g. `nix run '.#initKind'`. Check `flakes.nix` 
for all available commands.

To deploy the test cluster and run tests, run 
```bash
$ nix run '.#initKind'
$ nix run '.#getKubeconfig'
$ nix run '.#deployToKind'
$ go test -v ./...
```
in your shell, or if you're in a nix shell, run
```bash
$ init-kind-cluster
$ get-kind-kubeconfig
$ local-deploy
$ go test -v ./...
```


