DELETE FROM vendor_parameter_mappings WHERE logical_key IN ('optical.rx_power', 'optical.tx_power') AND value_scale = 0.001;
ALTER TABLE vendor_parameter_mappings DROP COLUMN value_scale;
