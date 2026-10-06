# Where Substrate Works

The first of two mandatory setup labs. Substrate needs `PodCertificateRequest` enabled on the Kubernetes API server. This lab tells you which clusters can do that. It installs nothing.

## Lab Objectives

- Decide whether your cluster can enable `PodCertificateRequest`
- Know when that gate stops mattering (Kubernetes v1.37 and above)

## Prerequisites

- A Kubernetes cluster you can inspect with `kubectl`
- Permission to read the API server version and, on a cluster you administer, its feature gates

## What This Lab Does **Not** Do

This lab does not install Substrate, kagent, or any cryptographic material. Those come in [002](002-install.md).

## 1. The Gate

`PodCertificateRequest` is an API-server feature. Enabling it is an API-server change, not a kubelet flag.

> **Kubernetes v1.37 and above.** The `PodCertificateRequest` gate is no longer required. Upstream Kubernetes makes it stable and leaves it on by default in 1.37.

The same note applies to the install prerequisites in [002](002-install.md).

## 2. Where It Works Today

1. GKE, which lets you enable the gate.
2. Any cluster where you own the API server: microk8s, kind, minikube, kubeadm, and the same class of install.

## 3. Kagent Fix

Within the latest build of kagent v1, the `PodCertificateRequest` need is fixed. It can now run on clusters back to k8s v1.34.

## What's in Place After This Lab

| Resource | State |
|---|---|
| Cluster | Unchanged |
| Substrate and kagent | Not installed |

You either continue to [002](002-install.md), or you stop because the control plane cannot expose `PodCertificateRequest`.

## Cleanup

This lab creates nothing.

## Next

- [002 - Install Substrate and kagent](002-install.md)
