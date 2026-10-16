DROP FUNCTION IF EXISTS set_updated_at();
-- citext/pgcrypto and the wallet_app role are left in place intentionally:
-- other objects may depend on them and roles are cluster-wide.
