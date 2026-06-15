# Appendix — Benchmarking with Locust

Substrate ships a benchmarking suite under `benchmarking/` in the upstream repo. It pairs a [Locust](https://locust.io/) load generator with a Prometheus + Grafana stack to drive traffic against `ate-api` and the counter demo, and to visualize the resulting RPS / latency / resource use.

This is **not part of the main workshop path** — use it when you want to push Substrate hard and see what breaks first.

## Lab Objectives

- Install Prometheus + Grafana in the `monitoring` namespace
- Generate Python gRPC stubs from the `ateapi.proto`
- Deploy workload templates the benchmark needs
- Build and deploy the Locust workers (with or without the Locust web UI)
- View results in Grafana and (for the `all` profile) the Locust UI

## Prerequisites

- [040 — Substrate installed](040-install-substrate-helm.md) (or [appendix-install-script-alternative](appendix-install-script-alternative.md))
- [020 — `.ate-dev-env.sh` sourced](020-configure-env.md)
- Python 3 + a venv for generating the Python proto clients
- `ko` (for the Locust container image build, via `KO_DOCKER_REPO`)
- Docker (the build pushes `locust-test:latest` to your registry)

## Components (in the cloned `substrate/` repo)

| Path | Purpose |
|---|---|
| `benchmarking/monitoring.yaml` | Prometheus `v2.45.0` + Grafana `10.0.0` in the `monitoring` namespace |
| `benchmarking/locust/` | Locust Python harness — `requirements.txt`, `Dockerfile`, generated `ateapi_pb2*.py` gRPC stubs, tests (`ate_api.py`, `counter_demo.py`, `kernelmem.py`, `sleep.py`, `usermem.py`), shapes (`burst_shape.py`), manifest templates |
| `benchmarking/locust/generate_protos.sh` | Regenerates the Python proto stubs from the `ateapi.proto` |
| `benchmarking/locust/build_and_push.sh` | Builds and pushes the `locust-test:latest` image to `KO_DOCKER_REPO` |
| `benchmarking/locust/deploy_locust.sh` | Applies the Locust worker manifests; controlled by `LOAD_TYPE` env var |
| `benchmarking/workloads/` | `deploy.sh` + workload YAML templates for the scale benchmarks |
| `monitoring/dashboards/ate-grpc-dashboard.json` | Grafana dashboard for `ate-api` gRPC metrics |

## 1. Rebuild the Python gRPC Stubs

From the upstream `substrate/` repo root:

```bash
cd benchmarking
python3 -m venv venv
source venv/bin/activate
pip install -r locust/requirements.txt
./locust/generate_protos.sh
```

You only need to regenerate when the upstream `ateapi.proto` changes.

## 2. Install Monitoring

This must be done **first** — it creates the `monitoring` namespace that the Locust manifests deploy into.

```bash
kubectl apply -f monitoring.yaml
```

Confirm:

```bash
kubectl get pods -n monitoring
```

You should see `prometheus` and `grafana`.

## 3. Deploy Workloads for the Benchmarks

The benchmarks need their target `ActorTemplate`s to exist. There are two flavors:

### Scale workloads (for `LOAD_TYPE=ate-api` or `LOAD_TYPE=all`)

The scale benchmarks (`kernelmem`, `sleep`, `usermem`) need the scale workload templates:

```bash
./workloads/deploy.sh --deploy
# To remove later:
./workloads/deploy.sh --delete
```

### Counter demo (for `LOAD_TYPE=counter`)

The counter benchmark uses the counter `ActorTemplate` from [050](050-counter-demo.md):

```bash
./hack/install-ate.sh --deploy-demo-counter
# To remove later:
./hack/install-ate.sh --delete-demo-counter
```

> **Source the env file** in your shell before running these — both deploy scripts need `BUCKET_NAME`, `PROJECT_ID`, etc.

## 4. Build and Deploy the Locust Worker

```bash
./locust/build_and_push.sh
LOAD_TYPE=all ./locust/deploy_locust.sh
```

`LOAD_TYPE` values:

| Value | Behavior |
|---|---|
| `all` (default) | Deploys all tests **with the Locust web UI enabled** |
| `ate-api` | Deploys only the standalone ate-api load test (headless) |
| `counter` | Deploys only the standalone counter demo load test (headless) |

The deploy script applies a Locust manifest from `benchmarking/locust/manifests/` that pulls the image you pushed in `build_and_push.sh` (it's templated to `${KO_DOCKER_REPO}/.../locust-test:latest`).

## 5. View Results

### Grafana

```bash
kubectl port-forward svc/grafana -n monitoring 3000:3000
```

Open <http://localhost:3000>. Default credentials and dashboards depend on the `monitoring.yaml` shipped — read it.

Import the Substrate dashboard at `monitoring/dashboards/ate-grpc-dashboard.json` (Grafana → Dashboards → Import → paste JSON → set the Prometheus data source).

### Locust Web UI (for `LOAD_TYPE=all`)

```bash
kubectl port-forward svc/locust-all -n monitoring 8089:8089
```

Open <http://localhost:8089>. Configure the load shape (users, ramp, duration) and click "Start swarming". The headless variants (`LOAD_TYPE=ate-api` and `LOAD_TYPE=counter`) skip the UI and run a preconfigured load.

## What to Watch For

| Signal | Where | What it means |
|---|---|---|
| `rpc_server_call_duration_seconds_bucket` p99 climbing | Grafana | `ate-api` is saturating |
| Failed actor resumes | Locust failure rate | snapshot fetch from GCS is slow, or `atelet` is dropping work |
| Worker pool exhaustion | `kubectl ate get workers` | All workers `ASSIGNED`; new resumes block until something suspends |
| `valkey` CPU / memory | Grafana (if you have node-exporter) | Valkey is the state-store bottleneck |
| GCS `5xx` rate | Cloud Monitoring | You're rate-limited at the bucket layer |

## Future (Per Upstream)

The upstream README explicitly calls out that running discrete load tests + storing results in a database is **future work**. Today the suite is good for "drive load and stare at Grafana"; it doesn't archive runs for trend comparison yet.

## Related

- [090 — Observability](090-observability.md) — the standard logs / metrics / traces lab
- [050 — Counter Demo](050-counter-demo.md) — the counter `ActorTemplate` is what `LOAD_TYPE=counter` targets
- Upstream: [benchmarking/README.md](https://github.com/agent-substrate/substrate/blob/main/benchmarking/README.md)
