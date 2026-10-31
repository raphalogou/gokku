# Gokku behind Caddy

Use this when Caddy, not Dokku's nginx, is the reverse proxy on the host: for example on a server where Dokku's
proxy is disabled or replaced, or where Caddy fronts several services. Any reverse proxy that supports HTTP/1.1
streaming works; Caddy needs no special settings.

1. Add a site to your Caddyfile (usually `/etc/caddy/Caddyfile`):

   ```
   gokku.example.com {
   	reverse_proxy 127.0.0.1:8080
   }
   ```

   Caddy gets and renews the certificate itself, and sends `X-Forwarded-For` and `X-Forwarded-Proto`.
   Server-sent events (live logs, job output) are flushed as they arrive: Caddy does not buffer
   `text/event-stream` responses, and Gokku sends a heartbeat every 15 seconds.

2. Tell Gokku to trust the proxy, in `/etc/gokku/gokku.env`:

   ```
   GOKKU_TRUSTED_PROXIES=127.0.0.1/32
   ```

   Then `sudo systemctl restart gokku`. Without it, Gokku ignores the forwarded headers, does not mark the session
   cookie `Secure`, and rate-limits by the proxy's address; it logs one warning when it sees a forwarded request
   (`SEC-3a`).

3. Reload Caddy: `sudo systemctl reload caddy`.

Caddy and Dokku's nginx cannot both listen on ports 80 and 443. If Dokku uses its nginx proxy, follow the nginx
steps in the README instead (`deploy/nginx.conf.example`).

Dokku's own Caddy proxy (`dokku proxy:set --global caddy`) runs Caddy in a container configured from Docker
labels of Dokku apps. It cannot reach a service on the host's loopback address, and Gokku is not a Dokku app,
so it does not fit that setup in the MVP. Exposing Gokku as a Dokku app comes with the SSH runner (`SPEC.md`,
section 12).
