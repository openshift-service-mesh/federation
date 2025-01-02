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
2. `ExportedService` - represents an exported service; parent for export-related Istio resources, i.e. `Gateway`, `DestinationRule`, etc.; can be created by a user or the controller (if proper rules are defined in `MeshFederation`).
3. `ImportedService` - represents an imported service; parent for import-related Istio resources, i.e. `ServiceEntry`, `WorkloadEntry`, etc.; created by FDS.

#### MeshFederation

`MeshFederation` must be a cluster-scoped resource, because it will be a parent for resources created in many namespaces.
This resource will contain settings related only to mesh-federation topology, not federation-controller settings.
The controller remains managed by helm values.
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
  remotes:
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
  conditions: []
  state: Healthy
  # TODO: keep state per remote peer
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