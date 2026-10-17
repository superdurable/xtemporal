# xtemporal

xtemporal is an independent fork of [Temporal Server](https://github.com/temporalio/temporal) by Temporal/Cadence's core contributor. 

It extends the upstream server with additional capabilities, much as Temporal originated from Uber Cadence. It is not affiliated with or endorsed by Temporal Technologies. “Temporal” is a trademark of its owner.

xtemporal runs and is configured exactly like Temporal Server. For installation, configuration, operations, and SDK usage, follow the [Temporal documentation](https://docs.temporal.io/) and its [self-hosted guide](https://docs.temporal.io/self-hosted-guide). The extensions below are opt-in; without them, server behavior is unchanged.

## Extensions

- **Aurora PostgreSQL Limitless default store.** The `postgres12_limitless` SQL plugin runs Temporal's default (non-visibility) store on Amazon Aurora PostgreSQL Limitless Database for horizontal scale-out. It has been validated on a real Limitless cluster: row-lock fencing, read-after-write across routers, single-shard history transactions, fencing across a shard group reboot, the full persistence suites, and functional tests. See the [Limitless guide](docs/xtemporal/limitless.md).
- **ParadeDB Dex visibility.** The fresh-only v2.0 schema is available through the `paradedb` schema-tool plugin; a dedicated custom visibility store follows in P2–P5. See the [design of record](docs/xtemporal/paradedb-dex-store.md) and [schema guide](docs/xtemporal/paradedb.md).

## Versions and images

xtemporal uses the same version numbers as upstream Temporal Server. The daily sync follows upstream's **Latest** release; a release is published after the update is merged and validated. When xtemporal ships its own changes on top of an upstream version, the release is tagged `vX.Y.Z-x.N`, for example `v1.32.1-x.2`. The `X.Y.Z`, minor, and `latest` image tags keep pointing at the plain upstream-version build.

The image `superdurable/xtemporal:<version>` contains `temporal-server` and `temporal-sql-tool`. Use it wherever the Temporal documentation uses the Temporal server image:

```sh
docker pull superdurable/xtemporal:1.32.1-x.2
```

## Contributing and license

See [CONTRIBUTING.md](CONTRIBUTING.md) for development instructions and [docs/xtemporal/](docs/xtemporal/README.md) for fork maintenance. xtemporal is distributed under the [MIT License](LICENSE). The upstream copyright notices remain in the license; see [NOTICE](NOTICE) for the project's origins.
