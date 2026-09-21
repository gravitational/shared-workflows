# ci-metrics

Compaction and reporting over the normalized CI test results that
[`ci-normalize`](../ci-normalize) writes to S3.

## Commands

### `migrate`

Copies one table of JSONL records into its Parquet counterpart, one statement
per day.

```sh
# Last 3 days.
ci-metrics migrate testcases \
  --database example_db \
  --source testcases_jsonl \
  --destination testcases_parquet \
  --days 3

# Backfill a period.
ci-metrics migrate suites \
  --database example_db \
  --source suites_jsonl --destination suites_parquet \
  --from 2026-08-01 --to 2026-09-15

# Render the SQL without executing it, does not need AWS credentials.
ci-metrics migrate meta \
  --database example_db \
  --source meta_jsonl --destination meta_parquet \
  --from 2026-09-14 --dryrun
```

#### Why one statement per day

Athena limits `INSERT INTO` writing more than 100 partitions at a time. 

#### Destination Table layout

The destination collapses `year`/`month`/`day` into one ISO-8601 `dt`, so a
range query is `dt BETWEEN '2026-09-13' AND '2026-09-15'`.
ISO-8601 sorts lexicographically in chronological order so pruning works all the same.


### `report`

Runs statistical reports over the records and writes them to one or more
destinations.

```sh
ci-metrics report --database example_db --config reports.yaml

# A specific window.
ci-metrics report --database example_db --config reports.yaml --from 2026-08-29 --to 2026-09-11

# Render the SQL without executing it, does not need AWS credentials.
ci-metrics report --database example_db --config reports.yaml --days 7 --dryrun
```

See [`docs/reports.example.yaml`](docs/reports.example.yaml) for the config format.

#### The `flaky` report

Ranks tests by a flake score, `4*p*(1-p)*execs/(execs+K)`, and emits two views
of the same window: a rollup over the window's totals, followed by one table
per day.


## Local Dev

```sh
make test
make lint
make binary
```