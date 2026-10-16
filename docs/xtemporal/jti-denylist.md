# Revocable JWT API keys

Set `global.authorization.claimMapper: jtiDenylist` and `global.authorization.authorizer: platformNamespaceLifecycle` on the frontend. The mapper wraps Temporal's default JWT mapper: signature, expiry, audience, subject, and permissions are checked before the denylist lookup. Internal frontend uses its separate internal claim mapper and is unaffected. The ordinary `default` mapper is unchanged.

The `platformNamespaceLifecycle` authorizer wraps Temporal's default authorizer. It denies `RegisterNamespace`, `UpdateNamespace`, `DeleteNamespace`, and `DeprecateNamespace` unless the caller has a system role, even when the token has `<ns>:admin`. The default authorizer still checks the system role's level, so a system admin can perform these operations. All other calls retain default authorization: `<ns>:admin` can add, remove, and list search attributes in its own namespace, but cannot add search attributes in another namespace or call cluster-scoped OperatorService or AdminService APIs. The platform-issued Dex credential needs the search-attribute permissions within its namespace.

Configure the frontend process with:

| Environment variable | Meaning |
| --- | --- |
| `XTEMPORAL_JTI_DENYLIST_FILE` | Required absolute path to the mounted denylist file. |
| `XTEMPORAL_JTI_DENYLIST_POLL_INTERVAL` | Positive Go duration; defaults to `30s`. |
| `XTEMPORAL_JTI_DENYLIST_STARTUP_MODE` | `fail-closed` (default) or `empty`. |

The file is JSON Lines, one revoked key per line. `exp` is the JWT expiration as Unix seconds:

```json
{"jti":"key-id-123","exp":1790000000}
```

Every API key token must contain a nonempty `jti` and a valid `exp`. Expired denylist entries are discarded on each load; the mapper also checks expiry when serving requests. A malformed or missing file emits `xtemporal_jti_denylist_reload_failure` through the server metrics handler and logs an error. The last successfully loaded list remains active. At startup, before any valid file has loaded, `fail-closed` rejects all JWTs; `empty` accepts valid JWTs until a file loads. An empty, valid file clears the list.

Revocation normally takes effect in about 1–2 minutes, including ConfigMap propagation and polling. A single ConfigMap holds roughly 15–20k entries; split or replace this source before reaching that scale. The `auth.Source` interface isolates the file loader so a Secret, object store, or DB/API source can replace it without changing the mapper. The lookup is in memory and does not add network requests to authentication.

Publish multiple `kid` values in the JWKS during signing key rotation. Keep old public keys published until their issued JWTs expire, while minting new tokens with the new `kid`. User tokens must never carry `temporal-system:*` permissions; grant only the required namespace permissions. The denylist is scoped to the public frontend and is not a substitute for limiting token permissions or lifetimes.
