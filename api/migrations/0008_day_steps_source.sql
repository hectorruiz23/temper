ALTER TABLE day ADD COLUMN steps_source TEXT NULL
    CHECK(steps_source IN ('watch', 'fitness_band', 'ring', 'chest_strap', 'phone', 'smart_display', 'head_mounted', 'scale', 'other'));
