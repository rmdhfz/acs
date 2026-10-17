ALTER TABLE users
    DROP INDEX uq_users_oidc_subject,
    DROP COLUMN oidc_subject;
