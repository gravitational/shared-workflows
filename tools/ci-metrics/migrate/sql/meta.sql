-- Copy one day of workflow metadata from the JSONL table to the Parquet table.
INSERT INTO {{.Database}}.{{.DestTable}}
SELECT meta_id, record_schema_version, "timestamp",
       git_ref, git_base_ref, git_head_ref,
       actor_name, actor_id,
       runner_arch, runner_os, runner_name, runner_environment,
       git_ref_name, canonical_meta_schema_version, provider,
       workflow, job, run_id, run_attempt, git_sha, repository_name,
       s.repository,
       s.year || '-' || s.month || '-' || s.day AS dt
FROM {{.Database}}.{{.SourceTable}} s
WHERE {{.SourcePredicate}}
  AND NOT EXISTS (
      SELECT 1
      FROM {{.Database}}.{{.DestTable}} d
      WHERE d.meta_id = s.meta_id
        AND d.dt = '{{.Day}}'
  )
