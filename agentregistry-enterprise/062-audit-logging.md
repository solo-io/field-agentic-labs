# Audit Logging - Local Debug and Splunk

Agentregistry Enterprise `2026.7.0` adds structured control-plane audit events. The server emits the events as OpenTelemetry logs over OTLP/gRPC. This is separate from the runtime traces collected in [060](060-observability-tracing.md).

| Signal | What it records | Destination in this workshop |
|---|---|---|
| Runtime tracing | Agent runs, model calls, and tool calls | Agentregistry telemetry collector + ClickHouse |
| Audit logging | Registry lifecycle, approvals, authorization decisions, and resources applied to runtimes | Debug collector or Splunk |

This lab has two independent outcomes:

1. **Without Splunk:** send audit events to a disposable collector and inspect them with `kubectl logs`.
2. **With Splunk:** send events through the durable agentregistry audit relay and a Splunk HEC bridge.

## Architecture

### Part 1 - No Splunk

```text
agentregistry server -- OTLP/gRPC --> audit debug collector --> stdout
```

### Part 2 - Splunk

```text
agentregistry server
        |
        | OTLP/gRPC
        v
bundled audit collector (10 GiB WAL)
        |
        | OTLP/gRPC, in-cluster
        v
Splunk bridge collector (5 GiB persistent queue)
        |
        | HTTPS + HEC token
        v
Splunk Enterprise / Splunk Cloud
```

The second collector is required when Splunk exposes HEC but not a native OTLP/gRPC logs endpoint. It converts OTLP log records into Splunk HEC events.

## Lab Objectives

- Confirm the installed agentregistry version supports audit logging
- Generate lifecycle and authorization audit events without requiring a SIEM
- Inspect the event schema and OTel instrumentation scopes
- Configure a Splunk HEC bridge without storing the HEC token in Git
- Enable the bundled audit relay with persistent buffering
- Query audit events in Splunk

## Prerequisites

- Baseline setup complete: [001](001-baseline-setup.md) -> [002a](002a-setup-oidc-keycloak.md) **or** [002b](002b-setup-oidc-entra.md) -> [003](003-install-components.md)
- Agentregistry Enterprise `2026.7.0` or newer
- `arctl` authenticated as a registry administrator
- `kubectl`, Helm 3, `jq`, and `curl`
- A local checkout of this repository, with the current directory set to `agentregistry-enterprise/`
- For Part 2 only: a Splunk Enterprise or Splunk Cloud HEC endpoint, token, and writable index

> Audit events include actor subject, email, name, and roles when those claims exist. Treat the destination as identity-bearing security data. Events do not include secret values, issued tokens, credentials, or full resource payloads.

## 1. Confirm the Version and Current State

Check the running server image:

```bash
kubectl get deployment agentregistry-enterprise-server \
  -n agentregistry-system \
  -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
```

The tag must be `v2026.7.0` or newer. If the installation is older, upgrade the chart while preserving its existing user-supplied values:

```bash
helm upgrade agentregistry-enterprise \
  oci://us-docker.pkg.dev/solo-public/agentregistry-enterprise/helm/agentregistry-enterprise \
  --version 2026.7.0 \
  --namespace agentregistry-system \
  --reuse-values \
  --set image.tag=v2026.7.0 \
  --wait --timeout 10m
```

Inspect the effective audit settings:

```bash
helm get values agentregistry-enterprise \
  -n agentregistry-system -a -o json | jq .audit
```

Fresh `2026.7.0` values show:

```json
{
  "enabled": false,
  "authz": {
    "allowedDecisions": "sensitive"
  },
  "collector": {
    "enabled": false
  },
  "destination": {
    "otlp": {
      "endpoint": "",
      "insecure": false
    }
  }
}
```

The full computed output contains additional collector queue, retry, resource, and persistence defaults.

# Part 1 - Audit Logs Without Splunk

## 2. Deploy the Debug Collector

The debug collector accepts OTLP/gRPC on port `4317` and writes detailed log records to stdout. It has no persistent storage and is only for a lab.

```bash
kubectl apply -f assets/observability/audit/debug-collector.yaml
kubectl rollout status deployment/agentregistry-audit-debug \
  -n agentregistry-system --timeout=3m
```

Confirm the receiver is available:

```bash
kubectl get pod,service -n agentregistry-system \
  -l app.kubernetes.io/name=agentregistry-audit-debug
```

## 3. Enable Audit Export to the Debug Collector

For the demo, set `allowedDecisions=all` so a successful `arctl get` produces an authorization event. Production environments normally start with `sensitive` to reduce event volume.

```bash
helm upgrade agentregistry-enterprise \
  oci://us-docker.pkg.dev/solo-public/agentregistry-enterprise/helm/agentregistry-enterprise \
  --version 2026.7.0 \
  --namespace agentregistry-system \
  --reuse-values \
  --set audit.enabled=true \
  --set audit.collector.enabled=false \
  --set-string audit.destination.otlp.endpoint=agentregistry-audit-debug.agentregistry-system.svc.cluster.local:4317 \
  --set audit.destination.otlp.insecure=true \
  --set audit.authz.allowedDecisions=all \
  --wait --timeout 10m
```

Verify the server received the audit environment:

```bash
kubectl get deployment agentregistry-enterprise-server \
  -n agentregistry-system -o json | jq '[
    .spec.template.spec.containers[0].env[]
    | select(.name | startswith("AUDIT_"))
    | {name, value}
  ]'
```

## 4. Generate Lifecycle and Authorization Events

Create an AccessPolicy. This generates a `lifecycle` event:

```bash
arctl apply -f - <<'EOF'
apiVersion: ar.dev/v1alpha1
kind: AccessPolicy
metadata:
  name: audit-lab-viewer
spec:
  description: "Audit lab lifecycle event"
  principals:
    - kind: Role
      name: audit-lab-viewer
  rules:
    - actions:
        - registry:read
      resources:
        - kind: agent
          name: "*"
EOF
```

Run a permitted registry read to generate an `authorization` event:

```bash
arctl get agents
```

Update and delete the policy to produce more lifecycle actions:

```bash
arctl apply -f - <<'EOF'
apiVersion: ar.dev/v1alpha1
kind: AccessPolicy
metadata:
  name: audit-lab-viewer
spec:
  description: "Updated audit lab lifecycle event"
  principals:
    - kind: Role
      name: audit-lab-viewer
  rules:
    - actions:
        - registry:read
      resources:
        - kind: agent
          name: "*"
        - kind: prompt
          name: "*"
EOF

arctl delete accesspolicy audit-lab-viewer
```

If [051 - Approval Workflows](051-approval-workflows.md) is enabled, submitting and approving its test agent also produces `approval` events. Deploying an artifact through [010](010-aws-bedrock-runtime.md), [020](020-kagent-runtime-and-agent.md), or [032](032-mcp-through-agentgateway.md) produces `applied_resource` events.

## 5. Inspect the Events

```bash
kubectl logs deployment/agentregistry-audit-debug \
  -n agentregistry-system --since=10m | grep -A 35 'agentregistry.audit'
```

Look for:

```text
InstrumentationScope audit.resource_activity
EventName: agentregistry.audit.lifecycle
event.schema_version: v1
event.action: create | update | delete
actor.subject: <OIDC sub>
actor.roles: [...]
resource.kind: AccessPolicy
resource.name: audit-lab-viewer
```

Authorization records use the `audit.authz` scope. Denials and policy-evaluation errors are always emitted while audit logging is enabled. Successful decisions follow `audit.authz.allowedDecisions`.

> If you are continuing to Part 2, leave audit logging enabled until the Splunk bridge is ready. Part 2 changes the destination before removing the debug collector.

# Part 2 - Audit Logs With Splunk

## 6. Prepare Splunk HEC

In Splunk:

1. Create or select an index, such as `agentregistry_audit`.
2. Enable HTTP Event Collector.
3. Create a HEC token named `agentregistry-audit` with permission to write to that index.
4. Record the complete event endpoint. It normally ends in `/services/collector`.

Examples:

```text
Splunk Enterprise: https://splunk.example.com:8088/services/collector
Splunk Cloud:      https://http-inputs-<stack>.splunkcloud.com:443/services/collector
```

Set the non-secret values and read the token without adding it to shell history:

```bash
export SPLUNK_HEC_URL="https://splunk.example.com:8088/services/collector"
export SPLUNK_INDEX="agentregistry_audit"
export AUDIT_CLUSTER_NAME="$(kubectl config current-context)"
export AUDIT_ENVIRONMENT="lab"

read -rsp "Splunk HEC token: " SPLUNK_HEC_TOKEN
echo
```

Optionally verify the HEC endpoint before deploying the bridge:

```bash
curl -fsS \
  -H "Authorization: Splunk ${SPLUNK_HEC_TOKEN}" \
  "${SPLUNK_HEC_URL%/services/collector}/services/collector/health"
```

Store the token and connection settings in Kubernetes. The token is never written to a repository file:

```bash
kubectl create secret generic agentregistry-splunk-hec \
  -n agentregistry-system \
  --from-literal=token="${SPLUNK_HEC_TOKEN}" \
  --from-literal=url="${SPLUNK_HEC_URL}" \
  --from-literal=index="${SPLUNK_INDEX}" \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl create configmap agentregistry-audit-metadata \
  -n agentregistry-system \
  --from-literal=cluster_name="${AUDIT_CLUSTER_NAME}" \
  --from-literal=environment="${AUDIT_ENVIRONMENT}" \
  --dry-run=client -o yaml | kubectl apply -f -

unset SPLUNK_HEC_TOKEN
```

## 7. Deploy the Splunk Bridge

The bridge receives OTLP logs, enriches them with cluster metadata, and exports them through Splunk HEC. Its exporter queue uses a 5 GiB PVC so an acknowledged event is not left only in memory while Splunk is unavailable.

```bash
kubectl apply -f assets/observability/audit/splunk-otel-collector.yaml
kubectl rollout status deployment/agentregistry-splunk-otel-collector \
  -n agentregistry-system --timeout=5m
```

Check collector startup before moving the audit destination:

```bash
kubectl get pod,service,pvc -n agentregistry-system \
  -l app.kubernetes.io/name=agentregistry-splunk-otel-collector

kubectl logs deployment/agentregistry-splunk-otel-collector \
  -n agentregistry-system --since=5m
```

Do not continue if the collector reports an invalid configuration, TLS error, HEC authentication error, or unknown index.

## 8. Enable the Bundled Audit Relay

Point Agentregistry at the bridge and enable the chart-managed audit collector. The `insecure=true` setting applies only to the in-cluster OTLP hop. The bridge still verifies Splunk's HTTPS certificate.

```bash
helm upgrade agentregistry-enterprise \
  oci://us-docker.pkg.dev/solo-public/agentregistry-enterprise/helm/agentregistry-enterprise \
  --version 2026.7.0 \
  --namespace agentregistry-system \
  --reuse-values \
  --set audit.enabled=true \
  --set audit.collector.enabled=true \
  --set-string audit.destination.otlp.endpoint=agentregistry-splunk-otel-collector.agentregistry-system.svc.cluster.local:4317 \
  --set audit.destination.otlp.insecure=true \
  --set audit.authz.allowedDecisions=all \
  --wait --timeout 10m
```

Confirm both layers exist:

```bash
kubectl get deployment,service,pvc -n agentregistry-system | grep -E 'audit|splunk'
```

The chart-managed collector defaults include:

- 10 GiB persistent WAL
- Retry with exponential backoff and no elapsed-time limit
- A 10,000-request sending queue
- Four queue consumers
- `blockOnOverflow=false`

The WAL protects ordinary destination outages, but a completely full queue can still drop new records. Size and alert on both collector queues for production.

The old debug collector is no longer in the data path and can now be removed:

```bash
kubectl delete -f assets/observability/audit/debug-collector.yaml --ignore-not-found
```

## 9. Generate Events for Splunk

```bash
arctl apply -f - <<'EOF'
apiVersion: ar.dev/v1alpha1
kind: AccessPolicy
metadata:
  name: audit-splunk-demo
spec:
  description: "Lifecycle event sent to Splunk"
  principals:
    - kind: Role
      name: audit-lab-viewer
  rules:
    - actions:
        - registry:read
      resources:
        - kind: agent
          name: "*"
EOF

arctl get agents
arctl delete accesspolicy audit-splunk-demo
```

Check both collectors for export errors:

```bash
kubectl logs -n agentregistry-system \
  -l app.kubernetes.io/component=audit-collector \
  --all-containers --since=10m

kubectl logs deployment/agentregistry-splunk-otel-collector \
  -n agentregistry-system --since=10m
```

## 10. Query Splunk

Start with a broad search:

```spl
index=agentregistry_audit sourcetype="agentregistry:audit" "agentregistry.audit"
```

Show the principal, action, and affected resource:

```spl
index=agentregistry_audit sourcetype="agentregistry:audit"
| spath
| table _time event.name event.action actor.email actor.roles resource.kind resource.namespace resource.name
| sort - _time
```

Find denials and internal failures:

```spl
index=agentregistry_audit sourcetype="agentregistry:audit"
(error.type="denied" OR error.type="internal_error")
| table _time actor.subject event.action resource.kind resource.name authz.reason error.type
| sort - _time
```

Track approval activity:

```spl
index=agentregistry_audit sourcetype="agentregistry:audit" event.activity="approval"
| table _time event.action approval.id approval.state approval.submitter actor.subject
| sort - _time
```

If your Splunk field extraction differs, run the broad search, open one event, and confirm whether the OTel attributes are already indexed fields or need `spath` extraction.

## Choosing Authorization Volume

The lab uses `all` so successful reads are visible immediately. Set the production value deliberately:

| Value | Successful authorization records | Denials and errors |
|---|---|---|
| `sensitive` | Sensitive reads, such as Secret reads | Always emitted |
| `all` | Every successful permission check | Always emitted |
| `none` | None | Always emitted |

High-volume environments should measure `all` before adopting it. `sensitive` is the default.

## Troubleshooting

### Helm reports that the audit endpoint is required

`audit.enabled=true` requires a non-empty `audit.destination.otlp.endpoint`, whether or not the bundled audit collector is enabled.

### The debug collector is running but has no events

- Confirm the server pod restarted after the Helm upgrade.
- Inspect the `AUDIT_*` environment variables in step 3.
- Generate a create, update, or delete operation after audit logging is enabled.
- Check connectivity to the collector Service on port `4317`.

### The Splunk bridge reports `401` or `403`

- Confirm the HEC token is enabled.
- Confirm the token is authorized for `${SPLUNK_INDEX}`.
- Confirm `SPLUNK_HEC_URL` ends with `/services/collector`.
- Recreate the Secret after fixing the values, then restart the bridge Deployment.

### The Splunk bridge reports an x509 error

Use a CA-signed Splunk certificate for production. For a private CA, mount its CA bundle and set `tls.ca_file` in the collector configuration. Do not disable certificate verification as a permanent fix.

### Events appear twice

Confirm Agentregistry has only one audit destination. Do not send the server directly to the bridge while also configuring another collector to fan out the same records. Use `log.record.uid` for downstream deduplication when needed.

## Cleanup

Disable audit production before deleting either destination:

```bash
helm upgrade agentregistry-enterprise \
  oci://us-docker.pkg.dev/solo-public/agentregistry-enterprise/helm/agentregistry-enterprise \
  --version 2026.7.0 \
  --namespace agentregistry-system \
  --reuse-values \
  --set audit.enabled=false \
  --set audit.collector.enabled=false \
  --set-string audit.destination.otlp.endpoint="" \
  --set audit.destination.otlp.insecure=false \
  --set audit.authz.allowedDecisions=sensitive \
  --wait --timeout 10m
```

Delete lab resources:

```bash
arctl delete accesspolicy audit-lab-viewer 2>/dev/null || true
arctl delete accesspolicy audit-splunk-demo 2>/dev/null || true

kubectl delete -f assets/observability/audit/debug-collector.yaml \
  --ignore-not-found
kubectl delete -f assets/observability/audit/splunk-otel-collector.yaml \
  --ignore-not-found

kubectl delete secret agentregistry-splunk-hec \
  -n agentregistry-system --ignore-not-found
kubectl delete configmap agentregistry-audit-metadata \
  -n agentregistry-system --ignore-not-found

unset SPLUNK_HEC_URL SPLUNK_INDEX AUDIT_CLUSTER_NAME AUDIT_ENVIRONMENT
```

> Deleting `splunk-otel-collector.yaml` also deletes its 5 GiB queue PVC. Confirm the collector has drained before cleanup if the audit events must be retained.

## References

- [Agentregistry audit logging](https://docs.solo.io/agentregistry/latest/observability/audit-logging/)
- [Agentregistry Helm values](https://docs.solo.io/agentregistry/latest/reference/helm/)
- [OpenTelemetry Splunk HEC exporter](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/splunkhecexporter)

## Next

- [099 - Cleanup](099-cleanup.md) - tear down the full workshop baseline
