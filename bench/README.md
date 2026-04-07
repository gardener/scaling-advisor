# WIP Scalebench documentation

To run the the basic snapshot test:
1. Go to the `bench` directory.
2. Build scalebench: `make build`
3. Ensure docker is running.
4. Run the setup command (generates the kwok-provider data for the scaler and builds the scaler image)
```
bin/scalebench setup cluster-autoscaler -c "./cmd/scenarios/basic-cluster-constraints.json"
```
To get the pricing data for the karpenter setup, `scadctl genprice` needs to be run.
```
bin/scalebench setup karpenter -c "./cmd/scenarios/basic-cluster-constraints.json" -p <pricing-data>
```
5. Run the scalebench with the basic snapshot. You can set `export KUBECONFIG=~/.kube/config` to target the kwok cluster for inspecting. 
```
bin/scalebench exec --snap "cmd/scenarios/test-data/basic-cluster-snapshot.json"
```

While the `exec` subcommand cleans up the kwok cluster on `C-c`, if that somehow fails then to manually stop the kwok cluster run:
```
kwokctl delete cluster --name=<cluster-name>
```

    
