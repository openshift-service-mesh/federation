### About the project

`Federation` is a controller that utilizes MCP protocol to configure mesh-federation in Istio.

### Example deployment

1. Create a KinD cluster:
```shell
kind create cluster --name test
```
2. Deploy federation controller:
```shell
kubectl create namespace istio-system
kubectl apply -f examples/federation.yaml -n istio-system
```
3. Install Istio:
```shell
istioctl install -f examples/istio.yaml -y
```
4. Deploy an app and test provided configuration:
```shell
kubectl label namespace default istio-injection=enabled
kubectl apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/sleep/sleep.yaml
```
```shell
kubectl exec $(kubectl get pods -l app=sleep -o jsonpath='{.items[].metadata.name}') -c sleep -- curl -v https://example.com
kubectl exec $(kubectl get pods -l app=sleep -o jsonpath='{.items[].metadata.name}') -c sleep -- curl -v https://istio.io
```

### Development
1. Build:
```shell
make
```
2. Run locally:
```shell
./out/federation-controller \
  --meshPeers '{"spec":{"remote":{"addresses": ["lb-1234567890.us-east-1.elb.amazonaws.com","192.168.10.56"],"ports":{"dataPlane":15443,"discoery":15020}}}}'\
  --exportedServiceSet '{"type":"LabelSelector","labelSelectors":[{"matchLabels":{"export-service":"true"}}]}'\
  --importedServiceSet '{"type":"LabelSelector","labelSelectors":[{"matchLabels":{"export-service":"true"}}]}'
```
