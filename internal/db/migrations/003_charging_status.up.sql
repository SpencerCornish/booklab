-- Transitional status while a charge is in flight (prevents concurrent double-charge).
ALTER TYPE booking_status ADD VALUE 'charging';
