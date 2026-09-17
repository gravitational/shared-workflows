-- Rank the flakiest tests per day over the reporting window.
--
-- Reads the Parquet tables, which collapse the JSONL year/month/day partitions
-- into a single ISO-8601 dt, so the window is a plain BETWEEN on one column
-- rather than a predicate per calendar month. Both sides of the join carry
-- their own dt, so it is applied twice to prune each.
--
-- Only runs on the configured branches count, which keeps pull request noise
-- out of the ranking. Merge-queue runs are admitted wholesale and their ref
-- normalised back to the branch they target, so a queued run counts towards
-- that branch.
--
-- A test is a flake candidate only when it both passed and failed on the same
-- day, so the scored CTE drops anything that always passed (fails = 0) or
-- always failed (fails = execs) - a consistent failure is broken, not flaky.
--
-- This is kept deliberately identical to the query run by hand against Athena
-- so that the two can be compared directly.
--
--   * agg groups by branch but scored drops it, so a test flaky on two
--     branches yields one row per branch
--   * the ranking has no tie-breaker beyond fails, so rn is arbitrary among
--     equal scores and can differ between runs over identical data.
WITH runs AS (
    -- The day comes from the run's own timestamp rather than from dt, which is
    -- the ingest date and can land on the following day for a run that
    -- finishes near midnight.
    SELECT m.meta_id,
           regexp_replace(m.git_ref,
             '^refs/heads/gh-readonly-queue/(.*)/pr-[0-9]+.*$',
             'refs/heads/$1') AS branch,
           date(from_iso8601_timestamp(m."timestamp")) AS day
    FROM {{.Database}}.{{.MetaTable}} m
    WHERE m.dt BETWEEN '{{.From}}' AND '{{.To}}'
      AND {{.BranchPredicate}}
),
agg AS (
    SELECT r.day, r.branch, t.classname, t.test_name,
           count(*) AS execs,
           count_if(t.status IN ('failed', 'error')) AS fails
    FROM {{.Database}}.{{.TestcasesTable}} t
    JOIN runs r ON r.meta_id = t.meta_id
    WHERE t.dt BETWEEN '{{.From}}' AND '{{.To}}'
      AND t.status <> 'skipped'
    GROUP BY 1, 2, 3, 4
),
scored AS (
    -- calculate a flake score
    -- score = 4 * p * (1 - p) * execs / (execs + K)
    -- p(1-p) is variance for a coin flip, 0 when we always pass or always fail
    -- we multiply by 4 to get to a peak value approaching 1.0
    -- execs/(execs+K) uses a magic constant of K={{.Smoothing}} to push low sample tests
    -- towards 0. Without it both 5/10 and 100/200 score perfect 1.0 despite the
    -- latter being much bigger pain for developers.
    SELECT day, classname, test_name, execs, fails,
           round(100.0 * fails / execs, 2) AS fail_pct,
           round(4.0 * (CAST(fails AS double) / execs)
                     * (1 - CAST(fails AS double) / execs)
                     * (execs / (execs + {{.Smoothing}})), 4) AS flake_score
    FROM agg
    WHERE execs >= {{.MinExecs}} AND fails > 0 AND fails < execs
)
SELECT day, rn, classname, test_name, execs, fails, fail_pct, flake_score
FROM (
    SELECT *, row_number() OVER (
               PARTITION BY day
               ORDER BY flake_score DESC, fails DESC) AS rn
    FROM scored
)
WHERE rn <= {{.Top}}
ORDER BY day DESC, rn
