## Demo

### Setup clusters

1. Create KinD clusters:
```shell
make kind-clusters
```

2. Prepare contexts:
```shell
kind get kubeconfig --name east > east.kubeconfig
alias keast="KUBECONFIG=$EAST_AUTH_PATH/kubeconfig kubectl"
alias helm-east="KUBECONFIG=$EAST_AUTH_PATH/kubeconfig helm"
kind get kubeconfig --name west > west.kubeconfig
alias kwest="KUBECONFIG=$WEST_AUTH_PATH/kubeconfig kubectl"
alias helm-west="KUBECONFIG=$WEST_AUTH_PATH/kubeconfig helm"
alias istioctl-east="istioctl --kubeconfig=$EAST_AUTH_PATH/kubeconfig"
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

### Deploy control planes and federation controllers

1. Deploy Istio control planes and gateways:
```shell
istioctl-east install -f examples/east-mesh.yaml -y
istioctl-west install -f examples/west-mesh.yaml -y
```

2. Deploy federation controller:
```shell
helm-east install east chart -n istio-system --values examples/east-federation-controller.yaml
helm-west install west chart -n istio-system --values examples/west-federation-controller.yaml
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

2. Delete `details` from west cluster and `ratings` from east cluster:
```shell
kwest delete svc details
kwest delete deploy details-v1
kwest delete sa bookinfo-details
```
```shell
keast delete svc ratings
keast delete deploy ratings-v1
keast delete sa bookinfo-ratings
```

3. Export services:
```shell
kwest label svc productpage export-service=true
kwest label svc reviews export-service=true
kwest label svc ratings export-service=true
```
```shell
keast label svc productpage export-service=true
keast label svc reviews export-service=true
keast label svc details export-service=true
```

4. Send a few requests to the east ingress gateway and check access log:
```shell
EAST_INGRESS_IP=$(keast get svc istio-ingressgateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')
curl -v "http://$EAST_INGRESS_IP:80/productpage"
```
```shell
keast logs deploy/istio-ingressgateway -n istio-system --tail=3 | grep "UPSTREAM_HOST"
```
You should see an output like this:
```shell
[ 2024-09-17T18:35:29.769Z ] "GET /productpage HTTP/1.1" 200 - via_upstream - "-" 0 4294 543 542 "10.244.0.1" "curl/8.2.1" "e5d1f558-adf2-4c84-b22b-dc75b482778b" "172.18.128.2" UPSTREAM_HOST="10.244.0.17:9080" outbound|9080||productpage.default.svc.cluster.local 10.244.0.9:60828 10.244.0.9:8080 10.244.0.1:47000 - -
[ 2024-09-17T18:35:39.743Z ] "GET /productpage HTTP/1.1" 200 - via_upstream - "-" 0 5293 679 679 "10.244.0.1" "curl/8.2.1" "1eadd5f7-9c65-4485-9851-517db2f90fa1" "172.18.128.2" UPSTREAM_HOST="172.18.64.1:15443" outbound|9080||productpage.default.svc.cluster.local 10.244.0.9:42642 10.244.0.9:8080 10.244.0.1:64376 - -
[ 2024-09-17T18:35:43.594Z ] "GET /productpage HTTP/1.1" 200 - via_upstream - "-" 0 4294 28 28 "10.244.0.1" "curl/8.2.1" "1c949a71-02f5-4b63-a62b-64dece63aa9f" "172.18.128.2" UPSTREAM_HOST="10.244.0.17:9080" outbound|9080||productpage.default.svc.cluster.local 10.244.0.9:48950 10.244.0.9:8080 10.244.0.1:2917 - -
```

Repeat for west cluster:
```shell
WEST_INGRESS_IP=$(kwest get svc istio-ingressgateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')
curl -v "http://$WEST_INGRESS_IP:80/productpage"
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
    keast delete "$resource" -n "$ns" -l federation.istio-ecosystem.io/peer=todo
    kwest delete "$resource" -n "$ns" -l federation.istio-ecosystem.io/peer=todo
  done
done
```
