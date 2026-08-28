ALTER TABLE vendor_parameter_mappings
    DROP KEY uq_vpm_scope_key,
    ADD UNIQUE KEY uq_vpm_scope_key (vendor_id, data_model_version_id, device_model_id, logical_key),
    DROP COLUMN software_version_pattern;
