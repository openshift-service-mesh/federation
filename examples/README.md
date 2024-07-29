### Setup clusters

1. Create 2 KinD clusters:
```shell
kind create cluster --name east --config=<<EOF
apiVersion: kind.x-k8s.io/v1alpha4
kind: Cluster
networking:
  podSubnet: "10.10.0.0/16"
  serviceSubnet: "10.255.10.0/24"
EOF
```
```shell
kind create cluster --name west --config=<<EOF
apiVersion: kind.x-k8s.io/v1alpha4
kind: Cluster
networking:
  podSubnet: "10.30.0.0/16"
  serviceSubnet: "10.255.30.0/24"
EOF
```

2. Setup contexts:
```shell
kind get kubeconfig --name east > east.kubeconfig
alias keast="KUBECONFIG=$(pwd)/east.kubeconfig kubectl"
kind get kubeconfig --name west > west.kubeconfig
alias kwest="KUBECONFIG=$(pwd)/west.kubeconfig kubectl"
```

3. Install MetalLB on and configure IP address pools:
```shell
keast apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.7/config/manifests/metallb-native.yaml
kwest apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.7/config/manifests/metallb-native.yaml
```
Before creating `IPAddressPool`, define CIDR based on kind network:
```shell
docker network inspect -f '{{.IPAM.Config}}' kind
```
Define east/west CIDRs as subnets of the `kind` network, e.g. if `kind` subnet is `172.18.0.0/16`,
east network could be `172.18.64.0/18` and west could be `172.18.128.0/18`, which will not overlap with node IPs.

CIDRs must have escaped slash before the network mask to make it usable with `sed`, e.g. `172.18.64.0\/18`.
```shell
export EAST_CLUSTER_CIDR="172.18.64.0\/18"
export WEST_CLUSTER_CIDR="172.18.128.0\/18"
```
```shell
sed "s/{{.cidr}}/$EAST_CLUSTER_CIDR/g" examples/ip-address-pool.tmpl.yaml | keast apply -n metallb-system -f -
sed "s/{{.cidr}}/$WEST_CLUSTER_CIDR/g" examples/ip-address-pool.tmpl.yaml | kwest apply -n metallb-system -f -
```

### Trust model

1. Download tools for certificate generation:
```shell
wget https://raw.githubusercontent.com/istio/istio/release-1.21/tools/certs/common.mk -O common.mk
wget https://raw.githubusercontent.com/istio/istio/release-1.21/tools/certs/Makefile.selfsigned.mk -O Makefile.selfsigned.mk
```

#### Common root

1. Generate certificates for east and west clusters:
```shell
make -f Makefile.selfsigned.mk \
  ROOTCA_CN="East Root CA" \
  ROOTCA_ORG=my-company.org \
  root-ca
make -f Makefile.selfsigned.mk \
  INTERMEDIATE_CN="East Intermediate CA" \
  INTERMEDIATE_ORG=my-company.org \
  east-cacerts
make -f Makefile.selfsigned.mk \
  INTERMEDIATE_CN="West Intermediate CA" \
  INTERMEDIATE_ORG=my-company.org \
  west-cacerts
make -f common.mk clean
```

2. Create `cacert` secrets:
```shell
keast create namespace istio-system
keast create secret generic cacerts -n istio-system \
  --from-file=root-cert.pem=east/root-cert.pem \
  --from-file=ca-cert.pem=east/ca-cert.pem \
  --from-file=ca-key.pem=east/ca-key.pem \
  --from-file=cert-chain.pem=east/cert-chain.pem
```
```shell
kwest create namespace istio-system
kwest create secret generic cacerts -n istio-system \
  --from-file=root-cert.pem=west/root-cert.pem \
  --from-file=ca-cert.pem=west/ca-cert.pem \
  --from-file=ca-key.pem=west/ca-key.pem \
  --from-file=cert-chain.pem=west/cert-chain.pem
```

### Deploy federation controllers
```shell
kwest apply -f examples/exporting-controller.yaml -n istio-system
```
```shell
WEST_FEDERATION_IP=$(kwest get svc federation-controller-lb -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
cat examples/importing-controller.yaml | sed "s/{{.federationControllerIP}}/$WEST_FEDERATION_IP/" | keast apply -n istio-system -f -
```

### Install Istio 1.22.1+

```shell
istioctl --kubeconfig=west.kubeconfig install -f examples/exporting-mesh.yaml -y
istioctl --kubeconfig=east.kubeconfig install -f examples/importing-mesh.yaml -y
```

### Import and export services

1. Enable mTLS, deploy `sleep` the east cluster and `httpbin` in the west cluster and export `httpbin`:
```shell
keast apply -f examples/mtls.yaml -n istio-system
kwest apply -f examples/mtls.yaml -n istio-system
keast create namespace sleep
keast label namespace sleep istio-injection=enabled
keast apply -f https://raw.githubusercontent.com/istio/istio/release-1.22/samples/sleep/sleep.yaml -n sleep
kwest create namespace httpbin
kwest label namespace httpbin istio-injection=enabled
kwest apply -f https://raw.githubusercontent.com/istio/istio/release-1.22/samples/httpbin/httpbin.yaml -n httpbin
kwest label service httpbin -n httpbin export-service=true
```

#### Without egress gateway

2. Import httpbin from west cluster to east cluster:
```shell
REMOTE_INGRESS_IP=$(kwest get svc -l istio=eastwestgateway -n istio-system -o jsonpath='{.items[0].status.loadBalancer.ingress[0].ip}')
helm template -s templates/import-remote.yaml . \
  --set eastwestGatewayIP=$REMOTE_INGRESS_IP \
  --set eastwestEgressEnabled=false \
  | keast apply -f -
```

Check endpoints in sleep's istio-proxy:
```shell
istioctl --kubeconfig=east.kubeconfig pc endpoints deploy/sleep -n sleep | grep httpbin
```

3. Test a request from sleep to httpbin:
```shell
SLEEP_POD_NAME=$(keast get pods -l app=sleep -n sleep -o jsonpath='{.items[0].metadata.name}')
keast exec $SLEEP_POD_NAME -n sleep -c sleep -- curl -v httpbin.httpbin.svc.cluster.local:8000/headers
```

4. Now deploy httpbin locally as well and test requests again. Traffic should be routed to both instances equally.
```shell
keast create namespace httpbin
keast label namespace httpbin istio-injection=enabled
keast apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/httpbin/httpbin.yaml -n httpbin
helm template -s templates/import-remote.yaml . \
  --set eastwestGatewayIP=$REMOTE_INGRESS_IP \
  | keast delete -f -
helm template -s templates/import-as-local.yaml . \
  --set eastwestGatewayIP=$REMOTE_INGRESS_IP \
  | keast apply -f -
```

#### With egress gateway

1. Install Istio with egress gateway:
```shell
helm template -s templates/istio.yaml . \
  --set localCluster=east \
  --set remoteCluster=west \
  --set eastwestEgressEnabled=true \
  | istioctl --kubeconfig=east.kubeconfig install -y -f -
```
```shell
helm template -s templates/istio.yaml . \
  --set localCluster=west \
  --set remoteCluster=east \
  --set eastwestIngressEnabled=true \
  | istioctl --kubeconfig=west.kubeconfig install -y -f -
```

2. Import httpbin from west cluster to east cluster:
```shell
REMOTE_INGRESS_IP=$(kwest get svc -l istio=eastwestgateway -n istio-system -o jsonpath='{.items[0].status.loadBalancer.ingress[0].ip}')
LOCAL_EGRESS_IP=$(keast get svc -l istio=eastwestgateway-egress -n istio-system -o jsonpath='{.items[0].spec.clusterIP}')
helm template -s templates/import-remote.yaml . \
  --set eastwestGatewayIP=$REMOTE_INGRESS_IP \
  --set egressGatewayIP=$LOCAL_EGRESS_IP \
  --set eastwestEgressEnabled=true \
  | keast apply -f -
```

Check endpoints in sleep's istio-proxy:
```shell
istioctl --kubeconfig=east.kubeconfig pc endpoints deploy/sleep -n sleep | grep httpbin
```

3. Test a request from sleep to httpbin:
```shell
SLEEP_POD_NAME=$(keast get pods -l app=sleep -n sleep -o jsonpath='{.items[0].metadata.name}')
keast exec $SLEEP_POD_NAME -n sleep -c sleep -- curl -v httpbin.httpbin.svc.cluster.local:8000/headers
```

4. Now deploy httpbin locally as well and test requests again. Traffic should be routed to both instances equally.
```shell
keast create namespace httpbin
keast label namespace httpbin istio-injection=enabled
keast apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/httpbin/httpbin.yaml -n httpbin
helm template -s templates/import-remote.yaml . \
  --set eastwestGatewayIP=$REMOTE_INGRESS_IP \
  --set egressGatewayIP=$LOCAL_EGRESS_IP \
  --set eastwestEgressEnabled=true \
  | keast delete -f -
helm template -s templates/import-as-local.yaml . \
  --set eastwestGatewayIP=$REMOTE_INGRESS_IP \
  --set egressGatewayIP=$LOCAL_EGRESS_IP \
  --set eastwestEgressEnabled=true \
  | keast apply -f -
```

#### Verify load balancing

1. Scale local deployment of httpbin:
```shell
keast scale deployment httpbin -n httpbin --replicas 2
```
Run a few requests and look at logs of httpbin:
```shell
while true
do
  keast exec $SLEEP_POD_NAME -n sleep -c sleep -- curl -v httpbin.httpbin.svc.cluster.local:8000/headers
  sleep 2
done
```
Sidecar of sleep app should have 3 endpoints and requests should be routed equally to all instances.

2. Scale remote deployment of httpbin:
```shell
kwest scale deployment httpbin -n httpbin --replicas 3
```
Run requests and look at logs again.

You should see that the new instances of httpbin in the west cluster do not receive traffic.

3. Scale sleep deployment:
```shell
keast scale deployment sleep -n sleep --replicas 10
```
```shell
cat <<EOF >> test-lb-for-different-clients.sh
#!/bin/bash
pod_names=$(KUBECONFIG=east.kubeconfig kubectl get pods -l app=sleep -n sleep -o jsonpath='{.items[*].metadata.name}')
for pod in $pod_names; do
  KUBECONFIG=east.kubeconfig kubectl exec $pod -n sleep -c sleep -- curl -v httpbin.httpbin.svc.cluster.local:8000/headers
done
EOF
bash test-lb-for-different-clients.sh
```
Now requests should be routed to different instances.

It's a known issue and as a workaround clients can always establish new TCP connection
or configure maxRequestsPerConnection to enforce establishing more connections.
* https://github.com/envoyproxy/envoy/issues/15071
* https://discuss.istio.io/t/need-help-understanding-load-balancing-between-clusters/9552

TODO: add examples with DestinationRule applied to east-west gateway with `maxRequestsPerConnection`.

#### Authorization

1. Deploy sleep in west cluster:
```shell
kwest create namespace sleep
kwest label namespace sleep istio-injection=enabled
kwest apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/sleep/sleep.yaml -n sleep
```

2. Send a test request from sleep to httpbin locally:
```shell
SLEEP_POD_NAME=$(kwest get pods -l app=sleep -n sleep -o jsonpath='{.items[0].metadata.name}')
kwest exec $SLEEP_POD_NAME -n sleep -c sleep -- curl -v httpbin.httpbin.svc.cluster.local:8000/headers
```

3. Deny access from sleep to west httpbin:
```shell
kwest apply -n istio-system -f - <<EOF
apiVersion: security.istio.io/v1beta1
kind: AuthorizationPolicy
metadata:
  name: deny-east-sleep
spec:
  selector:
    matchLabels:
      app: httpbin
  action: DENY
  rules:
  - from:
    - source:
        principals: ["east.local/ns/sleep/sa/sleep"]
EOF
```

3. Send a test request from the east cluster and then from the west cluster:
```shell
SLEEP_POD_NAME=$(kwest get pods -l app=sleep -n sleep -o jsonpath='{.items[0].metadata.name}')
kwest exec $SLEEP_POD_NAME -n sleep -c sleep -- curl -v httpbin.httpbin.svc.cluster.local:8000/headers
```
```shell
SLEEP_POD_NAME=$(keast get pods -l app=sleep -n sleep -o jsonpath='{.items[0].metadata.name}')
keast exec $SLEEP_POD_NAME -n sleep -c sleep -- curl -v httpbin.httpbin.svc.cluster.local:8000/headers
```
Both requests should fail with 403 when requests are routed to the west cluster.
```
* IPv6: (none)
* IPv4: 10.96.100.107
*   Trying 10.96.100.107:8000...
* Connected to httpbin.httpbin.svc.cluster.local (10.96.100.107) port 8000
> GET /headers HTTP/1.1
> Host: httpbin.httpbin.svc.cluster.local:8000
> User-Agent: curl/8.7.1
> Accept: */*
>
* Request completely sent off
< HTTP/1.1 403 Forbidden
< content-length: 19
< content-type: text/plain
< date: Wed, 24 Apr 2024 21:33:44 GMT
< server: envoy
< x-envoy-upstream-service-time: 1
<
{ [19 bytes data]
100    19  100    19    0     0   3882      0 --:--:-- --:--:-- --:--:--  4750
* Connection #0 to host httpbin.httpbin.svc.cluster.local left intact
RBAC: access denied%
```

### Notes / TODOs
