-- migration/000002_add_summary.up.sql
ALTER TABLE research_sessions ADD COLUMN summary TEXT;
