ALTER TABLE users
    DROP COLUMN locked_until,
    DROP COLUMN failed_login_attempts;
