## Design

`scalebench` leverages [e2e-framework](https://github.com/kubernetes-sigs/e2e-framework/tree/main) to construct the environment needed for running the benchmarking harness, this means running the control plane components:
1. kube-scheduler
2. kube-apiserver
3. etcd

In addition to the above, the scaler for which benchmarking needs to be done is also deployed alongwith the kwok-controller for managing the fake nodes. It uses kwokctl's "docker" runtime which leads to each component getting their own container making monitoring of the components extremely easy.

All the workload is deployed by the `exec` command after all the required components are up and then the deployed scaler can trigger node scaling depending on the pending, unscheduled pods. The required information is captured as part of `ClusterSnapshot`.

The templates for the new nodes that are spun up is provided as `kwok` provider specific data for the respective scaler.

For cluster autoscaler, this includes two configmaps:
1. `kwok-provider-config`: which just specifies how to get the nodegroup data for the kwok cloudprovider implementation.
2. `kwok-provider-templates`: consists of node templates used to create new nodes.
To learn more about the provider, check the [upstream documentation](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler/cloudprovider/kwok).

For karpenter, it needs:
1. `instance_types.json`: a master list consisting of all available offerings which can be used by karpenter as candidates when constructing nodes ([example](https://github.com/kubernetes-sigs/karpenter/blob/main/kwok/examples/instance_types.json))
2. `NodePool`s: this is used to set constraints on the nodes that can be created by and the pods that can run on those nodes. ([upstream documentation](https://karpenter.sh/docs/concepts/nodepools/))
3. `KWOKNodeClass`es: these are just dummy NodeClasses needed by the kwok provider implementation (for actual cloudproviders, these contain provider specific settings)
To learn more, check the [upstream documentation](https://github.com/kubernetes-sigs/karpenter/tree/main/kwok).

All this required data is generated using the `ScalingConstraints` file which is passed to the `setup` subcommand alongwith the scaler and the required version of the scaler used to build the docker image.

### Alternatives considered

During the development phase, alternative approaches for constructing the environment needed to run the harness were considered, these included:
1. `envtest`: this tool is used by [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime/tree/main/tools/setup-envtest) for fetching the control plane binaries, however this doesn't include kube-scheduler and doesn't support docker; only allows for running the components as local processes thereby making management and cleanup a chore.
2. `kind` runtime: this runtime provided by "kwokctl" allows one to leverage kind in order to deploy control plane components on a `Node`, however monitoring of individual components is then not as easy as it is in the case when the components are in isolated, standalone docker containers. Also the distinction between the kind control plane node and the worker nodes require tweaking the affinities/selectors of the workload to ensure that its not deployed on non-kwok nodes.

The advantages of leveraging `e2e-framework` and `kwokctl` lies in the fact that it gives one a golang-native way of managing the entire harness lifecycle rather than relying on bash scripts which don't do proper error handling. It also allows for automated log collection rather than relying on a different service or managing it yourself.

## Usage

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
bin/scalebench exec --snap "cmd/scenarios/basic-cluster-snapshot.json"
```

While the `exec` subcommand cleans up the kwok cluster on `C-c`, if that somehow fails then to manually stop the kwok cluster run:
```
kwokctl delete cluster --name=<cluster-name>
# or to remove all clusters
kwokctl delete cluster --all
```
