# amicleanup

A Go tool that walks every AMI an AWS account owns across every enabled region
and applies one of four lifecycle actions: `deprecate`, `make-public`,
`make-private`, `delete`. Dry-run is the default; only an explicit
`--dry-run=false` causes real writes.

`--action=delete` deregisters the AMI **and** deletes every EBS snapshot
backing it.

## Build & test

```
make deps     # go mod tidy
make test     # go test ./... -v
make build    # produces ./amicleanup
```

## IAM permissions

The calling principal needs:

```
ec2:DescribeRegions
ec2:DescribeImages
ec2:EnableImageDeprecation       # for --action=deprecate
ec2:ModifyImageAttribute         # for --action=make-public / make-private
ec2:DeregisterImage              # for --action=delete
ec2:DeleteSnapshot               # for --action=delete
sts:GetCallerIdentity            # for plan-file account validation
```

## Usage

```
amicleanup --action=ACTION [flags]

  --action               deprecate | make-public | make-private | delete (required)
  --dry-run              if true, log writes instead of performing them (default true)
  --yes                  skip TTY confirmation for make-public/delete
  --region-concurrency   regions processed in parallel (default 8)
  --plan-file            path to read/write the plan; enables resumable runs
  --plan-only            enumerate, write the plan, and exit (no apply)
```

## Examples

Dry-run a deprecation across the whole account:

```
amicleanup --action=deprecate
```

Write a plan, inspect it, then apply later:

```
amicleanup --action=delete --plan-file=cleanup.json --plan-only
jq . cleanup.json
amicleanup --action=delete --plan-file=cleanup.json --dry-run=false --yes
```

Resume after a crash: re-run the same command. Entries already marked
`completed` are skipped; `pending` and `failed` entries are retried.

```
amicleanup --action=delete --plan-file=cleanup.json --dry-run=false --yes
```

## Why a plan file?

The tool can iterate tens of thousands of AMIs across dozens of regions; a
transient API throttle or an AWS-side timeout in the middle of a multi-hour
run is common. The plan file makes a partial run safe to re-execute:
enumeration happens once, status is persisted entry-by-entry, and a second
invocation only touches the entries that didn't finish.

When `--plan-file` is omitted the tool still writes a temp plan under
`$TMPDIR` for crash forensics; check `/tmp/amicleanup-*.json` if a run dies
without `--plan-file` set.

### On-disk format

`--plan-file=PATH` actually produces two files:

- **`PATH`** — the plan: schema version, account, action, and the immutable
  list of AMIs. Written once at enumeration time, never updated.
- **`PATH.log`** — an append-only JSON-lines status log. Each completed
  (or failed) entry produces one short line. The log is replayed on resume
  to reconstruct each entry's current state.

This split keeps total writes at O(N) for N entries — for an account with
35,000 AMIs the log is a few megabytes, not the ~250 GB you'd get from
rewriting the whole plan after every entry.
