### Integration with SPIRE and trust domain federation

#### Prerequisites

1. Download charts:
```shell
helm repo add spiffe-hardened https://spiffe.github.io/helm-charts-hardened
```

2. Install SPIRE:
```shell
# CRDs
helm-east upgrade --install spire-crds spiffe-hardened/spire-crds --version 0.5.0
helm-west upgrade --install spire-crds spiffe-hardened/spire-crds --version 0.5.0
# CSI driver, server, agent and OIDC provider
helm-east upgrade --install spire spiffe-hardened/spire -n spire --create-namespace --values examples/spire/east/values.yaml --version 0.24.0 --wait
helm-west upgrade --install spire spiffe-hardened/spire -n spire --create-namespace --values examples/spire/west/values.yaml --version 0.24.0 --wait
```

3. Federate bundles:
```shell
# east
spire_bundle_endpoint_west=$(kwest get svc spire-server -n spire -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
west_bundle=$(kwest exec -c spire-server -n spire --stdin spire-server-0  -- spire-server bundle show -format spiffe)
indented_west_bundle=$(echo "$west_bundle" | jq -r '.' | sed 's/^/    /')
(cat examples/spire/trust-bundle-federation.yaml; echo -e "  trustDomainBundle: |-\n$indented_west_bundle") | sed "s/\${CLUSTER}/west/g" | sed "s/\${BUNDLE_ENDPOINT}/$spire_bundle_endpoint_west/g" | keast apply -f -
# west
spire_bundle_endpoint_east=$(keast get svc spire-server -n spire -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
east_bundle=$(keast exec -c spire-server -n spire --stdin spire-server-0  -- spire-server bundle show -format spiffe)
indented_east_bundle=$(echo "$east_bundle" | jq -r '.' | sed 's/^/    /')
(cat examples/spire/trust-bundle-federation.yaml; echo -e "  trustDomainBundle: |-\n$indented_east_bundle") | sed "s/\${CLUSTER}/east/g" | sed "s/\${BUNDLE_ENDPOINT}/$spire_bundle_endpoint_east/g" | kwest apply -f -
```
List bundles:
```shell
keast exec -it -n spire spire-server-0 -c spire-server -- spire-server bundle list
kwest exec -it -n spire spire-server-0 -c spire-server -- spire-server bundle list
```
You should see an output like below:
```
****************************************
* west.local
****************************************
-----BEGIN CERTIFICATE-----
MIIDxDCCAqygAwIBAgIQC+oF3uz+USh9/2fAJxZWCzANBgkqhkiG9w0BAQsFADBs
...
-----END CERTIFICATE-----
****************************************
* east.local
****************************************
-----BEGIN CERTIFICATE-----
MIIDxzCCAq+gAwIBAgIRAOSC+9AxMNaNqWdzd3QfbucwDQYJKoZIhvcNAQELBQAw
...
-----END CERTIFICATE-----
```

4. Deploy Istio:
```shell
sed -e "s/\${LOCAL_CLUSTER}/east/g" \
  -e "s/\${REMOTE_CLUSTER}/west/g" \
  -e "s/\${REMOTE_BUNDLE_ENDPOINT}/$spire_bundle_endpoint_west/g" \
  examples/spire/istio.yaml | istioctl-east install -y -f -
sed -e "s/\${LOCAL_CLUSTER}/west/g" \
  -e "s/\${REMOTE_CLUSTER}/east/g" \
  -e "s/\${REMOTE_BUNDLE_ENDPOINT}/$spire_bundle_endpoint_east/g" \
  examples/spire/istio.yaml | istioctl-west install -y -f -
```
Verify Spire's registry:
```shell
keast exec -t spire-server-0 -n spire -c spire-server -- spire-server entry show
kwest exec -t spire-server-0 -n spire -c spire-server -- spire-server entry show
```
You should see an output like below:
```
Found 4 entries
Entry ID         : east.7ee41587-cb65-474f-a944-4fe09c72a5e8
SPIFFE ID        : spiffe://east.local/ns/istio-system/sa/federation-ingress-gateway-service-account
Parent ID        : spiffe://east.local/spire/agent/k8s_psat/east/8817df8c-1518-4587-b940-9502a9791b5c
Revision         : 0
X509-SVID TTL    : default
JWT-SVID TTL     : default
Selector         : k8s:pod-uid:125f41c6-282f-48e0-9f37-8a0c238bd6f5
FederatesWith    : west.local
Hint             : istio-system
...
```

5. Deploy federation controllers:
```shell
WEST_GATEWAY_IP=$(kwest get svc federation-ingress-gateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
helm-east install east-mesh chart -n istio-system \
    --values examples/kind/east-federation-controller.yaml \
    --set "istio.spire.enabled=true" \
    --set "federation.meshPeers.remotes[0].addresses[0]=$WEST_GATEWAY_IP"
EAST_GATEWAY_IP=$(keast get svc federation-ingress-gateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
helm-west install west-mesh chart -n istio-system \
    --values examples/kind/west-federation-controller.yaml \
    --set "istio.spire.enabled=true" \
    --set "federation.meshPeers.remotes[0].addresses[0]=$EAST_GATEWAY_IP"
```

6. Deploy and export apps:
```shell
# east
keast label namespace default istio-injection=enabled
keast apply -f examples/spire/sleep.yaml
keast apply -f examples/mtls.yaml -n istio-system
# west
kwest label namespace default istio-injection=enabled
kwest apply -f examples/spire/sleep.yaml
kwest apply -f examples/spire/httpbin.yaml
kwest apply -f examples/mtls.yaml -n istio-system
kwest label service httpbin export-service=true
```

7. Verify connectivity with the imported service:
```shell
keast exec deploy/sleep -c sleep -- curl -v httpbin.default.svc.cluster.local:8000/headers
```
Expected response:
```
Host httpbin.default.svc.cluster.local:8000 was resolved.
* IPv6: (none)
* IPv4: 240.240.0.2
*   Trying 240.240.0.2:8000...
* Connected to httpbin.default.svc.cluster.local (240.240.0.2) port 8000
* using HTTP/1.x
> GET /headers HTTP/1.1
> Host: httpbin.default.svc.cluster.local:8000
> User-Agent: curl/8.10.1
> Accept: */*
> 
* Request completely sent off
< HTTP/1.1 200 OK
< access-control-allow-credentials: true
< access-control-allow-origin: *
< content-type: application/json; charset=utf-8
< date: Thu, 24 Oct 2024 19:52:12 GMT
< content-length: 627
< x-envoy-upstream-service-time: 0
< server: envoy
< 
{ [561 bytes data]
* Connection #0 to host httpbin.default.svc.cluster.local left intact
{
  "headers": {
    "Accept": [
      "*/*"
    ],
    "Host": [
      "httpbin.default.svc.cluster.local:8000"
    ],
    "User-Agent": [
      "curl/8.10.1"
    ],
    "X-Envoy-Attempt-Count": [
      "1"
    ],
    "X-Forwarded-Client-Cert": [
      "By=spiffe://west.local/ns/default/sa/httpbin;Hash=20d7bd38024492e9018d3427f60e3515e80c252122ee88afb40127ab8e6774ed;Subject=\"\";URI=spiffe://east.local/ns/default/sa/sleep"
    ],
    "X-Forwarded-Proto": [
      "http"
    ],
    "X-Request-Id": [
      "f90e877c-97ce-4e1d-8b3c-8bc4b3d10988"
    ]
  }
}
```

8. Configure authorization policy for httpbin that allows requests only from sleep in west.local trust domain:
```shell
kwest apply -f examples/spire/authz-policy.yaml
```

9. Send a test request from sleep in the east cluster:
```shell
keast exec deploy/sleep -c sleep -- curl -v httpbin.default.svc.cluster.local:8000/headers
```
Now it should return 403:
```
Host httpbin.default.svc.cluster.local:8000 was resolved.
* IPv6: (none)
* IPv4: 240.240.0.2
*   Trying 240.240.0.2:8000...
* Connected to httpbin.default.svc.cluster.local (240.240.0.2) port 8000
* using HTTP/1.x
> GET /headers HTTP/1.1
> Host: httpbin.default.svc.cluster.local:8000
> User-Agent: curl/8.12.1
> Accept: */*
> 
* Request completely sent off
< HTTP/1.1 403 Forbidden
< content-length: 19
< content-type: text/plain
< date: Mon, 17 Mar 2025 14:05:58 GMT
< server: envoy
< x-envoy-upstream-service-time: 8
< 
{ [19 bytes data]
* Connection #0 to host httpbin.default.svc.cluster.local left intact
```

10. Send a test request from sleep in the west cluster:
```shell
kwest exec deploy/sleep -c sleep -- curl -v httpbin.default.svc.cluster.local:8000/headers
```
It should succeed:
```
Host httpbin.default.svc.cluster.local:8000 was resolved.
* IPv6: (none)
* IPv4: 10.96.100.85
*   Trying 10.96.100.85:8000...
* Connected to httpbin.default.svc.cluster.local (10.96.100.85) port 8000
* using HTTP/1.x
> GET /headers HTTP/1.1
> Host: httpbin.default.svc.cluster.local:8000
> User-Agent: curl/8.12.1
> Accept: */*
> 
* Request completely sent off
< HTTP/1.1 200 OK
< access-control-allow-credentials: true
< access-control-allow-origin: *
< content-type: application/json; charset=utf-8
< date: Mon, 17 Mar 2025 14:07:01 GMT
< content-length: 561
< x-envoy-upstream-service-time: 7
< server: envoy
{
  "headers": {
    "Accept": [
      "*/*"
    ],
    "Host": [
      "httpbin.default.svc.cluster.local:8000"
    ],
    "User-Agent": [
      "curl/8.12.1"
    ],
    "X-Envoy-Attempt-Count": [
      "1"
    ],
    "X-Forwarded-Client-Cert": [
      "By=spiffe://west.local/ns/default/sa/httpbin;Hash=b5b574fb226390182ac75dcb70fc035e55ef9a21af41a348bd27b271e63d808b;Subject=\"\";URI=spiffe://west.local/ns/default/sa/sleep"
    ],
    "X-Forwarded-Proto": [
      "http"
    ],
    "X-Request-Id": [
      "9cddd4d1-514a-4833-ade5-7108ca0fbd5b"
    ]
  }
}
< 
{ [561 bytes data]
* Connection #0 to host httpbin.default.svc.cluster.local left intact
```