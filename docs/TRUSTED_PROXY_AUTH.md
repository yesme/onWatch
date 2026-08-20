# Trusted Reverse-Proxy / SSO Auth Mode

If your onWatch dashboard sits behind a reverse proxy that already authenticates users (Authentik, Authelia, oauth2-proxy, Traefik ForwardAuth, Caddy forward_auth, Nginx auth_request), you can skip the second onWatch login with `trusted_proxy` mode.

## How it works

```
ONWATCH_AUTH_MODE=local          # default: built-in login form + Basic Auth
ONWATCH_AUTH_MODE=trusted_proxy  # trust identity headers from configured proxies
```

In `trusted_proxy` mode a request is let through without the onWatch login when **both** are true:

1. The request's direct TCP peer (not `X-Forwarded-For`) is inside one of the configured `ONWATCH_TRUSTED_PROXY_CIDRS`.
2. The configured `ONWATCH_TRUSTED_USER_HEADER` is present and non-empty.

Everything else falls back to the normal auth flow:

- Requests from untrusted addresses are handled exactly as in `local` mode, even if they carry a (spoofed) identity header.
- Requests from a trusted address without the identity header get the login page / 401.
- `/api/*` still accepts session cookies and Basic Auth, so scripts and internal Docker callers that bypass the proxy keep working with credentials.

Fail-closed guarantees:

- The default mode is `local`; nothing changes unless you opt in.
- `trusted_proxy` without `ONWATCH_TRUSTED_PROXY_CIDRS` refuses to start.
- An invalid CIDR entry refuses to start.

## Configuration

```bash
ONWATCH_AUTH_MODE=trusted_proxy
ONWATCH_TRUSTED_PROXY_CIDRS=172.30.0.0/16,127.0.0.1   # CIDRs or bare IPs, comma-separated
ONWATCH_TRUSTED_USER_HEADER=X-authentik-username       # default: X-Forwarded-User
```

Keep the CIDR list as narrow as possible - ideally the exact IP of the proxy container. Any host inside a trusted CIDR can assert an identity header, so a wide range (like a whole Docker bridge network) means every container on that network can reach the dashboard unauthenticated.

Make sure the proxy strips or overwrites any client-supplied identity header on every request (all of the proxies above do this by default in forward-auth mode, but verify custom setups). onWatch trusts the header unconditionally once the request comes from a trusted peer - a proxy that merely appends its value would let a client smuggle in its own. As defense in depth, onWatch rejects requests that carry more than one value for the identity header.

On a shared or multi-user host, trusting loopback (`127.0.0.1`) exposes onWatch to every local user and process - any of them can connect with the header set. Only trust loopback on single-user machines.

## Example: Authentik + Traefik ForwardAuth (docker-compose)

```yaml
services:
  onwatch:
    image: ghcr.io/onllm-dev/onwatch:latest
    networks: [proxy]
    environment:
      ONWATCH_AUTH_MODE: trusted_proxy
      # the static IP assigned to traefik below
      ONWATCH_TRUSTED_PROXY_CIDRS: 172.30.0.2/32
      ONWATCH_TRUSTED_USER_HEADER: X-authentik-username
    labels:
      traefik.enable: "true"
      traefik.http.routers.onwatch.rule: Host(`onwatch.example.com`)
      traefik.http.routers.onwatch.middlewares: authentik@docker
      traefik.http.services.onwatch.loadbalancer.server.port: "9211"

  traefik:
    image: traefik:v3
    networks:
      proxy:
        ipv4_address: 172.30.0.2

networks:
  proxy:
    ipam:
      config:
        - subnet: 172.30.0.0/16
```

Authentik's proxy provider (forward auth mode) sets `X-authentik-username` on successful auth; Traefik's `authResponseHeaders` must include it:

```yaml
traefik.http.middlewares.authentik.forwardauth.address: http://authentik:9000/outpost.goauthentik.io/auth/traefik
traefik.http.middlewares.authentik.forwardauth.trustForwardHeader: "true"
traefik.http.middlewares.authentik.forwardauth.authResponseHeaders: X-authentik-username,X-authentik-email
```

## Example: oauth2-proxy

When oauth2-proxy sits in front of onWatch as a proxying upstream, `--pass-user-headers` (on by default) sets `X-Forwarded-User`:

```bash
ONWATCH_AUTH_MODE=trusted_proxy
ONWATCH_TRUSTED_PROXY_CIDRS=10.8.0.5/32     # oauth2-proxy's address
# ONWATCH_TRUSTED_USER_HEADER left at default X-Forwarded-User
```

If you instead use oauth2-proxy behind Nginx `auth_request` with `--set-xauthrequest`, the identity header is `X-Auth-Request-User` - set `ONWATCH_TRUSTED_USER_HEADER=X-Auth-Request-User` and list Nginx (the direct peer) in the CIDRs.

## Notes

- The identity is used for auth bypass and debug logging only; onWatch remains a single-admin app, so all proxy-authenticated users see the same dashboard.
- Sign out at your SSO provider: in `trusted_proxy` mode the onWatch logout button only clears the local session, and the next request through the proxy is authenticated again.
- Changing the dashboard password still requires the current password, even for proxy-authenticated requests.
- The login page, local Basic Auth, and the menubar's localhost endpoints keep working unchanged - useful if you also access onWatch directly on localhost.
- `/metrics` keeps its own bearer-token auth (`ONWATCH_METRICS_TOKEN`).
