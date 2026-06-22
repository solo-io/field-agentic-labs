# Kubernetes OIDC Authentication with Keycloak + Pinniped

This lab sets up Keycloak as an OIDC provider for **Kubernetes itself** (not kagent) using Pinniped. It is independent of the rest of the workshop - you don't need it to run kagent or apply `AccessPolicy`. It's how you make `kubectl` itself OIDC-authenticated instead of relying on the cluster's static admin credentials.

The same Keycloak you stand up here can later be used as the OIDC provider for the kagent UI ([020](003-install-kagent-enterprise.md)) and as the `UserGroup` token source for `AccessPolicy` ([061](031-accesspolicy-usergroup.md)).

## Architecture

```
User
 │
 ▼
pinniped get kubeconfig
 │
 ▼
kubectl (with Pinniped credential plugin)
 │
 ▼
Browser → Keycloak (authenticate) → OIDC Token
 │
 ▼
Pinniped Concierge validates token → impersonates user
 │
 ▼
Kubernetes API Server → RBAC authorization
```

## Lab Objectives

- Deploy Keycloak (`quay.io/keycloak/keycloak:26.0`) with HTTPS via a self-signed cert
- Configure a `kubernetes` realm, three groups (`k8s-admins`, `k8s-developers`, `k8s-viewers`), a test user, and a `pinniped-cli` client with a `groups` mapper
- Install Pinniped Concierge in the cluster and a `JWTAuthenticator` pointing at the Keycloak realm
- Bind the three Keycloak groups to `cluster-admin`, `edit`, and `view` `ClusterRoles`
- Generate a Pinniped-flavored kubeconfig and authenticate as the test user

## Prerequisites

- Any Kubernetes cluster with `kubectl` cluster-admin access
- `helm` v3
- `openssl`

## Part 1: Install Keycloak

### 1.1 Create the Namespace

```bash
kubectl create namespace keycloak
```

### 1.2 Generate a TLS Certificate

Pinniped requires HTTPS for the OIDC issuer. Generate a self-signed certificate. You'll need to know the LoadBalancer IP - if you don't yet, run with a placeholder, deploy Keycloak, then regenerate once you have the real IP.

```bash
# Replace with the LoadBalancer IP once known.
export KEYCLOAK_IP=<your-loadbalancer-ip-or-placeholder>

openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
 -keyout keycloak-tls.key \
 -out keycloak-tls.crt \
 -subj "/CN=keycloak" \
 -addext "subjectAltName=DNS:keycloak,DNS:keycloak.keycloak.svc.cluster.local,IP:${KEYCLOAK_IP}"

kubectl create secret tls keycloak-tls \
 --cert=keycloak-tls.crt \
 --key=keycloak-tls.key \
 -n keycloak

# Save the CA for Pinniped's JWTAuthenticator (step 3.2)
export KEYCLOAK_CA_BASE64=$(cat keycloak-tls.crt | base64 | tr -d '\n')
```

> **Security note:** the source repo committed `keycloak-tls.key` and `keycloak-tls.crt` directly. **They are not copied into this workshop.** Treat the private key as sensitive - never commit it. The `openssl` command above regenerates a fresh cert; that's the right pattern for both demo and production.

### 1.3 Deploy Keycloak

```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
 name: keycloak-data
 namespace: keycloak
spec:
 accessModes: [ReadWriteOnce]
 resources:
 requests:
 storage: 1Gi
---
apiVersion: apps/v1
kind: Deployment
metadata:
 name: keycloak
 namespace: keycloak
spec:
 replicas: 1
 selector: { matchLabels: { app: keycloak } }
 template:
 metadata: { labels: { app: keycloak } }
 spec:
 initContainers:
 - name: fix-permissions
 image: busybox
 command: ['sh', '-c', 'chown -R 1000:1000 /data']
 volumeMounts:
 - { name: data, mountPath: /data }
 containers:
 - name: keycloak
 image: quay.io/keycloak/keycloak:26.0
 args: ["start-dev"]
 env:
 - { name: KEYCLOAK_ADMIN, value: "admin" }
 - { name: KEYCLOAK_ADMIN_PASSWORD, value: "Password12!@" }
 - { name: KC_HTTPS_CERTIFICATE_FILE, value: "/etc/keycloak/tls/tls.crt" }
 - { name: KC_HTTPS_CERTIFICATE_KEY_FILE, value: "/etc/keycloak/tls/tls.key" }
 - { name: KC_HOSTNAME_STRICT, value: "false" }
 ports:
 - { containerPort: 8443 }
 volumeMounts:
 - { name: tls, mountPath: /etc/keycloak/tls, readOnly: true }
 - { name: data, mountPath: /opt/keycloak/data }
 resources:
 requests: { memory: 512Mi, cpu: 250m }
 limits: { memory: 1Gi, cpu: 1000m }
 volumes:
 - { name: tls, secret: { secretName: keycloak-tls } }
 - { name: data, persistentVolumeClaim: { claimName: keycloak-data } }
---
apiVersion: v1
kind: Service
metadata:
 name: keycloak
 namespace: keycloak
spec:
 type: LoadBalancer
 selector: { app: keycloak }
 ports:
 - { port: 443, targetPort: 8443 }
EOF
```

### 1.4 Verify

```bash
kubectl get pods -n keycloak --watch
kubectl get svc keycloak -n keycloak

export KEYCLOAK_IP=$(kubectl get svc keycloak -n keycloak \
 -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
echo "Keycloak is available at: https://${KEYCLOAK_IP}"
```

If you used a placeholder IP for the cert in Step 1.2, regenerate the cert + secret with the real IP now.

## Part 2: Configure Keycloak

### 2.1 Open the Admin Console

Browse to `https://<KEYCLOAK_IP>` (accept the self-signed cert warning) → Administration Console → log in with `admin / Password12!@`.

### 2.2 Create the `kubernetes` Realm

Hover the **master** dropdown (top-left) → **Create Realm** → Name: `kubernetes` → **Create**.

### 2.3 Create Groups

In the `kubernetes` realm, **Groups** → **Create group**, three times:

- `k8s-admins`
- `k8s-developers`
- `k8s-viewers`

### 2.4 Create a User

**Users** → **Add user** with:

- Username: `testuser`
- Email: `testuser@example.com`, **Email verified: ON**
- First name: `Test`, Last name: `User`

**Create** → **Credentials** tab → **Set password** = `Password123`, **Temporary: OFF** → **Groups** tab → **Join Group** → `k8s-admins`.

### 2.5 Create the `pinniped-cli` Client

**Clients** → **Create client**:

- **General Settings**: Client type `OpenID Connect`, Client ID `pinniped-cli` → Next
- **Capability config**: Client authentication **OFF** (public client), Authorization **OFF**, Authentication flow ✓ Standard flow, ✓ Direct access grants → Next
- **Login settings**: Valid redirect URIs `http://127.0.0.1/callback` → **Save**

### 2.6 Configure the Group Mapper

Keycloak doesn't include `groups` in tokens by default.

`pinniped-cli` → **Client scopes** tab → click `pinniped-cli-dedicated` → **Configure a new mapper** → **Group Membership**:

- Name: `groups`
- Token Claim Name: `groups`
- Full group path: **OFF**
- Add to ID token: **ON**
- Add to access token: **ON**
- Add to userinfo: **ON**

### 2.7 Verify Discovery

```
https://<KEYCLOAK_IP>/realms/kubernetes/.well-known/openid-configuration
```

The `issuer` field there is what Pinniped will validate.

## Part 3: Install Pinniped

Pinniped's **Concierge** runs in-cluster, validates external OIDC tokens, and impersonates the user when talking to the API server.

### 3.1 Install the Concierge

```bash
kubectl apply -f https://get.pinniped.dev/latest/install-pinniped-concierge.yaml
kubectl get pods -n pinniped-concierge
```

### 3.2 Create the JWTAuthenticator

```bash
kubectl apply -f - <<EOF
apiVersion: authentication.concierge.pinniped.dev/v1alpha1
kind: JWTAuthenticator
metadata:
 name: keycloak
spec:
 issuer: https://${KEYCLOAK_IP}/realms/kubernetes
 audience: pinniped-cli
 claims:
 username: email
 groups: groups
 tls:
 certificateAuthorityData: ${KEYCLOAK_CA_BASE64}
EOF
```

### 3.3 Verify

```bash
kubectl get jwtauthenticator keycloak -o yaml
```

The `status` should report the authenticator as ready.

## Part 4: Configure RBAC

Bind each Keycloak group to a built-in Kubernetes `ClusterRole`.

### 4.1 `k8s-admins` → `cluster-admin`

```bash
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
 name: keycloak-cluster-admins
subjects:
 - kind: Group
 name: k8s-admins
 apiGroup: rbac.authorization.k8s.io
roleRef:
 kind: ClusterRole
 name: cluster-admin
 apiGroup: rbac.authorization.k8s.io
EOF
```

### 4.2 `k8s-developers` → `edit`

```bash
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
 name: keycloak-developers
subjects:
 - kind: Group
 name: k8s-developers
 apiGroup: rbac.authorization.k8s.io
roleRef:
 kind: ClusterRole
 name: edit
 apiGroup: rbac.authorization.k8s.io
EOF
```

### 4.3 `k8s-viewers` → `view`

```bash
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
 name: keycloak-viewers
subjects:
 - kind: Group
 name: k8s-viewers
 apiGroup: rbac.authorization.k8s.io
roleRef:
 kind: ClusterRole
 name: view
 apiGroup: rbac.authorization.k8s.io
EOF
```

## Part 5: User Authentication

### 5.1 Install the Pinniped CLI

```bash
curl -L https://get.pinniped.dev/latest/pinniped-cli-linux-amd64 -o pinniped
chmod +x pinniped
sudo mv pinniped /usr/local/bin/
```

(For macOS, replace `linux-amd64` with `darwin-amd64` or `darwin-arm64`.)

### 5.2 Generate the Kubeconfig

```bash
pinniped get kubeconfig \
 --oidc-issuer https://${KEYCLOAK_IP}/realms/kubernetes \
 --oidc-client-id pinniped-cli \
 --oidc-scopes openid,email,groups \
 --oidc-listen-port 12345 \
 --oidc-ca-bundle keycloak-tls.crt \
 > pinniped-kubeconfig.yaml
```

### 5.3 Authenticate

```bash
export KUBECONFIG=pinniped-kubeconfig.yaml
kubectl get pods
```

`kubectl` will:

1. Open your browser to the Keycloak login page.
2. After you authenticate, redirect back with tokens.
3. Complete the `kubectl` command with your OIDC identity (`testuser@example.com`, group `k8s-admins`).

### 5.4 Verify

```bash
kubectl auth whoami
kubectl get pods --all-namespaces
```

## Part 6: Troubleshooting

### 6.1 Concierge Logs

```bash
kubectl logs -n pinniped-concierge -l app=concierge --tail=100
```

### 6.2 JWTAuthenticator Status

```bash
kubectl get jwtauthenticator keycloak -o jsonpath='{.status}' | jq .
```

### 6.3 Inspect a Real Token

```bash
curl -k -X POST "https://${KEYCLOAK_IP}/realms/kubernetes/protocol/openid-connect/token" \
 -d "client_id=pinniped-cli" \
 -d "grant_type=password" \
 -d "username=testuser" \
 -d "password=Password123" \
 -d "scope=openid email groups" \
 | jq -r '.access_token' \
 | cut -d'.' -f2 \
 | base64 -d \
 | jq .
```

Expected:

```json
{
 "email": "testuser@example.com",
 "groups": ["k8s-admins"],
 ...
}
```

### 6.4 Common Issues

| Symptom | Fix |
|---|---|
| "Unable to connect to issuer" | Keycloak must be reachable from inside the cluster. Verify `JWTAuthenticator.spec.issuer` exactly matches the value in the discovery document. |
| Groups not appearing in the token | The group mapper in Step 2.6 isn't right. Confirm "Add to ID token" and "Add to access token" are both ON. |
| `Unauthorized` after a successful login | The RBAC `Group` name doesn't match the value in the `groups` claim. Confirm both use the same string (e.g., `k8s-admins` vs `/k8s-admins`). |

## Cleanup

```bash
# Pinniped Concierge + JWTAuthenticator
kubectl delete jwtauthenticator keycloak --ignore-not-found
kubectl delete clusterrolebinding keycloak-cluster-admins --ignore-not-found
kubectl delete clusterrolebinding keycloak-developers --ignore-not-found
kubectl delete clusterrolebinding keycloak-viewers --ignore-not-found
kubectl delete -f https://get.pinniped.dev/latest/install-pinniped-concierge.yaml --ignore-not-found

# Keycloak
kubectl delete namespace keycloak --ignore-not-found

# Local files
rm -f keycloak-tls.crt keycloak-tls.key pinniped-kubeconfig.yaml pinniped
unset KEYCLOAK_IP KEYCLOAK_CA_BASE64
```

## Quick Reference

| Component | URL / Value |
|-----------|-------------|
| Keycloak admin | `https://<KEYCLOAK_IP>` |
| OIDC issuer | `https://<KEYCLOAK_IP>/realms/kubernetes` |
| Discovery | `https://<KEYCLOAK_IP>/realms/kubernetes/.well-known/openid-configuration` |
| Client ID | `pinniped-cli` |
| Username claim | `email` |
| Groups claim | `groups` |

## Next

- [061 - `UserGroup` `AccessPolicy`](031-accesspolicy-usergroup.md) - the same Keycloak (different realm or client) can be the JWT source for runtime AccessPolicy
- [090 - Microsoft Entra ID OBO](070-obo-entra.md) - same idea (OIDC token = identity), different IdP, completely different downstream
