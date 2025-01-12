## Demo

### Create clusters:

#### KinD

```shell
make kind-clusters
mkdir -p east
mkdir -p west
kind get kubeconfig --name east > east/kubeconfig
kind get kubeconfig --name west > west/kubeconfig
export EAST_AUTH_PATH=$(pwd)/east
export WEST_AUTH_PATH=$(pwd)/west
```

#### OpenShift

```shell
mkdir -p east
mkdir -p west
openshift-install create cluster --dir east
openshift-install create cluster --dir west
export EAST_AUTH_PATH=$(pwd)/east/auth
export WEST_AUTH_PATH=$(pwd)/west/auth
```

And prepare aliases:
```shell
alias keast="KUBECONFIG=$EAST_AUTH_PATH/kubeconfig kubectl"
alias helm-east="KUBECONFIG=$EAST_AUTH_PATH/kubeconfig helm"
alias istioctl-east="istioctl --kubeconfig=$EAST_AUTH_PATH/kubeconfig"
alias kwest="KUBECONFIG=$WEST_AUTH_PATH/kubeconfig kubectl"
alias helm-west="KUBECONFIG=$WEST_AUTH_PATH/kubeconfig helm"
alias istioctl-west="istioctl --kubeconfig=$WEST_AUTH_PATH/kubeconfig"
```

### Trust model

Currently, workloads in federated meshes cannot establish mTLS connections if meshes use different root certificates.
In such a case, use [SPIRE](https://spiffe.io/docs/latest/spire-about/) and trust domain [federation](https://spiffe.io/docs/latest/architecture/federation/readme/).
You can follow this [guide](spire/README.md) to see how these solutions work together.

Download tools for certificate generation:
```shell
wget https://raw.githubusercontent.com/istio/istio/release-1.22/tools/certs/common.mk -O common.mk
wget https://raw.githubusercontent.com/istio/istio/release-1.22/tools/certs/Makefile.selfsigned.mk -O Makefile.selfsigned.mk
```

#### Common root and trust domain

1. Generate root and intermediate certificates for both clusters:
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
kwest create namespace istio-system
kwest create secret generic cacerts -n istio-system \
  --from-file=root-cert.pem=west/root-cert.pem \
  --from-file=ca-cert.pem=west/ca-cert.pem \
  --from-file=ca-key.pem=west/ca-key.pem \
  --from-file=cert-chain.pem=west/cert-chain.pem
```

### Deploy Istio control planes and gateways

On KinD:
```shell
istioctl-east install -f examples/kind/east-mesh.yaml -y
istioctl-west install -f examples/kind/west-mesh.yaml -y
```

On OpenShift:
```shell
keast create namespace istio-cni
keast apply -f examples/openshift/east-mesh.yaml
keast apply -f examples/openshift/east-federation-ingress-gateway.yaml
kwest create namespace istio-cni
kwest apply -f examples/openshift/west-mesh.yaml
kwest apply -f examples/openshift/west-federation-ingress-gateway.yaml
```

### Deploy federation controllers:

On KinD:
```shell
WEST_GATEWAY_IP=$(kwest get svc federation-ingress-gateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
helm-east install east chart -n istio-system \
  --values examples/kind/east-federation-controller.yaml \
  --set "federation.meshPeers.remotes[0].addresses[0]=$WEST_GATEWAY_IP"
EAST_GATEWAY_IP=$(keast get svc federation-ingress-gateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
helm-west install west chart -n istio-system \
  --values examples/kind/west-federation-controller.yaml \
  --set "federation.meshPeers.remotes[0].addresses[0]=$EAST_GATEWAY_IP"
```

On OpenShift:
```shell
WEST_CONSOLE_URL=$(kwest get routes console -n openshift-console -o jsonpath='{.spec.host}')
helm-east install east chart -n istio-system --values examples/openshift/east-federation-controller.yaml \
  --set "federation.meshPeers.remotes[0].addresses[0]=$WEST_CONSOLE_URL"
EAST_CONSOLE_URL=$(keast get routes console -n openshift-console -o jsonpath='{.spec.host}')
helm-west install west chart -n istio-system --values examples/openshift/west-federation-controller.yaml \
  --set "federation.meshPeers.remotes[0].addresses[0]=$EAST_CONSOLE_URL"
```

### Deploy and export services

1. Enable mTLS and deploy apps:
```shell
keast label namespace default istio-injection=enabled
keast apply -f https://raw.githubusercontent.com/istio/istio/release-1.22/samples/bookinfo/platform/kube/bookinfo.yaml
keast apply -f https://raw.githubusercontent.com/istio/istio/release-1.22/samples/bookinfo/networking/bookinfo-gateway.yaml
kwest label namespace default istio-injection=enabled
kwest apply -f https://raw.githubusercontent.com/istio/istio/release-1.22/samples/bookinfo/platform/kube/bookinfo.yaml
kwest apply -f https://raw.githubusercontent.com/istio/istio/release-1.22/samples/bookinfo/networking/bookinfo-gateway.yaml
keast apply -f examples/mtls.yaml
kwest apply -f examples/mtls.yaml
```

On OpenShift create also the ingress gateway and route:
```shell
keast apply -f examples/openshift/ingress-gateway.yaml
kwest apply -f examples/openshift/ingress-gateway.yaml
```

2. Delete `details` from west cluster and `ratings` from east cluster:
```shell
kwest delete svc details
kwest delete deploy details-v1
kwest delete sa bookinfo-details
keast delete svc ratings
keast delete deploy ratings-v1
keast delete sa bookinfo-ratings
```

3. Export services:
```shell
kwest label svc productpage export-service=true
kwest label svc reviews export-service=true
kwest label svc ratings export-service=true
keast label svc productpage export-service=true
keast label svc reviews export-service=true
keast label svc details export-service=true
```

4. Get gateway addresses:

On KinD:
```shell
EAST_INGRESS_ADDR=$(keast get svc istio-ingressgateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
WEST_INGRESS_ADDR=$(kwest get svc istio-ingressgateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
```

On OpenShift:
```shell
EAST_INGRESS_ADDR=$(keast get routes productpage-route -n istio-system -o jsonpath='{.spec.host}')
WEST_INGRESS_ADDR=$(kwest get routes productpage-route -n istio-system -o jsonpath='{.spec.host}')
```

5. Send a few requests to the east ingress gateway and check access log:
```shell
while true; do curl -v "http://$EAST_INGRESS_ADDR:80/productpage" > /dev/null; sleep 1; done
```
```shell
keast logs deploy/istio-ingressgateway -n istio-system --tail=3 | grep "UPSTREAM_HOST"
```
You should see internal and external IP addresses in `UPSTREAM_HOST` field:
```shell
[ 2024-09-17T18:35:29.769Z ] "GET /productpage HTTP/1.1" 200 - via_upstream - "-" 0 4294 543 542 "10.244.0.1" "curl/8.2.1" "e5d1f558-adf2-4c84-b22b-dc75b482778b" "172.18.128.2" UPSTREAM_HOST="10.244.0.17:9080" outbound|9080||productpage.default.svc.cluster.local 10.244.0.9:60828 10.244.0.9:8080 10.244.0.1:47000 - -
[ 2024-09-17T18:35:39.743Z ] "GET /productpage HTTP/1.1" 200 - via_upstream - "-" 0 5293 679 679 "10.244.0.1" "curl/8.2.1" "1eadd5f7-9c65-4485-9851-517db2f90fa1" "172.18.128.2" UPSTREAM_HOST="172.18.64.1:15443" outbound|9080||productpage.default.svc.cluster.local 10.244.0.9:42642 10.244.0.9:8080 10.244.0.1:64376 - -
[ 2024-09-17T18:35:43.594Z ] "GET /productpage HTTP/1.1" 200 - via_upstream - "-" 0 4294 28 28 "10.244.0.1" "curl/8.2.1" "1c949a71-02f5-4b63-a62b-64dece63aa9f" "172.18.128.2" UPSTREAM_HOST="10.244.0.17:9080" outbound|9080||productpage.default.svc.cluster.local 10.244.0.9:48950 10.244.0.9:8080 10.244.0.1:2917 - -
```

Do the same for west cluster:
```shell
while true; do curl -v "http://$WEST_INGRESS_ADDR:80/productpage" > /dev/null; sleep 1; done
```
```shell
kwest logs deploy/istio-ingressgateway -n istio-system --tail=3 | grep "UPSTREAM_HOST"
```

#### Cleanup

```shell
helm-east uninstall east -n istio-system
helm-west uninstall west -n istio-system
for resource in "routes" "gateways" "serviceentries" "workloadentries" "envoyfilters" "destinationrules" "peerauthentications"
do
  for ns in "istio-system" "default"
  do
    keast delete "$resource" -n "$ns" -l federation.openshift-service-mesh.io/peer=todo
    kwest delete "$resource" -n "$ns" -l federation.openshift-service-mesh.io/peer=todo
  done
done
```
