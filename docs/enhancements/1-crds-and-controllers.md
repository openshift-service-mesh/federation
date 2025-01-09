| Status | Authors   | Created    | 
|--------|-----------|------------|
| Review | @jewertow | Jan 2 2025 |

# CRDs and controllers

## Overview

Currently, we use helm to manage the controller deployment and its configuration. The configuration is passed to the controller as CLI arguments.
This approach requires restarting the controller on any configuration change and causes pod failures in case of a misconfiguration.

Additionally, current approach to resource management is not reliable. We apply resources when it's necessary using the Istio client's `Apply` function,
so we are not able to reconcile created objects on user's modifications or controller uninstallation.

### Goals

Design CRDs for better user experience and ensure proper resource lifecycle and garbage collection.

### Non-goals

Design CRD for installing federation controller. Federation controller will be still installed with Helm,
and we are not planning any dedicated operator or integration with Sail Operator for now.

## Design

We need the following CRDs:
1. `MeshFederation` - cluster-scoped resource that includes general federation config, i.e. local settings, remote addresses and identities.
2. `FederationServiceRules` - specifies rules for exporting and importing services; parent for export-related Istio resources, i.e. `Gateway`, `DestinationRule`, etc.; must be created by the mesh admin.
3. `ImportedService` - represents an imported service; parent for import-related Istio resources, i.e. `ServiceEntry`, `WorkloadEntry`, etc.; managed only by the controller; should not be touch by users.

All resources will contain [standard conditions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties) in their status.

#### MeshFederation

`MeshFederation` includes settings related only to mesh federation, like remote ingress address, identities, etc.
This resource must be cluster-scoped, because it will be used as an owner for `ImportedService` resources, which can be created in multiple namespaces.

I would like to move away from thinking about "peers", as this is just an implementation detail.
I also thought that it may be confusing why peer address is used as the address for remote services.

Resource structure:

```yaml
apiGroup: federation.openshift-service-mesh.io/v1alpha1
kind: MeshFederation
metadata:
  name: default
spec:
  local:
    # ID is a unique identifier of the FDS peer, and it's used as a suffix in its Service and ServiceAccount names.
    id: east
    # Network name used by Istio for load balancing.
    network: east
    # Optional.
    # Trust domain is a property of the identity, which can't be determined by the controller.
    # Service account and namespace will be obtained from the downward API.
    # Default value: "cluster.local".
    trustDomain: mesh.east
    # Optional.
    # If no ingress is specified, it means the controller supports only single network topology.
    ingress:
      # Optional.
      # Local ingress type specifies how to expose exported services.
      # Currently, only two types are supported: istio and openshift-router.
      # If "istio" is set, then the controller assumes that the Service associated with federation ingress gateway
      # is LoadBalancer or NodePort and is directly accessible for remote peers, and then it only creates
      # an auto-passthrough Gateway to expose exported Services.
      # When "openshift-router" is enabled, then the controller creates also OpenShift Routes and applies EnvoyFilters
      # to customize the SNI filter in the auto-passthrough Gateway, because the default SNI DNAT format used by Istio
      # is not supported by OpenShift Router.
      type: istio
      # Optional.
      # Can be empty if ingress.type is openshift-router.
      gateway:
        # Ingress gateway selector specifies to which workloads Gateway configurations will be applied.
        selector:
          app: federation-ingress-gateway
        # Optional.
        port:
          # Optional.
          # Port name of the ingress gateway Service.
          # This is relevant only when the ingressType is openshift-router, but it cannot be empty.
          name: tls-passthrough
          # Optional.
          # Port of the ingress gateway Service.
          number: 15443
    # Optional.
    controlPlane:
      # Optional.
      # Control plane namespace used to create mesh-wide resources.
      namespace: istio-system
  remote:
  # ID is a unique identifier of the FDS peer, and it's used as a suffix for its Service name.
  - id: west
    # Network name used by Istio for load balancing.
    network: west
    # Optional.
    # Identity is needed to configure proper authorization policy.
    identity:
      # Optional.
      # Default value: "federation-discovery-service-<id>".
      serviceAccount: federation-discovery-service-west
      # Optional.
      # Default value: "istio-system".
      namespace: istio-system
      # Optional.
      # Default value: "cluster.local".
      trustDomain: mesh.west
    # Optional
    ingress:
      # Optional
      # Addresses contain a list of IPs or a single hostname used as addresses for imported services when meshes reside in different networks.
      # This field is not required in single network setup.
      addresses:
      - 192.168.1.1
      - 192.168.2.1
      # Optional.
      port: 443
      # Optional.
      # Remote ingress type specifies how to manage client mTLS.
      # Currently, only two types are supported: istio and openshift-router.
      # If "openshift-router" is set the controller applies DestinationRules with SNI compatible with OpenShift Router.
      # If "istio" is set client mTLS settings are not modified.
      type: istio
```

#### FederationServiceRules

`FederationServiceRules` is a namespaced resource for specifying export/import rules.
This resource is expected to be created as a single instance for all exported services.
This is necessary, because all exported services will be associated with a single e/w gateway, so we can't properly handle N:1 relation between resources.
Additionally, it will an owner of the egress gateway and its virtual service, because of the N:1 relation between
imported services and the egress gateway.

The key assumptions for export and import semantics in all variants of `FederationServicePolicy`:
1. Export and import rules DO NOT enforce any authorization policy.
2. Export and import rules ensure cross-cluster service discovery, and are not intended for enforcing security.
3. Export rules are always federation-wide - we do not allow to export services to particular meshes in a federation.
4. Import rules are defined per remote mesh to allow limiting service discovery, not access to services.

First variant of `FederationServiceRules` allows to export/import by label selectors and (names and namespaces).
All rules are OR-ed in this approach.

```yaml
apiGroup: federation.openshift-service-mesh.io/v1alpha1
kind: FederationServiceRules
metadata:
  # Name must be default.
  name: default
  # Namespace is expected to be the same as the controller's namespace.
  namespace: istio-system
spec:
  # Export rules applies to all meshes in the federation.
  # We do not allow to export services for particular meshes in the federation.
  export:
    # Service selectors allows to export particular services by label in any namespace
    serviceSelectors:
    - matchLabels:
        export: "true"
    - matchExpressions:
      - key: app.kubernetes.io/name
        operator: In
        values:
        - ratings
        - reviews
    # Namespace selectors allows to export all services from namespaces selected by label 
    namespaceSelectors:
    - matchLabels:
        istio-injection: enabled
    # Service list allows to export particular services in particular namespaces formatted as <namespace>/<service>.
    serviceList:
    - "ns-1/ratings"
    - "ns-1/reviews"
    - "ns-2/*"
    # Optional
    # DNS settings allow to customize DNS names for exported services.
    dnsSettings:
      # Optional
      # Search domain is a suffix for the service FQDN
      searchDomain: mesh.global
      # Optional
      # If true, then a service will be exposed as <original-svc-name>.<original-svc-ns>.<search-domain>
      includeNamespace: false
      # Examples:
      # 1. `searchDomain: svc.west.mesh` and `includeNamespace: true` will export ratings and reviews as ratings.bookinfo.svc.west.mesh and reviews.bookinfo.svc.west.mesh
      # 2. `searchDomain: mesh.global` and `includeNamespace: false` will export ratings and reviews as ratings.mesh.global and reviews.mesh.global.
  # Optional
  # Empty import allows to not subscribe FDS.
  # This rule imports only ratings service from any namespace in the west cluster.
  import:
  - meshID: west
    # Optional
    # If no selector is specified, then everything is imported.
    serviceSelectors:
    - matchExpressions:
      - key: app.kubernetes.io/name
        operator: In
        values:
        - ratings
  # This rule imports everything from the central cluster.
  - meshID: central
```

Rules based on label selectors are useful when an admin wants to give users the control on exporting Services,
while `serviceList` gives the admin full control on exported services.

We are able to provide the same functionality with only label selectors for services and namespaces.
In this variant, `export` is a list of rule sets containing `serviceSelector` or `namespaceSelector`.
Rules are OR-ed, but each rule set (an item in the `export` list) is AND-ed.

```yaml
apiGroup: federation.openshift-service-mesh.io/v1alpha1
kind: FederationServiceRules
metadata:
  # Name must be default.
  name: default
  # Namespace is expected to be the same as the controller's namespace.
  namespace: istio-system
spec:
  # The following list of rules exports services "ratings" and "reviews" from namespace "ns-1" and service "details" from namespace "ns-2".
  export:
    selectors:
    - serviceSelector:
        matchExpressions:
        - key: app.kubernetes.io/name
          operator: In
          values:
          - ratings
          - reviews
      namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: ns-1
    - serviceSelector:
        matchLabels:
          app.kubernetes.io/name: details
      namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: ns-2
    dnsSettings:
      searchDomain: mesh.global
  # Optional
  # Empty import allows to not subscribe FDS.
  import:
  - meshID: west
    # Optional
    # If no selector is specified, then everything is imported.
    serviceSelectors:
    - matchExpressions:
      - key: app.kubernetes.io/name
        operator: In
        values:
        - ratings
```

Example controller:
```go
func (r *FederationServiceRulesReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha1.FederationServiceRules{}).
        Owns(&v1.Gateway{}).
        Owns(&v1.ServiceEntry{}).
        Owns(&v1.EnvoyFilter{}).
        Watches(&corev1.Service{}, WithPredicates(checkIfMatchesExportRules)).
        Watches(&corev1.Namespace{}, WithPredicates(checkIfMatchesExportRules)).
        Watches(&v1alpha1.MeshFederation{}).
        Complete(r)
}
```

All export-related resource will include owner reference pointing to `FederationServiceRules`,
so deleting `FederationServiceRules` will result in removing these resources as well.

Example child resource:
```yaml
apiVersion: networking.istio.io/v1
kind: Gateway
metadata:
  name: federation-ingress-gateway
  namespace: istio-system
  ownerReferences:
  - apiVersion: federation.openshift-service-mesh.io/v1alpha1
    kind: FederationServiceRules
    name: default
    uid: a8e825b9-911e-40b8-abff-58f37bb3e05d
spec:
  selector:
    app: federation-ingress-gateway
  ...
```

#### ImportedService

`ImportedService` will be created for a group of services from multiple meshes with the same FQDN,
and it will have the following structure:

```yaml
apiGroup: federation.openshift-service-mesh.io/v1alpha1
kind: ImportedService
metadata:
  # Name is a dash-separated FQDN of an imported service
  name: productpage-bookinfo-svc-mesh-global
  # Namespace depends on whether the service exists in the local cluster.
  # If it does not exist locally, it will be root mesh namespace, otherwise it will be the original namespace. 
  namespace: istio-system
  # Having MeshFederation set as an owner ensures proper resource cleanup after deleting federation.
  ownerReferences:
  - apiVersion: federation.openshift-service-mesh.io/v1alpha1
    kind: MeshFederation
    name: default
    uid: a8e825b9-911e-40b8-abff-58f37bb3e05d
spec:
  # Original name
  name: productpage
  # Original namespace
  namespace: bookinfo
  # Hostname comes from remote FDS and is created based on export rules defined in FederationServiceRules.
  hostname: productpage.bookinfo.svc.mesh.global
  # If a service is federated in more than one mesh, and that service has different ports in meshes,
  # then it will be imported only from the first mesh. 
  ports:
  - name: http
    number: 80
    protocol: HTTP
  # Labels must be the same in all meshes - same as in case of ports.
  labels:
    app: productpage
    security.istio.io/tlsMode: istio
  # List of meshes from which the service is imported.
  # This is required to create correct list of endpoints and identities.
  meshes:
  - west
  - central
```

This resource will own import-related resources, like `ServiceEntry` or `WorkloadEntry`, and `DestinationRule`,
but it will not manage resource for egress gateway, i.e. `Gateway` and `VirtualService`.
This is because there will be only one gateway and one virtual service for all imported services,
so there would be N owners for 1 resource. Therefore, resources for egress gateway will need dedicated controllers,
and their owner will be `FederationServiceRules`.

Example controller implementation:
```go
func (r *ImportedServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha1.ImportedService{}).
        Owns(&v1.ServiceEntry{}).
        Owns(&v1.WorkloadEntry{}).
        Owns(&v1.DestinationRule{}).
        Watches(&corev1.Service{}, WithPredicates(checkIfMatchesExportRules)).
        Watches(&corev1.Namespace{}, WithPredicates(checkIfMatchesExportRules)).
        Watches(&v1alpha1.MeshFederation{}).
        Complete(r)
}
```

Example child resource:
```yaml
apiVersion: networking.istio.io/v1
kind: ServiceEntry
metadata:
  name: import-productpage-bookinfo
  namespace: istio-system
  ownerReferences:
  - apiVersion: federation.openshift-service-mesh.io/v1alpha1
    kind: ImportedService
    name: productpage-bookinfo-svc-mesh-global
    uid: a8e825b9-911e-40b8-abff-58f37bb3e05d
spec:
  hosts:
  - productpage.bookinfo.svc.mesh.global
  ...
```

> [!IMPORTANT]
> As the `ImportedService` is fully managed by the controller, the initial reconciliation must be triggered programmatically by the FDS client.
> We should evaluate possible option and choose the best one during the implementation.
> Example options:
> * FDS client can call the controller function directly during processing FDS response.
> * FDS client can trigger the reconciliation loop sending an imported service via a channel to the controller. 
