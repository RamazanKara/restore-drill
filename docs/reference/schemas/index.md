# Schemas

restore-drill ships machine-readable v1 JSON Schema contracts so you can validate
configs and consume run output in other tools.

- [config-v1.schema.json](config-v1.schema.json) — the drill YAML shape (after
  YAML is interpreted as JSON-compatible data).
- [run-result-v1.schema.json](run-result-v1.schema.json) — output from
  `restore-drill run --format json` and configured run JSON reports.

The schemas describe the public **wire shape**. The Go validator remains the
source of truth for semantic rules such as provider/tool/check compatibility, so
prefer `restore-drill validate` for authoritative config checks:

```bash
restore-drill validate --config drill.yaml
```

## Editor integration

Associate the config schema with your drill files for inline completion and
validation. With the YAML language server (VS Code "YAML" extension, Neovim,
etc.), add a modeline to the top of a config:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/RamazanKara/restore-drill/main/docs/reference/schemas/config-v1.schema.json
drills:
  - name: production-postgres
    provider: postgres
    # ...
```

Or map it globally in VS Code settings:

```json
{
  "yaml.schemas": {
    "https://raw.githubusercontent.com/RamazanKara/restore-drill/main/docs/reference/schemas/config-v1.schema.json": ["drill*.yaml", "**/restore-drill/*.yaml"]
  }
}
```

## CI validation

Any JSON Schema validator can gate configs in CI (for example
[`check-jsonschema`](https://github.com/python-jsonschema/check-jsonschema)),
though `restore-drill validate` is recommended because it also enforces semantic
rules.

## See also

- [Configuration reference](../../guides/configuration.md)
- [Reporting & alerts](../reporting.md) — the run JSON contract.
- [Support policy](../../project/support.md) — what stays stable within v1.
