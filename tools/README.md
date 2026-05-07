# scadctl

To build the binary: run `make build`, this generates `bin/scadctl`.

To fetch a gardener shoot cluster data and create snapshot variants:
```sh
./bin/scadctl genscenario gardener -l <landscape> -p <project> -s <shoot> "./gen"
```
The above saves the data in `./gen` directory, if nothing is specified, `/tmp/` is the fallback directory.

NOTE: This needs the gardener `oidc kubeconfigs` to be present in your system in order to create the required `viewerkubeconfig`s.

Other than the above specified required flags, there are two optional flags available as well
```sh
--exclude-system-components # (false) to remove system pods and priority classes from snapshot
--obfuscate-data # (false) to sanitize and obfuscate names, owners and specific selectors
```
