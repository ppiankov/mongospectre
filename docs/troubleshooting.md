# Troubleshooting

## Connection Timeout (ReplicaSetNoPrimary)

```
Error: connect: server selection error: context deadline exceeded,
current topology: { Type: ReplicaSetNoPrimary, Servers: [...] }
```

This means mongospectre could not reach any replica set member within the timeout.

**Common causes:**

1. **IP not whitelisted (Atlas)** — go to Atlas > Network Access and add your current IP, or `0.0.0.0/0` for testing
2. **Firewall or VPN** — port 27017 (or 27015-27017 for Atlas) must be open outbound
3. **DNS resolution failure** — test with `nslookup _mongodb._tcp.<cluster>.mongodb.net`
4. **Cluster paused (Atlas)** — free-tier (M0) clusters pause after 60 days of inactivity
5. **Timeout too short** — Atlas SRV resolution + TLS handshake can be slow on first connect

**Debug steps:**

```bash
# 1. Test with mongosh (rules out mongospectre-specific issues)
mongosh "<your-connection-string>"

# 2. Check DNS resolution
nslookup _mongodb._tcp.<cluster>.mongodb.net

# 3. Try with longer timeout
mongospectre audit --uri "<your-uri>" --timeout 60s

# 4. Use verbose mode for connection details
mongospectre audit --uri "<your-uri>" --timeout 60s --verbose
```

## Authentication Failed

```
Error: connect: authentication failed
```

**Common causes:**

1. **Wrong username or password** — check for special characters that need URL-encoding (e.g., `@` becomes `%40`)
2. **Wrong auth database** — Atlas uses `admin` by default. If your user is in a different database, append `?authSource=<db>` to the URI
3. **User does not exist** — verify the user exists in Atlas > Database Access

**URI format:**

```
<scheme>://<user>:<pass>@<host>/?authSource=admin
```

## Connection Refused

```
Error: connect: connection refused
```

**Common causes:**

1. **MongoDB is not running** — check with `systemctl status mongod` or `brew services list`
2. **Wrong host or port** — default is `localhost:27017`
3. **MongoDB bound to different interface** — check `bindIp` in `mongod.conf`

## DNS Resolution Failed

```
Error: connect: no such host
```

**Common causes:**

1. **Typo in hostname** — double-check the cluster name in your URI
2. **SRV record not found** — SRV-style URIs require DNS SRV records. If behind a corporate DNS that blocks SRV, use the standard URI scheme with individual host addresses
3. **Network not available** — check internet connectivity

## Timeout Tuning

The default timeout is 30 seconds. For Atlas or high-latency connections:

```bash
# CLI flag
mongospectre audit --uri "<your-uri>" --timeout 60s

# Config file (.mongospectre.yml)
defaults:
  timeout: 60s

# Environment variable (for URI only)
export MONGODB_URI="<your-uri>"
```

## Read-Only Access Requirements

mongospectre requires only read access. The minimum role is `readAnyDatabase` for multi-database scanning, or `read` on a specific database with `--database`.

Optional features require additional roles:

| Feature | Flag | Required Role |
|---------|------|---------------|
| User audit | `--audit-users` | `userAdmin` or `userAdminAnyDatabase` |
| Sharding analysis | `--sharding` | `read` on `config` database |
| Atlas suggestions | `--atlas-*` | Atlas API key (separate from DB user) |

## Panic or Unexpected Crash

If mongospectre panics, please report it at [github.com/ppiankov/mongospectre/issues](https://github.com/ppiankov/mongospectre/issues) with:

1. The full panic stack trace
2. MongoDB version (`mongosh --eval "db.version()"`)
3. mongospectre version (`mongospectre version`)
4. Whether the cluster is Atlas, self-hosted, or sharded
