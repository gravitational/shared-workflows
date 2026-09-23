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

Runs one statistical report over the records and writes it to one or more
destinations. 

```sh
ci-metrics report flaky_rollup --database example_db --config reports.yaml

# A specific window.
ci-metrics report flaky_daily --database example_db --config reports.yaml --from 2026-08-29 --to 2026-09-11

# Render the SQL without executing it, does not need AWS credentials.
ci-metrics report flaky_daily --database example_db --config reports.yaml --days 7 --dryrun
```

See [`docs/reports.example.yaml`](docs/reports.example.yaml) for the config format.

#### Reporters

A report writes to every reporter it names under `reporters:`, or to all of
them when it names none. Reporters are named instances to allow multiple instances of
the same type.

#### The `slack` reporter

Posts the document as Block Kit blocks: one message carrying the report's title
and first table, with any further sections as thread replies to avoid huge
messages. 

Tables go in a [markdown block][mdblock], which Slack lays out as a real table.

```yaml
reporters:
  example:
    type: slack
    # Caps the rows posted per table. Reports can be much longer than a
    # channel wants.
    max_rows: 15
    slack:
      channel: <id or name>
      token_env: CI_METRICS_SLACK_TOKEN
      username: ci-metrics
      icon_emoji: ":chart_with_upwards_trend:"

reports:
  flaky_rollup:
    reporters: [example]
```

Example invocation:

```sh
export CI_METRICS_SLACK_TOKEN=xoxb-...
ci-metrics report flaky_rollup --database example_db --config reports.yaml
```

Setting it up in Slack:

1. Create an app at <https://api.slack.com/apps> and add the `chat:write` bot
   token scope, plus `chat:write.customize` if you set `username` or
   `icon_emoji`.
2. Install the app to the workspace and copy the bot token (`xoxb-...`) into
   the environment variable named by `token_env`.
3. Invite the bot to the channel with `/invite @your-app`. Missing this is the
   most common failure, and it surfaces as a `not_in_channel` error at report
   time.

[mdblock]: https://docs.slack.dev/reference/block-kit/blocks/markdown-block/


## Local Dev

```sh
make test
make lint
make binary
```