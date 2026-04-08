# Installing lazyaz (while the repo is private)

These instructions are temporary — once `github.com/karlssonsimon/lazyaz` is
public, a plain `go install github.com/karlssonsimon/lazyaz/cmd/lazyaz@latest`
will be enough and this file can be deleted.

## One-time setup

```bash
# Treat the module as private — skips proxy.golang.org and sum.golang.org
go env -w 'GOPRIVATE=github.com/karlssonsimon/*'

# Make git use SSH for github.com instead of HTTPS so go's git clone
# falls back to your SSH key
git config --global url."git@github.com:".insteadOf "https://github.com/"
```

You need an SSH key registered with GitHub that has read access to the repo.
Verify with `ssh -T git@github.com`.

## Install

```bash
go install github.com/karlssonsimon/lazyaz/cmd/lazyaz@v0.3.0
```

The binary lands in `$GOBIN` (or `$HOME/go/bin` if `GOBIN` is unset). Make
sure that directory is on your `PATH`.

## First-run config

`lazyaz` reads its config from `~/.config/lazyaz/config.yaml` (or
`$XDG_CONFIG_HOME/lazyaz/config.yaml`). The cache lives at
`~/.lazyaz/cache.db` and rebuilds itself on first run.

If you previously ran the project under its old name, migrate your config:

```bash
mv ~/.config/aztools ~/.config/lazyaz
rm -rf ~/.aztui   # optional — old cache, will be rebuilt
```

## Upgrading

Bump the version in the install command:

```bash
go install github.com/karlssonsimon/lazyaz/cmd/lazyaz@v0.2.0
```
