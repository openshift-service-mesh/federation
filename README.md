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
4. Create a service:
```shell
kubectl apply -f <<EOF
apiVersion: v1
kind: Service
metadata:
  name: sleep
  labels:
    app: sleep
    service: sleep
    export-service: "true"
spec:
  ports:
  - port: 80
    name: http
  selector:
    app: sleep
EOF
```
5. Check listeners applied to the federation ingress gateway:
```shell
istioctl pc l deploy/istio-eastwestgateway -n istio-system
```
It should return the following output:
```
ADDRESSES PORT  MATCH                                                                                                                       DESTINATION
0.0.0.0   15021 ALL                                                                                                                         Inline Route: /healthz/ready*
0.0.0.0   15090 ALL                                                                                                                         Inline Route: /stats/prometheus*
0.0.0.0   15443 SNI: outbound_.80_._.sleep.default.svc.cluster.local; App: istio,istio-peer-exchange,istio-http/1.0,istio-http/1.1,istio-h2 Cluster: outbound_.80_._.sleep.default.svc.cluster.local
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
