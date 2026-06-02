-- migration/000002_add_summary.down.sql
-- SQLite doesn't support dropping columns directly in older versions, 
-- but for simplicity we'll just leave it or recreate the table if needed.
-- In a real prod env, we'd do the table-swap dance.
SELECT 1;
