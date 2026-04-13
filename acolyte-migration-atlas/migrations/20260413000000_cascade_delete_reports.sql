-- Add ON DELETE CASCADE to all FKs that transitively reference reports.
-- Enables single-statement DELETE FROM reports WHERE report_id = $1.

ALTER TABLE report_versions
    DROP CONSTRAINT report_versions_report_id_fkey,
    ADD CONSTRAINT report_versions_report_id_fkey
        FOREIGN KEY (report_id) REFERENCES reports(report_id) ON DELETE CASCADE;

ALTER TABLE report_change_items
    DROP CONSTRAINT report_change_items_report_id_version_no_fkey,
    ADD CONSTRAINT report_change_items_report_id_version_no_fkey
        FOREIGN KEY (report_id, version_no) REFERENCES report_versions(report_id, version_no) ON DELETE CASCADE;

ALTER TABLE report_sections
    DROP CONSTRAINT report_sections_report_id_fkey,
    ADD CONSTRAINT report_sections_report_id_fkey
        FOREIGN KEY (report_id) REFERENCES reports(report_id) ON DELETE CASCADE;

ALTER TABLE report_section_versions
    DROP CONSTRAINT report_section_versions_report_id_section_key_fkey,
    ADD CONSTRAINT report_section_versions_report_id_section_key_fkey
        FOREIGN KEY (report_id, section_key) REFERENCES report_sections(report_id, section_key) ON DELETE CASCADE;

ALTER TABLE report_runs
    DROP CONSTRAINT report_runs_report_id_fkey,
    ADD CONSTRAINT report_runs_report_id_fkey
        FOREIGN KEY (report_id) REFERENCES reports(report_id) ON DELETE CASCADE;

ALTER TABLE report_jobs
    DROP CONSTRAINT report_jobs_run_id_fkey,
    ADD CONSTRAINT report_jobs_run_id_fkey
        FOREIGN KEY (run_id) REFERENCES report_runs(run_id) ON DELETE CASCADE;

ALTER TABLE report_briefs
    DROP CONSTRAINT report_briefs_report_id_fkey,
    ADD CONSTRAINT report_briefs_report_id_fkey
        FOREIGN KEY (report_id) REFERENCES reports(report_id) ON DELETE CASCADE;
