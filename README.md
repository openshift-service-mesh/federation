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
kubectl apply -f - <<EOF
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

#### Tools
1. Go 1.22
2. protoc 3.19.0
3. protoc-gen-go v1.30.0
4. protoc-get-golang-deepcopy

#### Useful commands

1. Compile controller:
```shell
make
```
2. Run unit tests:
```shell
make test
```
3. Build image:
```shell
HUB=quay.io/jewertow TAG=test make docker
```
4. Run e2e tests:
```shell
make e2e
```
5. Run e2e tests with specific Istio version and custom controller image:
```shell
HUB=quay.io/jewertow TAG=test ISTIO_VERSION=1.23.0 make e2e
```
5. Re-run e2e tests without setting-up KinD clusters:
```shell
make e2e-test
```
