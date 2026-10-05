# Offline Developer Portal — Quick start (Linux)

Nothing needs to be installed; `offline-docs` is a static binary.

```sh
cp sources.example.yaml sources.yaml   # then edit sources.yaml
./index-linux.sh                       # phase 1: crawl and build the corpus (needs access to the sites)
./serve-linux.sh                       # phase 2: browse at http://localhost:8080 (no network needed)
```

- Each source in `sources.yaml` needs an `id`, `start_urls` and crawl boundaries
  (`allowed_prefixes` and/or `allowed_hosts`). The crawler never leaves them.
  Keep `max_pages` small to start with.
- `./serve-linux.sh --port 9000` uses another port; `--open` opens a browser;
  `--host 0.0.0.0` shares the portal on the network (default: this machine only).
- Re-run `./index-linux.sh` to refresh. `./index-linux.sh --source ID` re-crawls one
  source and keeps the others. The old corpus is replaced only if the new crawl succeeds.
- `./offline-docs search "query"` searches the corpus from the command line.
- `./offline-docs validate --config sources.yaml` checks a configuration without crawling.

`sources.yaml` and `corpus/` are local data; they are never part of the release
or the Git repository.

Full documentation: https://github.com/pcowhill/offline-developer-portal
