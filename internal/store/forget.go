package store

// forgetGroups is migration 4. The groups of the model before 2026-11-26 are
// deleted, not converted (planning/contracts/storage.md section 7). inbox keeps
// its AUTOINCREMENT sequence, so a host's audit of delivered item ids stays true.
const forgetGroups = `DELETE FROM row;
DELETE FROM grp;
DELETE FROM inbox;
DELETE FROM kb;
DELETE FROM perm;
DELETE FROM session;
DELETE FROM approval;
DELETE FROM setting WHERE key IN ('homes', 'moved');
DELETE FROM device_log WHERE user IS NOT (SELECT value->>'$' FROM setting WHERE key = 'owner');
DELETE FROM push_sub WHERE user IS NOT (SELECT value->>'$' FROM setting WHERE key = 'owner');
ALTER TABLE session DROP COLUMN takeover_at;
ALTER TABLE session ADD COLUMN report TEXT`
