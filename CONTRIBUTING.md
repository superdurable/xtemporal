# Contributing to xtemporal

This repository is the independent [xtemporal](https://github.com/superdurable/xtemporal) fork. Open xtemporal changes and issues here. For bugs or requests that affect only upstream Temporal Server, use the [upstream issue tracker](https://github.com/temporalio/temporal/issues).

## Development

Use the Go version in [go.mod](go.mod). Clone this repository and run the relevant checks before opening a pull request:

```sh
git clone https://github.com/superdurable/xtemporal.git
cd xtemporal
make bins
make lint-code
```

For Go tests, include the `test_dep` build tag. Integration tests also require the `integration` tag and their external services. See the [testing guide](docs/development/testing.md) and [maintainer documentation](docs/xtemporal/README.md).

Changes to upstream owned code may need to be reconciled during the next sync. Keep xtemporal extensions in the directories described in the maintainer documentation where possible.
