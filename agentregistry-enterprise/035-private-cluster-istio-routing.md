# Private-Cluster Routing via Istio Gateway + NLB

Use this lab when the AgentRegistry Enterprise Service must be `ClusterIP` — typically because the cluster lives in a private VPC and a public `LoadBalancer` Service isn't an option. Traffic enters through an existing (or new) Istio Gateway backed by an internal AWS NLB, then an `HTTPRoute` forwards it to the AgentRegistry Service.

## Architecture

```
Internet  ──▶  [ NLB ]  ──▶  [ Istio Gateway (GatewayClass: istio) ]
                                    │
                                    │ HTTPRoute
                                    ▼
                       Service/agentregistry-enterprise  (ClusterIP)
                              ├─ :8080  HTTP (UI + API)
                              ├─ :21212 gRPC
                              └─ :31313 MCP
```

## Lab Objectives

- Confirm the AgentRegistry Service is `ClusterIP`
- Attach an `HTTPRoute` to an existing Istio Gateway (Option A) or create a new dedicated Gateway (Option B)
- Optionally route by hostname using a Route 53 private hosted zone
- Smoke test from inside the VPC, plus `kubectl port-forward` for laptop access

## Prerequisites

- [030 — AgentRegistry Enterprise installed](030-install-agentregistry-helm.md) with `service.type: ClusterIP`
- Kubernetes Gateway API CRDs
- Istio installed with `GatewayClass: istio`
- An Istio Gateway with an NLB listener (or permissions to create one)

## 1. Verify the Service is ClusterIP

```bash
kubectl get svc agentregistry-enterprise -n agentregistry-system
```

Expected `TYPE: ClusterIP`, no `EXTERNAL-IP`:

```
NAME                       TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)
agentregistry-enterprise   ClusterIP   10.100.x.x     <none>        8080/TCP,21212/TCP,31313/TCP
```

## Option A — Attach to an Existing Istio Gateway

```bash
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: agentregistry-route
  namespace: agentregistry-system
spec:
  parentRefs:
    - name: <EXISTING_GATEWAY_NAME>
      namespace: <EXISTING_GATEWAY_NAMESPACE>
      sectionName: https
  hostnames:
    - "agentregistry.internal.example.com"
  rules:
    - matches:
        - path: { type: PathPrefix, value: / }
      backendRefs:
        - { group: "", kind: Service, name: agentregistry-enterprise, port: 8080 }
EOF
```

If the Gateway lives in a different namespace, allow the cross-namespace reference with a `ReferenceGrant`:

```bash
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: allow-gateway-to-agentregistry
  namespace: agentregistry-system
spec:
  from:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      namespace: <EXISTING_GATEWAY_NAMESPACE>
  to:
    - { group: "", kind: Service, name: agentregistry-enterprise }
EOF
```

## Option B — Create a Dedicated Gateway

```bash
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: agentregistry-gateway
  namespace: agentregistry-system
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-scheme: "internal"
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
spec:
  gatewayClassName: istio
  listeners:
    - name: http
      port: 80
      protocol: HTTP
      allowedRoutes:
        namespaces: { from: Same }
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: agentregistry-route
  namespace: agentregistry-system
spec:
  parentRefs:
    - { name: agentregistry-gateway, namespace: agentregistry-system }
  rules:
    - matches:
        - path: { type: PathPrefix, value: / }
      backendRefs:
        - { group: "", kind: Service, name: agentregistry-enterprise, port: 8080 }
EOF
```

## 2. Get the Gateway Address

```bash
export AR_ENDPOINT=$(kubectl get gateway <GATEWAY_NAME> -n <GATEWAY_NAMESPACE> \
  -o jsonpath='{.status.addresses[0].value}')
echo "AgentRegistry endpoint: http://$AR_ENDPOINT"
```

For an internal NLB this will be something like `internal-xxxx.elb.us-east-1.amazonaws.com`, reachable only from inside the VPC.

## 3. Smoke Test from Inside the VPC

From a bastion, VPN, or SSM session:

```bash
curl -s "http://$AR_ENDPOINT/v0/version" | python3 -m json.tool
curl -s -o /dev/null -w "%{http_code}" "http://$AR_ENDPOINT/healthz"
```

## 4. Laptop Access via `kubectl port-forward`

The internal NLB isn't reachable from your laptop. Tunnel through the API server:

```bash
# Forward the Service directly
kubectl -n agentregistry-system port-forward svc/agentregistry-enterprise 8080:8080

# ...or forward through the Gateway to test the full path
kubectl -n agentregistry-system port-forward svc/agentregistry-gateway-istio 8080:80
```

Then open <http://localhost:8080>.

## 5. (Optional) Private Hosted Zone DNS

```bash
aws route53 change-resource-record-sets \
  --hosted-zone-id <PRIVATE_HOSTED_ZONE_ID> \
  --change-batch "{
    \"Changes\": [{
      \"Action\": \"UPSERT\",
      \"ResourceRecordSet\": {
        \"Name\": \"agentregistry.internal.example.com\",
        \"Type\": \"CNAME\",
        \"TTL\": 300,
        \"ResourceRecords\": [{\"Value\": \"${AR_ENDPOINT}\"}]
      }
    }]
  }"
```

## Production Notes

- Replace the bundled PostgreSQL with RDS in the same VPC (`database.postgres.bundled.enabled: false`, `database.postgres.url: <conn-string>`).
- Terminate TLS at the Gateway (ACM cert on the NLB or a Gateway TLS listener).
- Run `arctl` from a bastion or a GitLab runner inside the VPC — see [095](095-gitops-gitlab-ci.md).

## Next

- [040 — Authenticate `arctl`](040-arctl-auth.md)
