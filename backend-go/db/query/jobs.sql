-- name: CreateJob :one
INSERT INTO jobs (
    job_id,
    job_type,
    scope_type,
    scope_id,
    status,
    priority,
    payload
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7
)
RETURNING *;

-- name: GetJob :one
SELECT *
FROM jobs
WHERE job_id = $1
LIMIT 1;
