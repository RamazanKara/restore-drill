---
name: Bug report
about: Report a restore, validation, runtime, or release issue
labels: bug
---

## What happened?

## What did you expect?

## Reproduction

Please include the smallest config excerpt that reproduces the issue.

```yaml
# redact secrets, bucket names, restored data values, and internal hostnames
```

## Environment

- restore-drill version:
- runtime: docker / kubernetes
- provider: postgres / mysql / redis
- backup tool:
- operating system or Kubernetes version:
- restore target image:

## Evidence

- command run:
- exit code:
- expected RTO/RPO behavior:
- actual RTO/RPO behavior:

## Logs

Please redact secrets, bucket names, and restored data values.

## Redaction checklist

- [ ] Credentials and tokens removed
- [ ] Bucket/repository names generalized when sensitive
- [ ] Restored production-like data values removed
- [ ] Internal hostnames/IPs removed when sensitive
