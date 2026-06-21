# klim examples

Ready-to-use `.klim.yaml` toolchain contracts. Drop one into a project root, then:

```bash
klim check                    # see what's installed vs missing for this contract
klim install go gh            # install the tools it lists (by name)
klim diff teammate.yaml       # compare your machine against another contract
```

A `.klim.yaml` is a tiny, version-control-friendly contract:

```yaml
name: my-project
tools:
    - name: go
      version: ">=1.25"   # optional version constraint
    - name: gh
optional:
    - name: golangci-lint   # nice-to-have, not enforced by `klim check`
```

Generate one automatically for your own project with `klim init` (it reads `go.mod`,
`package.json`, `Dockerfile`, CI workflows, Helm, Terraform, and more).

| File | For |
|---|---|
| [`go-developer.klim.yaml`](go-developer.klim.yaml) | Go backend / CLI development |
| [`frontend.klim.yaml`](frontend.klim.yaml)         | Node / web frontend |
| [`devops.klim.yaml`](devops.klim.yaml)             | Kubernetes / IaC / cloud |
| [`data.klim.yaml`](data.klim.yaml)                 | Python data / ML |
