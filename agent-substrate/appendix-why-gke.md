# Appendix - Why GKE (the Pod Certificate Requirement)

This workshop's main path targets **GKE Standard** because of one specific Substrate dependency: the Pod Certificate beta APIs. This appendix unpacks why that's the case, what GKE exposes that other managed offerings don't, and what your options are on AKS / EKS / self-managed clusters.

## Which Pods Need Pod Certificates

Three Substrate components mount a `podCertificate` projected volume - and one of them is a multi-pod StatefulSet, so it's not just one pod:

| Component | Replicas | Volume mount |
|---|---|---|
| `ate-api-server` | 1+ | `podCertificate` projected volume |
| `atenet-router` | 1+ | `podCertificate` projected volume |
| `valkey` | 6 (StatefulSet) + 1 cluster-init Job | `podCertificate` projected volume |

That's **8+ pods** that won't start without working Pod Certificate support.

> **Important distinction.** The `pod-certificate-controller` itself is the *signer* - it bootstraps from plain `secret` CA-pool volumes, not `podCertificate` projected volumes. It runs even on clusters without the beta APIs. What fails is everything the signer is supposed to issue certificates *to*.

## What the `podCertificate` Volume Needs

The `podCertificate` projected volume is a Kubernetes 1.30+ feature gated behind beta APIs and feature gates that are **off by default** in upstream Kubernetes 1.36:

| API | Feature gate |
|---|---|
| `certificates.k8s.io/v1beta1` | (registered as a beta API on the API server) |
| `PodCertificateRequest` | feature gate on the apiserver |
| `ClusterTrustBundle` | feature gate on the apiserver |
| `ClusterTrustBundleProjection` | feature gate on the apiserver |

If any of these aren't enabled, the volume **fails to mount**, the pod **fails to start**, and you get a stream of `MountVolume.SetUp failed ... ClusterTrustBundle projection is not supported in static kubelet mode` events from the kubelet.

There's a second tier: even with the apiserver feature gates on, the **kubelet** has to be new enough to honor the projection. That's why pre-existing nodes need to be recreated (see [030 reactive checks](002-gcp-iam-and-bucket.md#reactive-checks-run-only-if-step-3-in-040-hits-these-symptoms)) - kubelet feature support is decided at node creation time.

## What GKE Exposes

GKE has a supported knob for exactly this: `--enable-kubernetes-unstable-apis`. You tell GKE which beta resources to enable, and GKE flips the apiserver flags for you. The relevant invocation is in [030](002-gcp-iam-and-bucket.md):

```bash
gcloud container clusters update "$CLUSTER_NAME" \
 --location="$CLUSTER_LOCATION" --project="$PROJECT_ID" \
 --enable-kubernetes-unstable-apis=certificates.k8s.io/v1beta1/podcertificaterequests,certificates.k8s.io/v1beta1/clustertrustbundles
```

This is what makes GKE the **supported managed path** for Substrate today.

> **Side note:** "enabled beta APIs cannot be disabled" on GKE - `gcloud` has no `--disable-kubernetes-unstable-apis`. Once flipped on, the cluster carries them for life. See the [GKE beta API docs](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/use-beta-apis).

## What AKS Exposes

**Nothing equivalent today.** Managed AKS doesn't expose apiserver feature gates directly. The standing request is [Azure/AKS#1887](https://github.com/Azure/AKS/issues/1887). Until that lands, your options on Azure:

1. **Cluster API / kubeadm / k3s on Azure VMs** - full control over apiserver flags
2. **AKS with the Substrate Helm chart's override flags** - see [040 step 2: "If You're Not on GKE or Kind"](003-install-substrate.md#if-youre-not-on-gke-or-kind). This works for the JWT issuer side but **does not** solve the Pod Certificate apiserver-flag requirement
3. **Wait for the upstream AKS request**

The chart override flags from the kagent-substrate install doc treat AKS as a target *for kagent integration*, not for the Substrate control plane itself.

## What EKS Exposes

Same picture as AKS: no apiserver flag exposure. You'd need a cluster where you control the apiserver - Cluster API on EC2, or kubeadm/k3s on EC2.

## Local Dev - kind Works

CI for Substrate itself runs on `kind`, where the apiserver flags are passed via the cluster config file (`hack/create-kind-cluster.sh`). This is the local-dev escape hatch - see [appendix-kind-quickstart](appendix-kind-quickstart.md). It bypasses the GKE-vs-AKS-vs-EKS question entirely because kind is a development tool, not a managed offering.

## Why Pod Certificates At All

You might wonder: why does Substrate need per-pod mTLS at all?

The architecture doc puts it simply: every internal system communication (Control Plane ↔ atelet, atelet ↔ ateom, atenet → Control Plane) is secured via **mutual TLS with short-lived certificates**. Pod Certificates are how Substrate gets short-lived per-pod certs **without** running a sidecar or rolling certs out of band. The `pod-certificate-controller` is the signer; the `podCertificate` projected volume is how each pod consumes its cert.

This is independent of **actor identity** - `SessionIdentity` (`MintJWT` / `MintCert`) is a separate gRPC service backed by JWT/CA pool Secrets. Actor / worker / ateom pods do **not** mount `podCertificate` volumes - only the Substrate **infrastructure** pods do.

## TL;DR

- Substrate's infra pods (`ate-api-server`, `atenet-router`, 6× `valkey`) mount `podCertificate` projected volumes
- That volume needs Kubernetes apiserver-level feature gates that are off by default upstream
- GKE has a supported knob (`--enable-kubernetes-unstable-apis`)
- Managed AKS / EKS don't expose this knob today
- For local dev, kind works (passes the flags through the cluster config)

## Related

- [010 - GKE Cluster Prerequisites](001-baseline-setup.md)
- [030 - GCP IAM and Bucket](002-gcp-iam-and-bucket.md) - Step 2a flips the GKE knob
- [appendix-kind-quickstart](appendix-kind-quickstart.md) - local-dev alternative
- [Upstream architecture doc - Security & Isolation](https://github.com/agent-substrate/substrate/blob/main/docs/architecture.md#security--isolation)
