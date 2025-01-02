| Status | Authors   | Created    | 
|--------|-----------|------------|
| Review | @jewertow | Jan 2 2025 |

# CRDs and kubebuilder

## Overview
We need CRDs to properly manage lifecycle of the Istio resources created by our controller.
Currently, resources are not removed when the controller is uninstalled, and that results in mesh misconfiguration.

## Design
We need the following CRDs:
1. `MeshFederation` - cluster-scoped CRD that includes general federation config, i.e. local settings, remote addresses and identities.
2. `FederatedServicePolicy` - specifies rules for exporting (and/or importing - TBD) services; parent for export-related Istio resources, i.e. `Gateway`, `DestinationRule`, etc.; must be created by a user.
3. `ImportedService` - represents an imported service; parent for import-related Istio resources, i.e. `ServiceEntry`, `WorkloadEntry`, etc.; created by FDS.

#### MeshFederation

`MeshFederation` specifies settings related only to mesh federation topology, and it does not configure federation-controller workload resources.
Workload-related resources, like `Deployment`, `Service` etc. will be still configured with helm values.
This resource must be cluster-scoped, because it will be used as a parent for resources created in multiple namespaces.

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
    # Specifies ingress settings and properties.
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
status:
  conditions:
  - meshID: west
    status: Connected
  - meshID: central
    status: Disconnected
    lastErrorMessage: "No route to host 192.168.2.1"
```

#### FederatedServicePolicy

`FederatedServicePolicy` is a namespaced resource for specifying rules to export (and import - TBD) services.
This resource is expected to be created as a single instance for all exported (and imported - TBD) services.
This is necessary, because all exported services will be associated with a single e/w gateway, so we can't map n to 1 resources.

```yaml
apiGroup: federation.openshift-service-mesh.io/v1alpha1
kind: FederatedServicePolicy
metadata:
  # Name must be default.
  name: default
  # Namespace is expected to be the same as the controller's namespace.
  namespace: istio-system
spec:
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
    # Service list allows to export particular services in particular namespaces.
    serviceList:
    - "ratings/ns-1"
    - "reviews/ns-1"
    - "*/ns-2"
# TODO
# status:
```

`serviceSelectors`, `namespaceSelectors` and `serviceList` are OR-ed.
Rules based on label selectors are useful when an admin wants to give users the control on exporting Services,
while `serviceList` gives the admin full control on exported services.

This code shows what the controller for `FederatedServicePolicy` will own and watch:
```go
func (r *FederatedServicePolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha1.FederatedServicePolicy{}).
        Owns(&v1.Gateway{}).
        // only if custom domain is set
        Owns(&v1.ServiceEntry{}).
        // only if OpenShift router is enabled
        Owns(&v1.EnvoyFilter{}).
        Watches(
            &corev1.Service{},
            handler.EnqueueRequestsFromMapFunc(),
            builder.WithPredicates(checkIfMatchesExportRules),
        ).
        Watches(
            &corev1.Namespace{},
            handler.EnqueueRequestsFromMapFunc(),
            builder.WithPredicates(checkIfMatchesExportRules),
        ).
        Watches(
            &v1alpha1.MeshFederation{},
            handler.EnqueueRequestsFromMapFunc(),
        ).
        Complete(r)
}
```

All export-related resource will contain ownerReference pointing to `FederatedServicePolicy`,
so deleting `FederatedServicePolicy` will result in removing these resources.
```yaml
apiVersion: networking.istio.io/v1
kind: Gateway
metadata:
  name: federation-ingress-gateway
  namespace: istio-system
  ownerReferences:
  - apiVersion: federation.openshift-service-mesh.io/v1alpha1
    kind: FederatedServicePolicy
    name: default
    uid: a8e825b9-911e-40b8-abff-58f37bb3e05d
spec:
  selector:
    app: federation-ingress-gateway
```

### User Stories

### API Changes

### Architecture

### Performance Impact

### Backward Compatibility

### Kubernetes vs OpenShift vs Other Distributions

## Alternatives Considered
Other approaches that have been discussed and discarded during or before the creation of the SEP. Should include the reasons why they have not been chosen.

## Implementation Plan
In the beginning, this should give a rough overview of the work required to implement the SEP. Later on when the SEP has been accepted, this should list the epics that have been created to track the work.

## Test Plan
When and how can this be tested? We'll want to automate testing as much as possible, so we need to start about testability early.

## Change History (only required when making changes after SEP has been accepted)
* 2024-07-09 Fixed a typo