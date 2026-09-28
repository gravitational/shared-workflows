-- Copy one day of suite records from the JSONL table to the Parquet table.
INSERT INTO {{.Database}}.{{.DestTable}}
SELECT suite_id, meta_id, record_schema_version, suite_name,
       "timestamp", tests, failures, errors, skipped, duration_ms, properties,
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
