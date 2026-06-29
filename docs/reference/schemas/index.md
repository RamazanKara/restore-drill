# Schemas

This directory contains machine-readable v1 contracts for integrations:

- [config-v1.schema.json](config-v1.schema.json): drill YAML shape after YAML is
  interpreted as JSON-compatible data.
- [run-result-v1.schema.json](run-result-v1.schema.json): output from
  `restore-drill run --format json` and configured run JSON reports.

The schemas describe the public wire shape. The Go validator remains the source
of truth for semantic rules such as provider/tool/check compatibility.
