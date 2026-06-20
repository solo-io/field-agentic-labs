# Keycloak for Dev Environment

In this directory you will find files to support local instance of keycloak that can be used in developer environment. `realm-data/` contains realm configuration and several users. `ssl/` directory contains SSL certificate and key.

## Important Note

This config is ported from kagent-enterprise and matches the config there to provide an easier dev experience when working across repos (e.g., testing all products, using the enterprise UI.)

Worst-case, if this Keycloak doesn't work as expected with kagent enterprise, you can spin up Keycloak from that repository with its own `keycloak-up` Make target.

The below is from the kagent enterprise's README, as it also relates to this. We may want to branch off and create more "realistic" users for this or create scripts to allow creating users for better demo RBAC setups (e.g. security, docs, etc.)

## User Accounts in kagent-dev Realm
Three pre-existing user accounts ship in the kagent-dev realm:

* username: admin, password: password, member of admins group
* username: writer, password: password, member of writers group
* username: reader, password: password, member of readers group

Federated SSO users (via JumpCloud SAML) are auto-assigned to the `admins`
group on first login through the realm's `defaultGroups` setting.

# Keycloak Configuration
This is an instance of keycloak meant to be used in a dev environment, it uses an H2 db to store its internal config and user configs. It serves both plain-text http (port 8088) and tls encrypted traffic (port 8443). 

The TLS cert is self-signed and has both CN and SAN set (as otherwise token introspector used in ui-backend fails cert checks). You can use `generate-keycloak-cert` make target to re-generate the key and the cert.

The host name is set to `keycloak.default`; this matches one of the alternative names in the cert and this name is used to access keycloak from mgmt-cluster.

## Realm Configuration
There's a pre-configured realm called "kagent-dev" that:
 - has four clients configured:
     - **kagent-ui**: public client, kagent-enterprise UI frontend (PKCE).
     - **kagent-backend**: confidential, kagent-enterprise UI backend token validation.
     - **are-backend**: confidential, agentregistry-enterprise backend token validation.
     - **are-cli**: public, arctl device-authorization-grant login flow.
 - has three users (admin, writer, reader) plus three groups (admins, writers, readers).
 - has a JumpCloud SAML identity provider plus email/firstName/lastName
   attribute mappers — the metadata URL is templated in the realm JSON and
   substituted at install time by `scripts/dev-cluster/install-keycloak.sh`.
 - sets `defaultGroups: ["/admins"]` so every newly-federated SSO user lands
   in the admins group automatically.

Various end-points that are available for kagent-dev realm:
issuer	"https://keycloak.default:8443/realms/kagent-dev"
authorization_endpoint	"https://keycloak.default:8443/realms/kagent-dev/protocol/openid-connect/auth"
token_endpoint	"https://keycloak.default:8443/realms/kagent-dev/protocol/openid-connect/token"
introspection_endpoint	"https://keycloak.default:8443/realms/kagent-dev/protocol/openid-connect/token/introspect"

### kagent-ui Client
Public client used by the kagent-enterprise UI frontend (`solo-enterprise-ui-frontend`).
Authorization code with PKCE; maps group membership to a custom `Groups` claim.
No client secret (public client).

### kagent-backend Client
Confidential client used by the kagent-enterprise UI backend
(`solo-enterprise-ui-backend`) for token validation. The client secret is
copied into the `ui-backend-oidc-secret` Kubernetes Secret by
`scripts/dev-cluster/install-kagent.sh`.

### are-backend Client
Confidential client used by agentregistry-enterprise's backend (token
validation middleware). Client secret: `c1fba58c133e8db0c2311f149d832bd4`.

### are-cli Client
Public client used by `arctl` through its
[Device Authorization Grant](https://datatracker.ietf.org/doc/html/rfc8628)
login flow.

## Accessing Keycloak from Kind Clusters
kagent-enterprise-dev chart configures a service called "keycloak" in default namespace that points to an external name "host.docker.internal". 

### /etc/hosts Changes on MacOS (to be verified)
Add to /etc/hosts

```
127.0.0.1   keycloak.default
```

This is required in order for keycloak admin interface to function correctly when accessed from the local host.

### /etc/hosts Changes on Linux
Add to /etc/hosts

```
172.17.0.1  host.docker.internal
127.0.0.1   keycloak.default
```

`host.docker.internal` is a default hostname used by docker to access processes bound/listening on localhost network on MacOS. It is not available on linux, hence the need for this workaround. Please note that you may have a different bridge configured on your machine, to verify use commands `docker network ls` and `docker network inspect <network name>` and look for bridge networks.

`keycloak.default` is required in order for keycloak admin interface to function correctly when accessed from the local host.

## Updating Keycloak Configuration
You can use keycloak admin ui to make changes to the config and or remove users. When finished, export resulting config (in this case for kagent-dev realm):

```
docker exec -u root -it <container-id-here> /opt/keycloak/bin/kc.sh export --dir ./realm-export --realm kagent-dev
```
this command places realm config and realm user configs in files within /realm-export directory of the container. You'll need to copy them to the host machine:
```
docker cp <container-id-here>:/realm-export/kagent-dev-realm.json ./dev-keycloak/realm-data/
docker cp <container-id-here>:/realm-export/kagent-dev-users-0.json ./dev-keycloak/realm-data/
```