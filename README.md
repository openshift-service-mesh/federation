# Federation

This project implements Istio mesh federation using a Kubernetes controller that provides an API
for managing multi-mesh communication, implements service discovery and automates the management of Istio configuration.

Mesh federation enables mTLS communication between applications across mesh boundaries.
Each mesh can federate a subset of its services to allow applications from other meshes to connect to these services.

Federated services are exposed on a passthrough gateway, so mTLS is not terminated at the edge of the cluster,
and authorization can be performed by the federated application.

## How it works

Controllers are deployed with sidecars like any other application in the mesh. Controllers connect to remote peers
and send a discovery request to subscribe to federated services.
When a controller receives discovery response with a federated service, it creates `ServiceEntriy` or `WorkloadEntry`
in the local cluster depending on whether the service exists locally.
The controller also watches local `Service` objects and checks if they match the export rules - if so,
it adds the service's FQDN to the auto-passthrough `Gateway` hosts.

### Architecture

![architecture](docs/img/architecture.jpg)

## Multi-primary vs federation

[Multi-primary](https://istio.io/latest/docs/setup/install/multicluster/multi-primary_multi-network/) and
[primary-remote](https://istio.io/latest/docs/setup/install/multicluster/primary-remote_multi-network/) topologies
are great solutions for expanding single mesh to multiple k8s clusters for better system resiliency and higher availability.
However, these solutions do not fit well in the following cases:

1. Decentralized control and ownership of clusters.

    **Use case**: Different teams or departments manage their own clusters and control planes independently.

    **Reason**: Federation allows each team to maintain autonomy over their cluster’s Istio configurations while still enabling
    selective cross-mesh communication.

1. Simplified networking between clusters.

    **Use case**: Clusters communicate over public networks without a shared private network (e.g., VPC peering).

    **Reason**: Both federation and multi-primary rely on gateway-based communication for the data-plane traffic,
    but in multi-primary deployment control planes need access to remote kube-apiservers and that usually requires
    additional network configuration for secure access, as most users do not want to expose kube-apiserver to the internet.

1. Limited service sharing.

    **Use case**: Only a subset of services needs to be shared across clusters (e.g., common APIs or external-facing services).

    **Reason**: Federation allows you to expose and consume specific services across meshes using service entries, 
    without fully integrating the control planes. This is partially possible in multi-primary deployment, 
    but exporting services could be limited only to namespaces matching configured discovery selectors.

1. Operational simplicity for isolated meshes.

   **Use case**: You want to simplify troubleshooting and upgrades by isolating cluster-specific issues.

   **Reason**: Since federated meshes don’t rely on a shared control plane, issues are localized to individual clusters.

## Identity and trust model

This controller does not provide any mechanism to share trust bundles between meshes using different CAs.
It can only enable mTLS communication between meshes if all use the same CA or use SPIRE with enabled trust bundle federation.

## Getting started

Follow these guides to see how it works in practice:
1. [Simple multi-cluster bookinfo deployment](examples/README.md).
2. [Integration with SPIRE](examples/spire/README.md).

## Comparing to other projects

#### Admiral

Admiral is primarily designed to manage multi-cluster service discovery and traffic distribution in Istio,
focusing on use cases where clusters are part of a single logical mesh, such as multi-primary or primary-remote topologies.
Admiral does not natively support mesh federation in the way Istio defines it, so it's a very different project
and federation controller is not an alternative for it.

#### SPIRE

SPIRE federation enables secure communication and authentication for workloads in different trust domains integrated with different CAs.
These projects ideally complement each other, as SPIRE federates identities, and federation controller federates Istio services and endpoints.
