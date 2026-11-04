# OAuth for Models, MCP Servers, and Agents

Put OAuth on all three sides of an agent: who may call it, and which identity it presents to its model and its MCP server.

- **Inbound.** kagent Enterprise validates the caller's OIDC access token on the agent's A2A endpoint. Keycloak issues the token.
- **Outbound as the Actor (`actorJwt`).** Substrate mints a short-lived JWT for the Actor running the conversation. The egress gateway adds it to every model or MCP request bound for that destination.
- **Outbound as the user (`tokenPassthrough`).** kagent stores the caller's token per Session. The egress gateway adds the caller's own token to the request, so the upstream sees the user, not the agent.

An agentgateway sits in front of OpenAI and an MCP server and validates both kinds of token. It holds the OpenAI key itself. The agent runtime never holds a credential: it sends a placeholder, and the egress gateway replaces the header on the way out.

```
              Bearer $TOKEN (Keycloak)
  curl / UI ──────────────────────────► kagent controller, A2A :8083
                                          checks iss, and aud = kagent-enterprise-api
                                          stores the caller token for the Session
                                                   │
                                                   ▼
                                          Actor on a Worker
                                          sends only placeholder credentials
                                                   │ HTTPS
                                                   ▼
                                          atenet-egress (ate-system)
                        jwt-assistant:    adds the Actor JWT
                passthrough-assistant:    adds the caller's Keycloak token
                                                   │ HTTPS, lab CA
                                                   ▼
                                  agentgateway (agentgateway namespace)
                         :8443 actor-jwt     validates Actor JWTs,    aud = agentgateway
                         :443  user-token    validates Keycloak JWTs, aud = agentgateway
                                     │                         │
                                     ▼                         ▼
                             api.openai.com             mcp-everything
                             (key added here)           (echo, get-sum)
```

## Lab Objectives

- Run Keycloak in the cluster with a `kagent` realm, a demo user, and the clients kagent needs
- Turn on kagent Enterprise OIDC and token propagation, and register kagent's caller-token provider with Substrate's egress gateway
- Deploy an agentgateway that validates Actor JWTs on one listener and Keycloak tokens on the other, in front of OpenAI and an MCP server
- Create two agents: `jwt-assistant` uses `actorJwt` for its model and MCP server, and `passthrough-assistant` uses `tokenPassthrough` for both
- Call both agents over A2A with a bearer token, then read who agentgateway saw in its access log
- Sign in to the UI through Keycloak

## Prerequisites

- An **rc2 baseline**. This lab uses kagent Enterprise `1.0.0-rc2` and Substrate `1.0.0-rc.1-76ace06f`. If the cluster runs the alpha baseline, tear it down with [099](099-cleanup.md) first. Then run [002](002-install.md) with these changes:

  | 002 step | Change |
  |---|---|
  | [1. Set the Versions](002-install.md#1-set-the-versions) | `export KAGENT_VERSION=1.0.0-rc2`<br>`export SUBSTRATE_VERSION=1.0.0-rc.1-76ace06f`<br>`export SUBSTRATE_API_VERSION=1.0.0-rc1`<br>`export KAGENT_OSS_VERSION=v1.0.0-rc1` |
  | [8. Deploy PostgreSQL](002-install.md#8-deploy-postgresql) | The manifest moved. Download `https://raw.githubusercontent.com/kagent-dev/kagent/${KAGENT_OSS_VERSION}/go/core/pkg/install/bundled_postgres.yaml`. The `envsubst` variable list, both `setup.sql` scripts, and the `enterprise` schema step are unchanged. |
  | [11. Install kagent Enterprise](002-install.md#11-install-kagent-enterprise) | The worker image is renamed: `workerImage: ${SUBSTRATE_IMAGE_REPO}/ateworker-gvisor:v${SUBSTRATE_VERSION}` |

  Every other step runs as written. Keep `kagent-values.yaml` and `substrate-values.yaml` in the working directory: this lab layers its own values files on top of them.
- The `kagent-openai` Secret from [002 step 9](002-install.md#9-create-the-license-and-provider-secrets). agentgateway reuses that key.
- `kubectl`, `helm` v3, `curl`, `jq`, `openssl`, `envsubst`, `uuidgen`
- Pods can pull `quay.io/keycloak/keycloak`, `cr.agentgateway.dev/agentgateway`, `curlimages/curl`, `node:24-alpine`, and `alpine`. The MCP server also downloads its package from `registry.npmjs.org` when it starts.
- Local ports `8080`, `8083`, `8180`, `8443`, and `9443` are free

> **[010](010-create-an-agent.md) is not required.** This lab creates its own Harness, `kagent-oauth`, so it does not touch 010's `kagent` Harness or `assistant` Agent.

## What Gets Installed

| Object | Namespace | Purpose |
|---|---|---|
| Keycloak Deployment, Service, realm and admin Secrets | `keycloak` | The OIDC issuer for users |
| `kagent-enterprise-oidc-secret` | `kagent` | The `kagent-backend` client secret |
| `kagent` Helm release, upgraded | `kagent` | OIDC, token propagation, and the Substrate client certificate turned on |
| `substrate` Helm release, upgraded | `ate-system` | Registers the `caller-token.kagent.dev` credential provider |
| `lab-upstream-ca` ConfigMap, and a patch on `atenet-egress` | `ate-system` | The egress gateway trusts the lab CA |
| `mcp-everything` Deployment and Service | `agentgateway` | The MCP reference server |
| agentgateway Deployment, Service, ConfigMap, TLS and OpenAI Secrets | `agentgateway` | Validates tokens and fronts OpenAI and the MCP server |
| ModelConfigs `openai-actor-jwt`, `openai-user-token` | `kagent` | Models through agentgateway |
| RemoteMCPServers `everything-actor-jwt`, `everything-user-token` | `kagent` | The MCP server through agentgateway |
| Harness `kagent-oauth`, AgentTemplates and Agents `jwt-assistant`, `passthrough-assistant` | `kagent` | The two agents |

## 1. Set the Lab Variables

```bash
export DEMO_USER=alice
export DEMO_PASSWORD=$(openssl rand -hex 16)
export OIDC_CLIENT_SECRET=$(openssl rand -hex 16)
export KEYCLOAK_ADMIN_PASSWORD=$(openssl rand -hex 16)
```

The commands below send these into Secrets without writing them to disk. Every step uses this shell, so keep it open.

## 2. Deploy Keycloak

Write the realm template. It holds placeholders, not values:

```bash
cat > kagent-realm.json.tmpl <<'EOF'
{
  "realm": "kagent",
  "enabled": true,
  "accessTokenLifespan": 1800,
  "groups": [
    { "name": "kagent-users" }
  ],
  "users": [
    {
      "username": "${DEMO_USER}",
      "enabled": true,
      "email": "${DEMO_USER}@example.com",
      "emailVerified": true,
      "firstName": "Demo",
      "lastName": "User",
      "credentials": [
        { "type": "password", "value": "${DEMO_PASSWORD}", "temporary": false }
      ],
      "groups": ["/kagent-users"]
    }
  ],
  "clients": [
    {
      "clientId": "kagent-backend",
      "enabled": true,
      "publicClient": false,
      "secret": "${OIDC_CLIENT_SECRET}",
      "standardFlowEnabled": true,
      "directAccessGrantsEnabled": false,
      "redirectUris": ["http://localhost:8080/*", "http://127.0.0.1:8080/*"],
      "webOrigins": ["+"]
    },
    {
      "clientId": "kagent-ui",
      "enabled": true,
      "publicClient": true,
      "standardFlowEnabled": true,
      "directAccessGrantsEnabled": true,
      "redirectUris": ["http://localhost:8080/*", "http://127.0.0.1:8080/*"],
      "webOrigins": ["+"],
      "attributes": {
        "pkce.code.challenge.method": "S256",
        "post.logout.redirect.uris": "+"
      },
      "protocolMappers": [
        {
          "name": "groups",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-group-membership-mapper",
          "config": {
            "claim.name": "Groups",
            "full.path": "false",
            "id.token.claim": "true",
            "access.token.claim": "true",
            "userinfo.token.claim": "true"
          }
        },
        {
          "name": "kagent-api-audience",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-audience-mapper",
          "config": {
            "included.custom.audience": "kagent-enterprise-api",
            "access.token.claim": "true",
            "id.token.claim": "false"
          }
        },
        {
          "name": "agentgateway-audience",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-audience-mapper",
          "config": {
            "included.custom.audience": "agentgateway",
            "access.token.claim": "true",
            "id.token.claim": "false"
          }
        }
      ]
    }
  ]
}
EOF
```

What the realm sets up:

- `kagent-backend` is the confidential client the kagent controller uses.
- `kagent-ui` is the public client the UI signs in with (PKCE). Password grants are on, so the CLI steps below can get a token without a browser.
- `kagent-ui` tokens carry two audiences. `kagent-enterprise-api` is the audience kagent Enterprise accepts by default, and `agentgateway` is the audience the gateway requires. They also carry a `Groups` claim.
- Access tokens last 30 minutes.

Create the namespace and Secrets, filling in the template on the way:

```bash
kubectl create namespace keycloak

kubectl create secret generic keycloak-admin -n keycloak \
  --from-literal=password="${KEYCLOAK_ADMIN_PASSWORD:?run step 1}"

envsubst '${DEMO_USER} ${DEMO_PASSWORD} ${OIDC_CLIENT_SECRET}' < kagent-realm.json.tmpl \
  | kubectl create secret generic keycloak-realm -n keycloak --from-file=kagent-realm.json=/dev/stdin
```

Deploy Keycloak:

```bash
kubectl apply -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: keycloak
  namespace: keycloak
spec:
  replicas: 1
  selector:
    matchLabels:
      app: keycloak
  template:
    metadata:
      labels:
        app: keycloak
    spec:
      containers:
      - name: keycloak
        image: quay.io/keycloak/keycloak:26.8.1
        args: ["start-dev", "--import-realm"]
        env:
        - name: KC_BOOTSTRAP_ADMIN_USERNAME
          value: admin
        - name: KC_BOOTSTRAP_ADMIN_PASSWORD
          valueFrom:
            secretKeyRef:
              name: keycloak-admin
              key: password
        - name: KC_HEALTH_ENABLED
          value: "true"
        - name: KC_HOSTNAME
          value: http://keycloak.keycloak.svc.cluster.local:8180
        ports:
        - name: http
          containerPort: 8080
        - name: management
          containerPort: 9000
        readinessProbe:
          httpGet:
            path: /health/ready
            port: management
          initialDelaySeconds: 20
          periodSeconds: 5
          failureThreshold: 60
        volumeMounts:
        - name: realm
          mountPath: /opt/keycloak/data/import
          readOnly: true
      volumes:
      - name: realm
        secret:
          secretName: keycloak-realm
---
apiVersion: v1
kind: Service
metadata:
  name: keycloak
  namespace: keycloak
spec:
  selector:
    app: keycloak
  ports:
  - name: http
    port: 8180
    targetPort: http
EOF

kubectl rollout status deploy/keycloak -n keycloak --timeout=10m
```

> **`KC_HOSTNAME` fixes the issuer.** Every token's `iss` is `http://keycloak.keycloak.svc.cluster.local:8180/realms/kagent`, whatever address you reach Keycloak on. A token fetched through a laptop port-forward therefore matches the issuer the kagent controller and agentgateway validate in-cluster.

> **`start-dev` is for this lab only.** It keeps state in memory and serves plain HTTP. A pod restart re-imports the realm, and the demo user gets a new `sub`.

## 3. Get a User Token

In a second terminal, forward Keycloak and leave it running:

```bash
kubectl port-forward -n keycloak svc/keycloak 8180:8180
```

Back in the lab shell, define a helper that runs a password grant on `kagent-ui`, then get a token:

```bash
kc_token() {
  curl -fsS http://127.0.0.1:8180/realms/kagent/protocol/openid-connect/token \
    -d grant_type=password -d client_id=kagent-ui -d scope=openid \
    --data-urlencode username="$DEMO_USER" --data-urlencode password="$DEMO_PASSWORD" \
    | jq -r .access_token
}

jwt_claims() {
  jq -R 'split(".")[1] | gsub("-";"+") | gsub("_";"/") | @base64d | fromjson'
}

export TOKEN=$(kc_token)
echo "$TOKEN" | jwt_claims | jq '{iss, sub, aud, azp, Groups, preferred_username, exp}'
```

Expected:

```json
{
  "iss": "http://keycloak.keycloak.svc.cluster.local:8180/realms/kagent",
  "sub": "<a UUID>",
  "aud": ["kagent-enterprise-api", "agentgateway"],
  "azp": "kagent-ui",
  "Groups": ["kagent-users"],
  "preferred_username": "alice",
  "exp": 1793811312
}
```

The token expires 30 minutes after it is issued. Rerun `export TOKEN=$(kc_token)` whenever a later step needs a fresh one.

## 4. Turn On OIDC and Token Propagation

kagent needs three changes:

- `enterprise.oidc` points the controller and UI at Keycloak. This replaces the bundled demo login.
- `enterprise.tokenPropagation` stores each caller's token per Session and serves it to the egress gateway through kagent's credential provider, `kagent-credential-provider:8086`.
- `controller.substrateClientCertificate` gives the controller the Substrate client certificate that token propagation requires.

Create the client Secret, then upgrade kagent with a values file layered over 002's:

```bash
kubectl create secret generic kagent-enterprise-oidc-secret -n kagent \
  --from-literal=clientSecret="${OIDC_CLIENT_SECRET:?run step 1}"

cat > kagent-oauth-values.yaml <<'EOF'
controller:
  substrateClientCertificate:
    enabled: true

enterprise:
  oidc:
    issuer: http://keycloak.keycloak.svc.cluster.local:8180/realms/kagent
    clientId: kagent-backend
    secretRef: kagent-enterprise-oidc-secret
    secretKey: clientSecret
  tokenPropagation:
    enabled: true
EOF

helm upgrade kagent \
  oci://us-docker.pkg.dev/solo-public/kagent-enterprise-helm/charts/kagent-enterprise \
  --version $KAGENT_VERSION --namespace kagent \
  -f kagent-values.yaml -f kagent-oauth-values.yaml --wait --timeout 15m
```

> **The UI client and the accepted audience are chart defaults.** The UI signs in as `kagent-ui` and returns to `/callback`. The API accepts tokens whose `aud` includes `kagent-enterprise-api`. The realm in step 2 matches both, so neither appears in the values file.

Register kagent's credential provider with Substrate's egress gateway under the `caller-token.kagent.dev` authority, again layered over 002's values:

```bash
cat > substrate-oauth-values.yaml <<'EOF'
enterprise:
  credentialProvider:
    extraProviders:
    - uriAuthority: caller-token.kagent.dev
      host: kagent-credential-provider.kagent.svc:8086
EOF

helm upgrade substrate oci://us-docker.pkg.dev/solo-public/enterprise-substrate-helm/substrate \
  --version $SUBSTRATE_VERSION --namespace ate-system \
  -f substrate-values.yaml -f substrate-oauth-values.yaml

kubectl rollout status deploy/atenet-egress -n ate-system --timeout=300s
```

Confirm both sides:

```bash
kubectl get svc kagent-credential-provider -n kagent
kubectl get configmap -n ate-system -o yaml | grep -c caller-token.kagent.dev
```

The Service exists on port `8086`, and the count is at least `1`.

## 5. Create the Lab CA and Trust It at Egress

Passthrough targets must be HTTPS, and this lab uses HTTPS for both paths. The egress gateway terminates the Actor's TLS and opens a new TLS connection to the destination, which it verifies against `/etc/ssl/certs/ca-certificates.crt`. agentgateway's certificate comes from a lab CA, so that CA has to be in the bundle. The runtimes need no change, because they already trust the egress gateway.

Create the CA and agentgateway's server certificate:

```bash
openssl req -x509 -sha256 -days 30 -nodes \
  -newkey ec -pkeyopt ec_paramgen_curve:P-256 -pkeyopt ec_param_enc:named_curve \
  -subj "/CN=kagent oauth lab CA" -keyout lab-ca.key -out lab-ca.crt \
  -addext "basicConstraints=critical,CA:TRUE" -addext "keyUsage=critical,keyCertSign,cRLSign"

openssl req -new -sha256 -nodes \
  -newkey ec -pkeyopt ec_paramgen_curve:P-256 -pkeyopt ec_param_enc:named_curve \
  -subj "/CN=agentgateway.agentgateway.svc.cluster.local" \
  -keyout agentgateway.key -out agentgateway.csr

cat > agentgateway.ext <<'EOF'
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature
extendedKeyUsage=serverAuth
subjectAltName=DNS:agentgateway.agentgateway.svc.cluster.local,DNS:agentgateway.agentgateway.svc,DNS:agentgateway.agentgateway,DNS:agentgateway
EOF

openssl x509 -req -sha256 -days 30 -in agentgateway.csr \
  -CA lab-ca.crt -CAkey lab-ca.key -CAcreateserial \
  -extfile agentgateway.ext -out agentgateway.crt
```

> **Keep `ec_param_enc:named_curve`.** macOS ships LibreSSL, which otherwise writes the EC parameters out in full. agentgateway rejects such a key at startup with `keys may not be consistent: KeyMismatch`. OpenSSL 3 accepts the flag too.

Publish the CA to `ate-system`, then patch the egress gateway. An init container concatenates the system bundle with the lab CA, and the result is mounted over the bundle file in the gateway's `agentgateway` container:

```bash
kubectl create configmap lab-upstream-ca -n ate-system --from-file=ca.crt=lab-ca.crt

CA_SHA=$(openssl dgst -sha256 < lab-ca.crt | awk '{print $NF}')

kubectl patch deploy atenet-egress -n ate-system --type strategic -p "$(cat <<EOF
{"spec": {"template": {
  "metadata": {"annotations": {"lab.kagent.dev/upstream-ca-sha256": "${CA_SHA}"}},
  "spec": {
    "initContainers": [{
      "name": "lab-roots",
      "image": "alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6",
      "command": ["sh", "-c", "cat /etc/ssl/certs/ca-certificates.crt /run/lab-ca/ca.pem > /roots/ca-certificates.crt"],
      "securityContext": {"runAsUser": 65532, "runAsNonRoot": true, "allowPrivilegeEscalation": false},
      "volumeMounts": [
        {"name": "lab-upstream-ca", "mountPath": "/run/lab-ca", "readOnly": true},
        {"name": "lab-roots", "mountPath": "/roots"}
      ]
    }],
    "containers": [{"name": "agentgateway", "volumeMounts": [
      {"name": "lab-roots", "mountPath": "/etc/ssl/certs/ca-certificates.crt", "subPath": "ca-certificates.crt", "readOnly": true}
    ]}],
    "volumes": [
      {"name": "lab-upstream-ca", "configMap": {"name": "lab-upstream-ca", "items": [{"key": "ca.crt", "path": "ca.pem"}]}},
      {"name": "lab-roots", "emptyDir": {}}
    ]
  }
}}}
EOF
)"

kubectl rollout status deploy/atenet-egress -n ate-system --timeout=300s
```

The patch merges by name, so running it again changes nothing. The annotation restarts the pods when the CA changes.

> **Rerun this step if the Deployment is replaced.** A Substrate uninstall and reinstall, or a `kubectl replace`, drops the patch, and model calls through agentgateway then fail certificate verification at egress.

## 6. Deploy the MCP Server

The MCP reference server, `server-everything`, serves Streamable HTTP at `/mcp` on port `3001`. It runs through `npx`, so the pod downloads the package from npm when it starts.

```bash
kubectl create namespace agentgateway

kubectl apply -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mcp-everything
  namespace: agentgateway
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mcp-everything
  template:
    metadata:
      labels:
        app: mcp-everything
    spec:
      containers:
      - name: server
        image: node:24-alpine
        command: ["npx", "-y", "@modelcontextprotocol/server-everything@2.1.0", "streamableHttp"]
        env:
        - name: PORT
          value: "3001"
        ports:
        - name: mcp
          containerPort: 3001
        readinessProbe:
          tcpSocket:
            port: mcp
          periodSeconds: 5
          failureThreshold: 60
---
apiVersion: v1
kind: Service
metadata:
  name: mcp-everything
  namespace: agentgateway
spec:
  selector:
    app: mcp-everything
  ports:
  - name: mcp
    port: 3001
    targetPort: mcp
EOF

kubectl rollout status deploy/mcp-everything -n agentgateway --timeout=5m
kubectl logs -n agentgateway deploy/mcp-everything | tail -2
```

The log ends with `MCP Streamable HTTP Server listening on port 3001`. The server has no authentication of its own. Only agentgateway reaches it.

## 7. Deploy agentgateway

agentgateway runs two HTTPS listeners on one certificate. Each listener accepts one issuer:

| Listener | Service port | Accepts | Used by |
|---|---|---|---|
| `actor-jwt` | `8443` | Actor JWTs from `https://idp.ate-system.svc`, `aud` = `agentgateway` | `jwt-assistant` |
| `user-token` | `443` | Keycloak tokens from the `kagent` realm, `aud` = `agentgateway` | `passthrough-assistant` |

Each listener has two routes: `/mcp` goes to the MCP server, and everything else goes to OpenAI with the API key added. `jwtAuth` runs in `strict` mode on every route, so a request without a valid token gets a 401. agentgateway removes the validated token before the request leaves for OpenAI or the MCP server.

Create the TLS Secret, and copy the OpenAI key from `kagent-openai` without printing it:

```bash
kubectl create secret tls agentgateway-tls -n agentgateway \
  --cert=agentgateway.crt --key=agentgateway.key

kubectl get secret kagent-openai -n kagent -o jsonpath='{.data.OPENAI_API_KEY}' | base64 -d \
  | kubectl create secret generic agentgateway-openai -n agentgateway --from-file=OPENAI_API_KEY=/dev/stdin
```

Apply the configuration, the Deployment, and the Service:

```bash
kubectl apply -f - <<'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: agentgateway-config
  namespace: agentgateway
data:
  config.yaml: |
    # yaml-language-server: $schema=https://agentgateway.dev/schema/config
    frontendPolicies:
      accessLog:
        add:
          jwt.iss: jwt.iss
          jwt.sub: jwt.sub
          jwt.aud: jwt.aud

    gateways:
      actor-jwt:
        port: 8443
        tls:
          cert: /tls/tls.crt
          key: /tls/tls.key
      user-token:
        port: 9443
        tls:
          cert: /tls/tls.crt
          key: /tls/tls.key

    routes:
    - name: actor-jwt-mcp
      gateways: actor-jwt
      matches:
      - path:
          pathPrefix: /mcp
      policies:
        jwtAuth:
          mode: strict
          issuer: https://idp.ate-system.svc
          audiences: [agentgateway]
          jwks:
            file: /jwks/actor-jwks.json
      backends:
      - mcp:
          targets:
          - name: everything
            mcp:
              host: http://mcp-everything.agentgateway.svc.cluster.local:3001/mcp

    - name: actor-jwt-openai
      gateways: actor-jwt
      policies:
        jwtAuth:
          mode: strict
          issuer: https://idp.ate-system.svc
          audiences: [agentgateway]
          jwks:
            file: /jwks/actor-jwks.json
        backendAuth:
          key: "$OPENAI_API_KEY"
      backends:
      - ai:
          name: openai
          provider:
            openAI: {}

    - name: user-token-mcp
      gateways: user-token
      matches:
      - path:
          pathPrefix: /mcp
      policies:
        jwtAuth:
          mode: strict
          issuer: http://keycloak.keycloak.svc.cluster.local:8180/realms/kagent
          audiences: [agentgateway]
          jwks:
            url: http://keycloak.keycloak.svc.cluster.local:8180/realms/kagent/protocol/openid-connect/certs
      backends:
      - mcp:
          targets:
          - name: everything
            mcp:
              host: http://mcp-everything.agentgateway.svc.cluster.local:3001/mcp

    - name: user-token-openai
      gateways: user-token
      policies:
        jwtAuth:
          mode: strict
          issuer: http://keycloak.keycloak.svc.cluster.local:8180/realms/kagent
          audiences: [agentgateway]
          jwks:
            url: http://keycloak.keycloak.svc.cluster.local:8180/realms/kagent/protocol/openid-connect/certs
        backendAuth:
          key: "$OPENAI_API_KEY"
      backends:
      - ai:
          name: openai
          provider:
            openAI: {}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: agentgateway
  namespace: agentgateway
spec:
  replicas: 1
  selector:
    matchLabels:
      app: agentgateway
  template:
    metadata:
      labels:
        app: agentgateway
    spec:
      initContainers:
      - name: fetch-actor-jwks
        image: curlimages/curl:8.23.0
        command: ["sh", "-c"]
        args:
        - |
          set -eu
          ca=/run/ate-trust/trust-bundle.pem
          jwks_uri=$(curl -fsS --cacert "$ca" https://idp.ate-system.svc/.well-known/openid-configuration \
            | grep -o '"jwks_uri" *: *"[^"]*"' | cut -d'"' -f4)
          curl -fsS --cacert "$ca" -o /jwks/actor-jwks.json "$jwks_uri"
        volumeMounts:
        - name: ate-trust
          mountPath: /run/ate-trust
          readOnly: true
        - name: jwks
          mountPath: /jwks
      containers:
      - name: agentgateway
        image: cr.agentgateway.dev/agentgateway:v1.7.0
        args: ["-f", "/config/config.yaml"]
        env:
        - name: OPENAI_API_KEY
          valueFrom:
            secretKeyRef:
              name: agentgateway-openai
              key: OPENAI_API_KEY
        ports:
        - name: actor-jwt
          containerPort: 8443
        - name: user-token
          containerPort: 9443
        volumeMounts:
        - name: config
          mountPath: /config
          readOnly: true
        - name: tls
          mountPath: /tls
          readOnly: true
        - name: jwks
          mountPath: /jwks
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: agentgateway-config
      - name: tls
        secret:
          secretName: agentgateway-tls
      - name: jwks
        emptyDir: {}
      - name: ate-trust
        projected:
          sources:
          - clusterTrustBundle:
              signerName: servicedns.podcert.ate.dev/identity
              labelSelector:
                matchLabels:
                  podcert.ate.dev/canarying: live
              path: trust-bundle.pem
---
apiVersion: v1
kind: Service
metadata:
  name: agentgateway
  namespace: agentgateway
spec:
  selector:
    app: agentgateway
  ports:
  - name: user-token
    port: 443
    targetPort: user-token
  - name: actor-jwt
    port: 8443
    targetPort: actor-jwt
EOF

kubectl rollout status deploy/agentgateway -n agentgateway --timeout=5m
```

How the two key sets arrive:

- **Keycloak keys** come from the realm's JWKS URL over plain HTTP inside the cluster. agentgateway fetches them itself.
- **Actor JWT keys** come from Substrate's IdP, `https://idp.ate-system.svc`. Its certificate is signed by the `servicedns.podcert.ate.dev` CA, so the init container mounts that `ClusterTrustBundle`, reads `jwks_uri` from the discovery document, and saves the key set to `/jwks/actor-jwks.json`.

> **Restart agentgateway if the Substrate IdP rotates its keys.** The Actor key set is read once, at pod start. After a rotation, Actor JWT routes return 401 and the log says `token uses the unknown key`. Run `kubectl rollout restart deploy/agentgateway -n agentgateway`.

## 8. Check agentgateway from Your Laptop

In another terminal, forward both listeners:

```bash
kubectl port-forward -n agentgateway svc/agentgateway 9443:443 8443:8443
```

Back in the lab shell, send an MCP `initialize` three ways. `--resolve` keeps the certificate's DNS name while connecting to the port-forward:

```bash
AGW=agentgateway.agentgateway.svc.cluster.local
INIT='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"1"}}}'

mcp_init() {
  curl -sS -o /dev/null -w "%{http_code}\n" --cacert lab-ca.crt \
    --resolve "$AGW:$1:127.0.0.1" "https://$AGW:$1/mcp" \
    -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
    ${2:+-H "Authorization: Bearer $2"} -d "$INIT"
}

mcp_init 9443                # no token
mcp_init 9443 "$TOKEN"       # Keycloak token on the user-token listener
mcp_init 8443 "$TOKEN"       # Keycloak token on the actor-jwt listener
```

Expected: `401`, `200`, `401`. The `actor-jwt` listener only accepts tokens from Substrate's issuer, so the Keycloak token fails there. No Actor JWT exists outside an Actor, so the agents test that listener in step 11.

Then check the OpenAI route on the `user-token` listener. agentgateway adds the API key:

```bash
curl -sS --cacert lab-ca.crt --resolve "$AGW:9443:127.0.0.1" "https://$AGW:9443/v1/chat/completions" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4.1-mini","messages":[{"role":"user","content":"Say hello in three words."}]}' \
  | jq -r '.choices[0].message.content // .error'
```

The model's reply prints. You can stop the port-forward now.

## 9. Create the ModelConfigs and RemoteMCPServers

Each path gets one ModelConfig and one RemoteMCPServer. Neither references a Secret: the credentials come from Substrate and the caller.

```bash
kubectl apply -f - <<'EOF'
apiVersion: api.kagent.dev/v1alpha3
kind: ModelConfig
metadata:
  name: openai-actor-jwt
  namespace: kagent
spec:
  provider: OpenAI
  model: gpt-4.1-mini
  actorJwt:
    audiences: [agentgateway]
  openAI:
    baseUrl: https://agentgateway.agentgateway.svc.cluster.local:8443/v1
---
apiVersion: api.kagent.dev/v1alpha3
kind: RemoteMCPServer
metadata:
  name: everything-actor-jwt
  namespace: kagent
spec:
  description: MCP reference server through agentgateway, authenticated as the Actor
  url: https://agentgateway.agentgateway.svc.cluster.local:8443/mcp
  actorJwt:
    audiences: [agentgateway]
---
apiVersion: api.kagent.dev/v1alpha3
kind: ModelConfig
metadata:
  name: openai-user-token
  namespace: kagent
spec:
  provider: OpenAI
  model: gpt-4.1-mini
  tokenPassthrough: true
  openAI:
    baseUrl: https://agentgateway.agentgateway.svc.cluster.local/v1
---
apiVersion: api.kagent.dev/v1alpha3
kind: RemoteMCPServer
metadata:
  name: everything-user-token
  namespace: kagent
  labels:
    kagent.dev/discovery: disabled
spec:
  description: MCP reference server through agentgateway, authenticated as the caller
  url: https://agentgateway.agentgateway.svc.cluster.local/mcp
  tokenPassthrough: true
EOF

kubectl get modelconfig,remotemcpserver -n kagent
```

What each setting does:

- **`actorJwt.audiences`** sets the `aud` of the Actor JWT. The egress gateway sends it as `Authorization: Bearer <jwt>` to the ModelConfig or RemoteMCPServer origin. The token's `sub` is `actor/<atespace>/<actor>`, for example `actor/kagent/session-<session id>`. It lasts 900 seconds by default (`expirationSeconds`, 300 to 3600), and the gateway refreshes it on its own. `kagent-runtime` is a reserved audience.
- **`tokenPassthrough: true`** sends the caller's token. The runtime only holds the placeholder `kagent-credential-injected`. The egress gateway asks kagent's `caller-token.kagent.dev` provider for the real token and puts it in the header.
- **`kagent.dev/discovery: disabled`** stops the controller from listing the passthrough server's tools. The controller has no caller token to list them with. Tool discovery runs inside the Actor instead. The controller turns discovery off by itself for `actorJwt` servers.

> **One credential per origin and header.** The model and the MCP server on each path share an origin, for example `https://agentgateway.agentgateway.svc.cluster.local:8443`. That works because their credentials are identical. If two objects send different credentials to the same origin and header, for example different `actorJwt` audiences, the agent is rejected with `conflicting credentials`. Use a different port or hostname per credential.

## 10. Create the Harness, AgentTemplates, and Agents

The Harness omits `workload.image`, so it runs the release's built-in kagent runtime image.

```bash
kubectl apply -f - <<'EOF'
apiVersion: api.kagent.dev/v1alpha3
kind: Harness
metadata:
  name: kagent-oauth
  namespace: kagent
spec:
  kagent: {}
  substrate:
    workerPoolRef:
      name: kagent-default
    snapshotPolicy:
      location: gs://ate-snapshots/kagent/
---
apiVersion: api.kagent.dev/v1alpha3
kind: AgentTemplate
metadata:
  name: jwt-assistant
  namespace: kagent
spec:
  description: Calls its model and MCP server with an Actor JWT.
  systemPrompt: You are a helpful assistant. Use your tools when asked, and report their exact output.
  modelConfig:
    name: openai-actor-jwt
  tools:
  - mcp:
      server:
        kind: RemoteMCPServer
        name: everything-actor-jwt
      tools: [echo, get-sum]
---
apiVersion: api.kagent.dev/v1alpha3
kind: Agent
metadata:
  name: jwt-assistant
  namespace: kagent
spec:
  template:
    ref:
      name: jwt-assistant
  harnessRef:
    name: kagent-oauth
---
apiVersion: api.kagent.dev/v1alpha3
kind: AgentTemplate
metadata:
  name: passthrough-assistant
  namespace: kagent
spec:
  description: Calls its model and MCP server with the caller's token.
  systemPrompt: You are a helpful assistant. Use your tools when asked, and report their exact output.
  modelConfig:
    name: openai-user-token
  tools:
  - mcp:
      server:
        kind: RemoteMCPServer
        name: everything-user-token
      tools: [echo, get-sum]
---
apiVersion: api.kagent.dev/v1alpha3
kind: Agent
metadata:
  name: passthrough-assistant
  namespace: kagent
spec:
  template:
    ref:
      name: passthrough-assistant
  harnessRef:
    name: kagent-oauth
EOF

kubectl wait agent/jwt-assistant agent/passthrough-assistant -n kagent --for=condition=Ready --timeout=600s
```

`tools: [echo, get-sum]` exposes two of the server's tools. agentgateway does not prefix tool names when it fronts a single target, so the names match the server's own.

## 11. Call the Agents over A2A

In another terminal, forward the controller's A2A port:

```bash
kubectl port-forward -n kagent svc/kagent-controller 8083:8083
```

Define a helper that sends one A2A `SendMessage` with the bearer token and prints the reply:

```bash
a2a() {
  curl -sS --max-time 300 "http://127.0.0.1:8083/agents/kagent/$1" \
    -H "Authorization: Bearer $TOKEN" -H "A2A-Version: 1.0" -H "Content-Type: application/json" \
    -d "$(jq -n --arg id "$(uuidgen | tr '[:upper:]' '[:lower:]')" --arg text "$2" \
      '{jsonrpc: "2.0", id: 1, method: "SendMessage",
        params: {message: {messageId: $id, role: "ROLE_USER", parts: [{text: $text}]}}}')" \
  | jq '.error // (.result | .task // .message | {state: .status.state,
      text: ([.artifacts[]?.parts[]?.text, .status.message.parts[]?.text, .parts[]?.text]
             | map(select(. != null)) | join("\n"))})'
}
```

Call `jwt-assistant`:

```bash
export TOKEN=$(kc_token)
a2a jwt-assistant "Use the echo tool to echo 'hello from jwt-assistant', then use get-sum to add 2 and 3."
```

The first call can take a minute while the Session's Actor starts. Expected: `"state": "TASK_STATE_COMPLETED"`, and text that contains `Echo: hello from jwt-assistant` and `5`.

Call `passthrough-assistant`:

```bash
a2a passthrough-assistant "Use the echo tool to echo 'hello from passthrough-assistant', then use get-sum to add 4 and 5."
```

Expected: completed, with `Echo: hello from passthrough-assistant` and `9`.

Every call went through the same steps:

1. The controller validated `$TOKEN`: the issuer matched `enterprise.oidc.issuer`, the signature matched the realm's keys, and `aud` included `kagent-enterprise-api`.
2. The controller stored the token for the new Session. Only the owner's own `SendMessage` stores it.
3. The Actor called its model and its MCP server with placeholder credentials.
4. The egress gateway replaced the `Authorization` header with the Actor JWT or the stored caller token, and verified agentgateway's certificate against the lab CA.
5. agentgateway validated the token, removed it, and forwarded the request. OpenAI requests got the API key.

## 12. Read What agentgateway Saw

```bash
kubectl logs -n agentgateway deploy/agentgateway --since=15m \
  | grep -E 'route=default/(actor-jwt|user-token)' \
  | sed -E 's/.*(route=[^ ]+).*(http\.status=[0-9]+).*(jwt\.iss="[^"]*").*(jwt\.sub="[^"]*").*/\1 \2 \3 \4/'
```

The filter keeps the route, status, issuer, and subject from each request line. Lines that failed authentication have no `jwt.*` fields and print in full. Drop the `sed` to see every field.

Look for:

| Route | `jwt.iss` | `jwt.sub` |
|---|---|---|
| `actor-jwt-openai`, `actor-jwt-mcp` | `https://idp.ate-system.svc` | `actor/kagent/session-<session id>` |
| `user-token-openai`, `user-token-mcp` | `http://keycloak.keycloak.svc.cluster.local:8180/realms/kagent` | the `sub` of your token |

In the full lines, `jwt.aud` is `["agentgateway"]` for Actor JWTs and lists both Keycloak audiences for user tokens. MCP lines also carry `mcp.method.name`, for example `tools/call`.

Compare the passthrough `sub` with your token's:

```bash
echo "$TOKEN" | jwt_claims | jq -r .sub
```

They match: OpenAI and the MCP server were reached as `alice`, not as the agent. On the Actor JWT path, every user's requests reach agentgateway as that Session's Actor.

## 13. Check the Failures

A2A without a token:

```bash
curl -sS -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8083/agents/kagent/jwt-assistant \
  -H "A2A-Version: 1.0" -H "Content-Type: application/json" -d '{}'
```

Expected: `401`. The agent card also needs a token:

```bash
curl -sS -o /dev/null -w "%{http_code}\n" \
  http://127.0.0.1:8083/agents/kagent/jwt-assistant/.well-known/agent-card.json
curl -sS -H "Authorization: Bearer $TOKEN" \
  http://127.0.0.1:8083/agents/kagent/jwt-assistant/.well-known/agent-card.json | jq '{name, description}'
```

Expected: `401`, then the card.

A token without the `kagent-enterprise-api` audience is refused the same way. Any signed-in user can call either agent, because this lab creates no RBAC policies.

> **Passthrough after the token expires.** The stored caller token is valid until its `exp`, 30 minutes in this realm. After that, the egress gateway refuses the passthrough agent's model and MCP calls with `403`. Get a new token and send another message in that conversation as its owner: the new token replaces the stored one. The Actor JWT path does not depend on the caller's token lifetime.

## 14. Sign In to the UI

The browser follows redirects to the issuer URL, `keycloak.keycloak.svc.cluster.local:8180`, so make that name resolve to your laptop:

```bash
echo "127.0.0.1 keycloak.keycloak.svc.cluster.local" | sudo tee -a /etc/hosts
```

Keep the Keycloak port-forward from step 3 running, and forward the UI in another terminal:

```bash
kubectl port-forward -n kagent svc/kagent-ui 8080:8080
```

Open `http://localhost:8080`. The UI sends you to Keycloak. Sign in as `$DEMO_USER` with the password from step 1 (`echo $DEMO_PASSWORD` in the lab shell). Back in the UI, open `passthrough-assistant` and ask it to echo something. The new user-token lines in the agentgateway log carry your `sub`, because the UI's token is the same kind of `kagent-ui` token.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `helm upgrade kagent` fails, or the controller does not start, after step 4 | `kagent-enterprise-oidc-secret` is missing, or Keycloak is not reachable at the issuer URL | Check the Secret, and `kubectl rollout status deploy/keycloak -n keycloak` |
| A2A returns `401` with a token | The token expired, came from another issuer, or lacks `aud` `kagent-enterprise-api` | `echo "$TOKEN" \| jwt_claims \| jq '{iss, aud, exp}'`, then `export TOKEN=$(kc_token)` |
| `kc_token` prints nothing | The Keycloak port-forward stopped, or the realm import failed | Restart the port-forward. Check `kubectl logs -n keycloak deploy/keycloak \| grep -i import` |
| Keycloak not Ready after several minutes | First start builds and imports the realm | Wait. The readiness probe allows five minutes |
| agentgateway exits with `KeyMismatch` | The EC key was written with explicit parameters | Recreate the key and certificate with `ec_param_enc:named_curve` (step 5), then recreate `agentgateway-tls` |
| `fetch-actor-jwks` init container fails | `idp.ate-system.svc` is unreachable, or the trust bundle is empty | `kubectl logs -n agentgateway deploy/agentgateway -c fetch-actor-jwks`. Check `kubectl get clustertrustbundles \| grep servicedns` |
| Actor JWT routes log `token uses the unknown key` | Substrate's IdP rotated its keys | `kubectl rollout restart deploy/agentgateway -n agentgateway` |
| agentgateway logs `InvalidAudience` | The token lacks `aud` `agentgateway` | Check the `kagent-ui` audience mappers in the realm, or `actorJwt.audiences` |
| The agent fails on model calls and agentgateway logs nothing | Egress cannot verify agentgateway's certificate | Rerun step 5's patch. Check `kubectl get deploy atenet-egress -n ate-system -o jsonpath='{.spec.template.spec.initContainers[*].name}'` includes `lab-roots` |
| Passthrough calls fail with `403` | The stored caller token expired | Get a new token and send another message as the owner |
| Passthrough calls fail and agentgateway logs no user-token lines | Token propagation is not wired | Check step 4: `svc/kagent-credential-provider` exists, and the egress config contains `caller-token.kagent.dev` |
| Agent rejected with `conflicting credentials` | Two objects send different credentials to the same origin and header | Give each credential its own port or hostname |
| `mcp-everything` never becomes Ready | The pod cannot reach `registry.npmjs.org` | Allow egress to npm, or build an image with the package installed |
| The browser cannot open the Keycloak sign-in page | The `/etc/hosts` line or the 8180 port-forward is missing | Add the line from step 14, and keep the Keycloak port-forward running |

## Cleanup

Removes everything this lab created and returns kagent and Substrate to the rc2 baseline values.

```bash
kubectl delete agent jwt-assistant passthrough-assistant -n kagent --ignore-not-found
kubectl delete agenttemplate jwt-assistant passthrough-assistant -n kagent --ignore-not-found
kubectl delete harness kagent-oauth -n kagent --ignore-not-found
kubectl delete modelconfig openai-actor-jwt openai-user-token -n kagent --ignore-not-found
kubectl delete remotemcpserver everything-actor-jwt everything-user-token -n kagent --ignore-not-found
```

Remove the egress trust patch and the CA ConfigMap:

```bash
kubectl patch deploy atenet-egress -n ate-system --type strategic -p '{"spec":{"template":{
  "metadata":{"annotations":{"lab.kagent.dev/upstream-ca-sha256":null}},
  "spec":{
    "initContainers":[{"name":"lab-roots","$patch":"delete"}],
    "containers":[{"name":"agentgateway","volumeMounts":[{"mountPath":"/etc/ssl/certs/ca-certificates.crt","$patch":"delete"}]}],
    "volumes":[{"name":"lab-upstream-ca","$patch":"delete"},{"name":"lab-roots","$patch":"delete"}]
  }}}}'
kubectl rollout status deploy/atenet-egress -n ate-system --timeout=300s
kubectl delete configmap lab-upstream-ca -n ate-system --ignore-not-found
```

Return both releases to the baseline values, then remove the OIDC Secret:

```bash
helm upgrade substrate oci://us-docker.pkg.dev/solo-public/enterprise-substrate-helm/substrate \
  --version $SUBSTRATE_VERSION --namespace ate-system -f substrate-values.yaml

helm upgrade kagent \
  oci://us-docker.pkg.dev/solo-public/kagent-enterprise-helm/charts/kagent-enterprise \
  --version $KAGENT_VERSION --namespace kagent -f kagent-values.yaml --wait --timeout 15m

kubectl delete secret kagent-enterprise-oidc-secret -n kagent --ignore-not-found
```

The UI returns to the bundled demo login.

Delete Keycloak and agentgateway:

```bash
kubectl delete namespace keycloak agentgateway --ignore-not-found
```

Stop the port-forwards, then remove the `/etc/hosts` line, the local files, and the shell variables:

```bash
sudo sed -i.bak '/keycloak.keycloak.svc.cluster.local/d' /etc/hosts
rm -f kagent-oauth-values.yaml substrate-oauth-values.yaml kagent-realm.json.tmpl \
  lab-ca.key lab-ca.crt lab-ca.srl agentgateway.key agentgateway.csr agentgateway.crt agentgateway.ext
unset DEMO_USER DEMO_PASSWORD OIDC_CLIENT_SECRET KEYCLOAK_ADMIN_PASSWORD TOKEN CA_SHA AGW INIT
unset -f kc_token jwt_claims mcp_init a2a
```

## Next

- [020 - Access Control](020-access-control.md) - the mTLS hops the egress gateway sits on
- [040 - Policy Enforcement](040-policy-enforcement.md) - RBAC by Keycloak group, egress grants, and an LLM guardrail. Run it before this lab's cleanup.
- [099 - Cleanup](099-cleanup.md) - when you are done with the workshop
