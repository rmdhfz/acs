ALTER TABLE tenants
    DROP KEY uq_tenants_cwmp_inform_username,
    DROP COLUMN cwmp_inform_password_enc,
    DROP COLUMN cwmp_inform_username;
