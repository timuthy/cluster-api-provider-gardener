# KCP Cluster API Syncer For Gardener `Shoot`s

In the first terminal pane, run

```shell
pushd $GOPATH/src/github.com/gardener/gardener > /dev/null
make kind-up gardener-up
popd > /dev/null

kcp start --bind-address=127.0.0.1 --external-hostname=127.0.0.1
```

Now you have a local kind cluster with a Gardener installation, and you run the KCP control plane locally (as process) in this terminal pane.

In another terminal pane, run

```shell
export KUBECONFIG=.kcp/kubeconfig

kubectl apply -f config/apiresourceschema-clusters.yaml
kubectl apply -f config/apiexport.yaml
kubectl apply -f config/apibinding.yaml

kubectl ws use root
kubectl apply -f config/apibinding.yaml
```
