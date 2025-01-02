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

A controller for `FederatedServicePolicy` will look like:
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

#### ImportedService

`ImportedService` will be created for or updated by the FDS client for each imported service.
A dedicated controller will watch this CR and manage its child resources, like `ServiceEntry`, `WorkloadEntry`, `DestinationRule`, etc.
Its owner will be `MeshFederation`, so when `MeshFederation` will be removed `ImportedService` and its child resources will be removed.

```yaml
apiGroup: federation.openshift-service-mesh.io/v1alpha1
kind: ImportedService
metadata:
  name: <generated-name>
  namespace: istio-system
  ownerReferences:
  - apiVersion: federation.openshift-service-mesh.io/v1alpha1
    kind: MeshFederation
    name: default
    uid: a8e825b9-911e-40b8-abff-58f37bb3e05d
spec:
  # importAsLocal specifies if the controller needs to create ServiceEntry or WorkloadEntry and what namespace should be
  # used for child resources - root mesh namespace or the original namespace.
  importAsLocal: false
  name: <original-svc-name>
  namespace: <original-svc-namespace>
# TODO
# status:
```

Example child resource
```yaml
apiVersion: networking.istio.io/v1
kind: ServiceEntry
metadata:
  name: import-productpage-bookinfo
  namespace: istio-system
  ownerReferences:
  - apiVersion: federation.openshift-service-mesh.io/v1alpha1
    kind: ImportedService
    name: productpage
    uid: a8e825b9-911e-40b8-abff-58f37bb3e05d
spec:
  hosts:
  - productpage.bookinfo.svc.cluster.local
  ...
```

A controller for `ImportedService` will look like:
```go
func (r *ImportedServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha1.ImportedService{}).
        Owns(&v1.ServiceEntry{}).
        Owns(&v1.WorkloadEntry{}).
        Owns(&v1.DestinationRule{}).
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

`ImportedService` will be applied by FDS client like below:
```go
func (h *ImportedServiceHandler) Handle(source string, resources []*anypb.Any) error {
	...
    _ := r.client.V1alpha1().ImportedService(namespace).Apply(ctx, &v1alpha1.ImportedServiceApplyConfiguration{
        TypeMetaApplyConfiguration: applyconfigurationv1.TypeMetaApplyConfiguration{
            APIVersion: "federation.openshift-service-mesh.io/v1alpha1",
			Kind:       "ImportedService",
        },
        ObjectMetaApplyConfiguration: &applyconfigurationv1.ObjectMetaApplyConfiguration{
            Name:      name,
            Namespace: namespace,
            Labels:    labels,
            OwnerReferences: []applyconfigurationv1.OwnerReferenceApplyConfiguration{{
                APIVersion: "federation.openshift-service-mesh.io/v1alpha1",
                Kind:       "MeshFederation",
                Name:       "default",
                UID:        uid,
                Controller: true,
            }},
        },
        Spec: spec,
        Status: nil,
        }, metav1.ApplyOptions{
            TypeMeta: metav1.TypeMeta{
            APIVersion: "ImportedService",
            Kind:       "federation.openshift-service-mesh.io/v1alpha1",
        },
        Force:        true,
        FieldManager: "federation-controller",
    })
    return nil
}
```
