-- 0021_preset_engine (down)

DROP TABLE IF EXISTS preset_applications;

ALTER TABLE presets
    DROP COLUMN channel,
    DROP COLUMN enforce;
