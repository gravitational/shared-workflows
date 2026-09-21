-- Rank the flakiest tests over the whole reporting window.
--
-- Filtering, the score and the smoothing constant match flaky.sql; only the
-- grouping differs, so the numbers here are the window's totals rather than a
-- sum or an average of the daily rows.
--
-- Note that:
--
--   * agg groups by branch but scored drops it, so a test flaky on two
--     branches yields one row per branch
--   * the ranking has no tie-breaker beyond fails, so rn is arbitrary among
--     equal scores and can differ between runs over identical data
WITH runs AS (
    -- No day here: the window is the only bucket, so the run's timestamp
    -- matters no more than dt does.
    SELECT m.meta_id,
           regexp_replace(m.git_ref,
             '^refs/heads/gh-readonly-queue/(.*)/pr-[0-9]+.*$',
             'refs/heads/$1') AS branch
    FROM {{.Database}}.{{.MetaTable}} m
    WHERE m.dt BETWEEN '{{.From}}' AND '{{.To}}'
      AND {{.BranchPredicate}}
),
agg AS (
    SELECT r.branch, t.classname, t.test_name,
           count(*) AS execs,
           count_if(t.status IN ('failed', 'error')) AS fails
    FROM {{.Database}}.{{.TestcasesTable}} t
    JOIN runs r ON r.meta_id = t.meta_id
    WHERE t.dt BETWEEN '{{.From}}' AND '{{.To}}'
      AND t.status <> 'skipped'
    GROUP BY 1, 2, 3
),
scored AS (
    -- Same score as flaky.sql, over the window's totals:
    -- score = 4 * p * (1 - p) * execs / (execs + K)
    -- p(1-p) is variance for a coin flip, 0 when we always pass or always fail
    -- we multiply by 4 to get to a peak value approaching 1.0
    -- execs/(execs+K) uses a magic constant of K={{.Smoothing}} to push low sample tests
    -- towards 0. Over a window execs is larger than it is in a day, so the
    -- term is closer to 1 and the smoothing bites less.
    SELECT classname, test_name, execs, fails,
           round(100.0 * fails / execs, 2) AS fail_pct,
           round(4.0 * (CAST(fails AS double) / execs)
                     * (1 - CAST(fails AS double) / execs)
                     * (execs / (execs + {{.Smoothing}})), 4) AS flake_score
    FROM agg
    WHERE execs >= {{.MinExecs}} AND fails > 0 AND fails < execs
)
SELECT rn, classname, test_name, execs, fails, fail_pct, flake_score
FROM (
    SELECT *, row_number() OVER (
               ORDER BY flake_score DESC, fails DESC) AS rn
    FROM scored
)
WHERE rn <= {{.Top}}
ORDER BY rn
