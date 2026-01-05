# WIP Scalebench documentation

To run the the basic snapshot test:
1. Go to the `tools` directory.
2. Build scalebench: `make build-scalebench`
3. Ensure docker is running.
4. Run the setup command (generates the kwok-provider configmaps and builds the scalar image). If you wish to skip the scalar image build and deploy process for future runs, pass `-s=true` to the setup command.
```
bin/scalebench setup -d "./cmd/bench/cmd/scenarios/test-data/" cluster-autoscaler -o ./cmd/bench/cmd/scenarios/test-data/
```
5. Run the scalebench with the basic snapshot. You can set `export KUBECONFIG=~/.kube/config` to target the kwok cluster for inspecting. 
```
bin/scalebench exec --snap "cmd/bench/cmd/scenarios/test-data/basic-cluster-snapshot.json"
```
