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

Ranks the flakiest tests over a window and prints one table to stdout. Scores
are the window's totals.

```sh
# Daily
ci-metrics report --database gh_test_metrics_v2 \
  --branches refs/heads/master,refs/heads/branch/v18,refs/heads/branch/v17 \
  --from 2026-09-27 --to 2026-09-27

# Yesterday (UTC), same as --from and --to both set to yesterday's date
ci-metrics report --database gh_test_metrics_v2 \
  --branches refs/heads/master,refs/heads/branch/v18,refs/heads/branch/v17 \
  --yesterday

# Weekly
ci-metrics report --database gh_test_metrics_v2 \
  --branches refs/heads/master,refs/heads/branch/v18,refs/heads/branch/v17 \
  --from 2026-09-21 --to 2026-09-28

# Render the SQL without executing it, does not need AWS credentials.
ci-metrics report --database gh_test_metrics_v2 --days 7 --dryrun
```


## Local Dev

```sh
make test
make lint
make binary
```