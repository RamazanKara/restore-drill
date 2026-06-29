# Incident response

During a real recovery you often need more than a pass/fail signal: you need to
restore to a specific point in time and keep the restored target alive to inspect
it. restore-drill supports both through incident mode.

## Restore to a point in time

Pass `--target` with an RFC 3339 timestamp to override the PITR target for every
drill in the config. This applies to PITR-capable providers/tools (pgBackRest,
WAL-G):

```bash
restore-drill run \
  --config drill.yaml \
  --runtime docker \
  --target "2026-05-20T14:30:00Z"
```

## Keep the restored target for inspection

`--no-cleanup` leaves the container or pod running after the drill so you can
connect to it and inspect the restored data:

```bash
restore-drill run \
  --config drill.yaml \
  --target "2026-05-20T14:30:00Z" \
  --no-cleanup
```

When `--no-cleanup` is set, stdout, the run JSON, and saved state include the
retained target's ID, host, and port map so you can reach it.

### Inspect a retained Docker target

```bash
docker ps                          # find the retained restore-drill container
docker exec -it <container-id> sh  # open a shell inside it
```

### Inspect a retained Kubernetes target

```bash
kubectl get pods -n restore-drill -l restore-drill/ephemeral=true
kubectl exec -it -n restore-drill <pod> -- sh
```

## Clean up afterwards

Retained targets are **not** removed automatically — tear them down when you are
done (treat them as sensitive; they hold restored data):

```bash
docker rm -f <container-id>
# or, for Kubernetes:
kubectl delete pod -n restore-drill -l restore-drill/ephemeral=true
```

## See also

- [CLI reference](../reference/cli.md) — `--target` and `--no-cleanup`.
- [CI/CD integration](ci-cd.md) — scripting drills in pipelines.
- [Troubleshooting](../reference/troubleshooting.md) — when a restore fails.
