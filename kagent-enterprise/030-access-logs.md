# Gateway Access Logs (kgateway `HTTPListenerPolicy`)

When you install kagent-enterprise with the Gloo Operator ([020](020-install-kagent-enterprise.md)), Gloo Gateway runs in `gloo-system` and the agentgateway dataplane is one of the listeners on the `kagent-gateway` Gateway. To get structured JSON access logs out of the gateway, attach a kgateway `HTTPListenerPolicy` with a JSON-formatted file sink that writes to `/dev/stdout` (so logs flow to the gateway pod's stdout and your log aggregator picks them up automatically).

## Lab Objectives

- Apply a `gateway.kgateway.dev/v1alpha1 HTTPListenerPolicy` named `access-logs` to the `kagent-gateway` Gateway in `gloo-system`
- Confirm structured JSON access logs appear in the gateway pod's stdout

## Prerequisites

- [020 — Kagent Enterprise installed via the Gloo Operator](020-install-kagent-enterprise.md)

## Apply the Policy

The manifest lives at [`assets/observability/kagent-gateway-access-logs.yaml`](assets/observability/kagent-gateway-access-logs.yaml).

```bash
kubectl apply -f assets/observability/kagent-gateway-access-logs.yaml
```

That manifest is:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: HTTPListenerPolicy
metadata:
  name: access-logs
  namespace: gloo-system
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: kagent-gateway
  accessLog:
  - fileSink:
      path: /dev/stdout
      jsonFormat:
          start_time: "%START_TIME%"
          method: "%REQ(X-ENVOY-ORIGINAL-METHOD?:METHOD)%"
          path: "%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%"
          protocol: "%PROTOCOL%"
          response_code: "%RESPONSE_CODE%"
          response_flags: "%RESPONSE_FLAGS%"
          bytes_received: "%BYTES_RECEIVED%"
          bytes_sent: "%BYTES_SENT%"
          total_duration: "%DURATION%"
          resp_backend_service_time: "%RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)%"
          req_x_forwarded_for: "%REQ(X-FORWARDED-FOR)%"
          user_agent: "%REQ(USER-AGENT)%"
          request_id: "%REQ(X-REQUEST-ID)%"
          authority: "%REQ(:AUTHORITY)%"
          backendHost: "%UPSTREAM_HOST%"
          backendCluster: "%UPSTREAM_CLUSTER%"
```

## Verify

Send any request through the gateway (the easiest is to open the kagent UI and prompt an agent — see [050](050-troubleshooting-pod.md)) and tail the gateway pod's stdout:

```bash
kubectl logs -n gloo-system -l app.kubernetes.io/name=kagent-gateway -f
```

You should see structured JSON lines like:

```json
{"start_time":"2026-06-15T11:23:54.123Z","method":"POST","path":"/api/agents/k8s-agent/invoke","protocol":"HTTP/1.1","response_code":200,"response_flags":"-","bytes_received":312,"bytes_sent":1024,"total_duration":842,"resp_backend_service_time":"840","req_x_forwarded_for":"10.0.0.5","user_agent":"kagent-ui/0.1.5","request_id":"a1b2c3d4-...","authority":"localhost:8090","backendHost":"10.0.0.42:8083","backendCluster":"kagent_controller_kagent_8083"}
```

## Notes

- The Gateway name must match what Gloo Gateway provisions in `gloo-system`. The Gloo Operator install in [020](020-install-kagent-enterprise.md) creates `kagent-gateway` — confirm with `kubectl get gateway -n gloo-system`.
- The `targetRefs` selector binds this policy to the Gateway. To narrow to a specific listener, add `sectionName` to the target reference.
- If you don't want every field, drop entries from `jsonFormat`. Envoy supports a wide range of `%...%` substitutions — see the Envoy access-log documentation for the full list.
- If you also want to enrich logs with JWT claims (group, email, etc.) once OIDC is in place, agentgateway has its own `EnterpriseAgentgatewayPolicy.frontend.accessLog` mechanism with `jwt.*` CEL expressions — that's a different policy applied to the agentgateway Gateway rather than the Gloo Gateway one.

## Next

- [040 — Declarative MCP Server + Agent](040-mcp-connection-agent-config.md)
