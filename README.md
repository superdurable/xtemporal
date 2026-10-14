# xtemporal

xtemporal is an independent fork of [Temporal Server](https://github.com/temporalio/temporal) that extends the upstream server with additional capabilities, much as Temporal originated from Uber Cadence. It is not affiliated with or endorsed by Temporal Technologies. “Temporal” is a trademark of its owner.

The first extension is advanced visibility backed by ParadeDB. **ParadeDB visibility is EXPERIMENTAL and not for production use:** the pinned ParadeDB 0.26.0 backend exited during a 16-writer benchmark. See the [ParadeDB visibility guide](docs/xtemporal/paradedb.md), [benchmark release gate](docs/xtemporal/paradedb-benchmark.md), and [maintainer documentation](docs/xtemporal/README.md).

## Versions and releases

xtemporal uses the same version numbers as upstream Temporal Server, starting with `v1.32.1`. The planned daily sync follows upstream's **Latest** release and proposes an updated snapshot for review. A release is published after that update is merged and validated.

Go module tags such as `v1.32.1` are immutable. If xtemporal needs its own hotfix on an upstream version, the hotfix receives a Docker tag such as `1.32.1-x.1`, while the moving `1.32.1` Docker tag points to the newest build for that version. Go consumers use the hotfix commit's pseudo-version; the `v1.32.1` Go tag does not move.

## Quick start

The planned release image is `superdurable/xtemporal:<version>` (for example, `superdurable/xtemporal:1.32.1`). It contains both `temporal-server` and `temporal-sql-tool`. Once the first image is published, pull it with:

```sh
docker pull superdurable/xtemporal:1.32.1
```

To embed the server in a Go application after the first module release:

```sh
go get github.com/superdurable/xtemporal@v1.32.1
```

```go
import (
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/temporal"
)

func newServer(cfg *config.Config) (temporal.Server, error) {
	return temporal.NewServer(temporal.WithConfig(cfg))
}
```

Supply your application configuration and any other required options; see the [server package](temporal/). Do not import xtemporal and `go.temporal.io/server` into the same binary: that would include two copies of the server.

## Contributing and license

See [CONTRIBUTING.md](CONTRIBUTING.md) for development instructions and [docs/xtemporal/](docs/xtemporal/README.md) for fork maintenance. xtemporal is distributed under the [MIT License](LICENSE). The upstream copyright notices remain in the license; see [NOTICE](NOTICE) for the project's origins.
