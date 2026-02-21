-- +goose Up

-- #10: Add index on request_logs.request_id for fast lookup by trace/correlation ID.
CREATE INDEX idx_logs_request_id ON request_logs (request_id);

-- #11: Add 2027 and 2028 monthly partitions to prevent INSERT failures after 2026-12-31.
CREATE TABLE request_logs_2027_01 PARTITION OF request_logs
    FOR VALUES FROM ('2027-01-01') TO ('2027-02-01');

CREATE TABLE request_logs_2027_02 PARTITION OF request_logs
    FOR VALUES FROM ('2027-02-01') TO ('2027-03-01');

CREATE TABLE request_logs_2027_03 PARTITION OF request_logs
    FOR VALUES FROM ('2027-03-01') TO ('2027-04-01');

CREATE TABLE request_logs_2027_04 PARTITION OF request_logs
    FOR VALUES FROM ('2027-04-01') TO ('2027-05-01');

CREATE TABLE request_logs_2027_05 PARTITION OF request_logs
    FOR VALUES FROM ('2027-05-01') TO ('2027-06-01');

CREATE TABLE request_logs_2027_06 PARTITION OF request_logs
    FOR VALUES FROM ('2027-06-01') TO ('2027-07-01');

CREATE TABLE request_logs_2027_07 PARTITION OF request_logs
    FOR VALUES FROM ('2027-07-01') TO ('2027-08-01');

CREATE TABLE request_logs_2027_08 PARTITION OF request_logs
    FOR VALUES FROM ('2027-08-01') TO ('2027-09-01');

CREATE TABLE request_logs_2027_09 PARTITION OF request_logs
    FOR VALUES FROM ('2027-09-01') TO ('2027-10-01');

CREATE TABLE request_logs_2027_10 PARTITION OF request_logs
    FOR VALUES FROM ('2027-10-01') TO ('2027-11-01');

CREATE TABLE request_logs_2027_11 PARTITION OF request_logs
    FOR VALUES FROM ('2027-11-01') TO ('2027-12-01');

CREATE TABLE request_logs_2027_12 PARTITION OF request_logs
    FOR VALUES FROM ('2027-12-01') TO ('2028-01-01');

CREATE TABLE request_logs_2028_01 PARTITION OF request_logs
    FOR VALUES FROM ('2028-01-01') TO ('2028-02-01');

CREATE TABLE request_logs_2028_02 PARTITION OF request_logs
    FOR VALUES FROM ('2028-02-01') TO ('2028-03-01');

CREATE TABLE request_logs_2028_03 PARTITION OF request_logs
    FOR VALUES FROM ('2028-03-01') TO ('2028-04-01');

CREATE TABLE request_logs_2028_04 PARTITION OF request_logs
    FOR VALUES FROM ('2028-04-01') TO ('2028-05-01');

CREATE TABLE request_logs_2028_05 PARTITION OF request_logs
    FOR VALUES FROM ('2028-05-01') TO ('2028-06-01');

CREATE TABLE request_logs_2028_06 PARTITION OF request_logs
    FOR VALUES FROM ('2028-06-01') TO ('2028-07-01');

CREATE TABLE request_logs_2028_07 PARTITION OF request_logs
    FOR VALUES FROM ('2028-07-01') TO ('2028-08-01');

CREATE TABLE request_logs_2028_08 PARTITION OF request_logs
    FOR VALUES FROM ('2028-08-01') TO ('2028-09-01');

CREATE TABLE request_logs_2028_09 PARTITION OF request_logs
    FOR VALUES FROM ('2028-09-01') TO ('2028-10-01');

CREATE TABLE request_logs_2028_10 PARTITION OF request_logs
    FOR VALUES FROM ('2028-10-01') TO ('2028-11-01');

CREATE TABLE request_logs_2028_11 PARTITION OF request_logs
    FOR VALUES FROM ('2028-11-01') TO ('2028-12-01');

CREATE TABLE request_logs_2028_12 PARTITION OF request_logs
    FOR VALUES FROM ('2028-12-01') TO ('2029-01-01');

-- #12: Replace the permissive audit_logs SELECT policy (USING true) with an
-- org-scoped policy. Rows without an org_id (system events) remain visible to
-- all sessions; rows with an org_id are visible only when the session variable
-- app.current_org_id matches.
--
-- Application code must set:  SET LOCAL app.current_org_id = '<uuid>';
-- before querying audit_logs within a transaction.
DROP POLICY IF EXISTS audit_log_select ON audit_logs;

CREATE POLICY audit_log_select ON audit_logs
    FOR SELECT
    USING (
        org_id IS NULL
        OR org_id = current_setting('app.current_org_id', true)::uuid
    );

-- +goose Down

DROP POLICY IF EXISTS audit_log_select ON audit_logs;
CREATE POLICY audit_log_select ON audit_logs FOR SELECT USING (true);

DROP TABLE IF EXISTS request_logs_2028_12;
DROP TABLE IF EXISTS request_logs_2028_11;
DROP TABLE IF EXISTS request_logs_2028_10;
DROP TABLE IF EXISTS request_logs_2028_09;
DROP TABLE IF EXISTS request_logs_2028_08;
DROP TABLE IF EXISTS request_logs_2028_07;
DROP TABLE IF EXISTS request_logs_2028_06;
DROP TABLE IF EXISTS request_logs_2028_05;
DROP TABLE IF EXISTS request_logs_2028_04;
DROP TABLE IF EXISTS request_logs_2028_03;
DROP TABLE IF EXISTS request_logs_2028_02;
DROP TABLE IF EXISTS request_logs_2028_01;
DROP TABLE IF EXISTS request_logs_2027_12;
DROP TABLE IF EXISTS request_logs_2027_11;
DROP TABLE IF EXISTS request_logs_2027_10;
DROP TABLE IF EXISTS request_logs_2027_09;
DROP TABLE IF EXISTS request_logs_2027_08;
DROP TABLE IF EXISTS request_logs_2027_07;
DROP TABLE IF EXISTS request_logs_2027_06;
DROP TABLE IF EXISTS request_logs_2027_05;
DROP TABLE IF EXISTS request_logs_2027_04;
DROP TABLE IF EXISTS request_logs_2027_03;
DROP TABLE IF EXISTS request_logs_2027_02;
DROP TABLE IF EXISTS request_logs_2027_01;

DROP INDEX IF EXISTS idx_logs_request_id;
