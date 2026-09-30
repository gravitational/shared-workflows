-- List GitHub Actions runs where a test failed or errored over a date window.
--
-- The test predicate is rendered by Go after escaping user input. It is either
-- an exact (classname, test_name) match when both are known, or a test_name
-- match for search results.
WITH runs AS (
    SELECT m.meta_id,
           m."timestamp" AS run_timestamp,
           m.repository_name,
           m.repository,
           m.workflow,
           m.job,
           m.run_id,
           m.run_attempt,
           regexp_replace(m.git_ref,
             '^refs/heads/gh-readonly-queue/(.*)/pr-[0-9]+.*$',
             'refs/heads/$1') AS branch,
           m.git_sha
    FROM {{.Database}}.{{.MetaTable}} m
    WHERE m.dt BETWEEN '{{.From}}' AND '{{.To}}'
      AND regexp_replace(m.git_ref,
            '^refs/heads/gh-readonly-queue/(.*)/pr-[0-9]+.*$',
            'refs/heads/$1') IN ({{.TargetBranches}})
),
failed AS (
    SELECT r.run_timestamp,
           coalesce(r.repository_name, r.repository) AS repository,
           r.workflow,
           r.job,
           r.run_id,
           r.run_attempt,
           r.branch,
           r.git_sha,
           min(t.classname) AS classname,
           t.test_name,
           min(t.status) AS status,
           min(coalesce(t.failure_message, t.error_message)) AS message,
           concat('https://github.com/', coalesce(r.repository_name, r.repository),
                  '/actions/runs/', CAST(r.run_id AS varchar),
                  '/attempts/', CAST(r.run_attempt AS varchar)) AS github_url
    FROM {{.Database}}.{{.TestcasesTable}} t
    JOIN runs r ON r.meta_id = t.meta_id
    WHERE t.dt BETWEEN '{{.From}}' AND '{{.To}}'
      AND t.status IN ('failed', 'error')
      AND {{.TestPredicate}}
    GROUP BY 1, 2, 3, 4, 5, 6, 7, 8, t.test_name,
             concat('https://github.com/', coalesce(r.repository_name, r.repository),
                    '/actions/runs/', CAST(r.run_id AS varchar),
                    '/attempts/', CAST(r.run_attempt AS varchar))
)
SELECT run_timestamp,
       repository,
       workflow,
       job,
       run_id,
       run_attempt,
       branch AS git_ref,
       git_sha,
       classname,
       test_name,
       status,
       message,
       github_url
FROM failed
ORDER BY run_timestamp DESC, repository, workflow, job
LIMIT {{.Limit}}
