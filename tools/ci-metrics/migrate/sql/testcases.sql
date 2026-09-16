-- Copy one day of testcase records from the JSONL table to the Parquet table.
INSERT INTO {{.Database}}.{{.DestTable}}
SELECT testcase_id, suite_id, meta_id, record_schema_version,
       test_name, suite_name, classname, duration_ms, status,
       skip_message, error_message, failure_message,
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
