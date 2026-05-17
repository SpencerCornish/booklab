-- Enum value 'charging' is not dropped (PostgreSQL cannot remove enum values safely).
UPDATE bookings SET status = 'completed' WHERE status = 'charging';
